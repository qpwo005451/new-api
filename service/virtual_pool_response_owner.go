package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const virtualPoolResponseOwnerRedisPrefix = "new-api:virtual-sticky:response-owner:v1:"

var (
	ErrVirtualPoolResponseOwnerUnknown = errors.New("virtual pool response owner is unknown")
	ErrVirtualPoolResponseOwnerInvalid = errors.New("virtual pool previous_response_id is invalid")
)

// VirtualPoolResponseOwner records which pool candidate created an upstream
// response. A later request that references previous_response_id must use that
// same account/model identity instead of being routed to another upstream.
type VirtualPoolResponseOwner struct {
	ResponseID   string
	CandidateKey string
	ExpiresAt    time.Time
}

type VirtualPoolResponseOwnerStore interface {
	Save(ctx context.Context, scope, responseID, candidateKey string, now time.Time, ttl time.Duration) error
	Lookup(ctx context.Context, scope, responseID string, now time.Time) (VirtualPoolResponseOwner, bool, error)
}

func VirtualPoolResponseOwnerStoreForSetting(setting operation_setting.VirtualPoolStickySetting) (VirtualPoolResponseOwnerStore, error) {
	if setting.RedisRequiredForReady && (!common.RedisEnabled || common.RDB == nil) {
		return nil, ErrVirtualPoolStoreUnavailable
	}
	if setting.BindingMode == operation_setting.VirtualPoolBindingModeRedis {
		if !common.RedisEnabled || common.RDB == nil {
			return nil, ErrVirtualPoolStoreUnavailable
		}
		return NewVirtualPoolRedisResponseOwnerStore(common.RDB), nil
	}
	return getVirtualPoolMemoryResponseOwnerStore(), nil
}

var (
	virtualPoolMemoryResponseOwnerOnce  sync.Once
	virtualPoolMemoryResponseOwnerStore *VirtualPoolMemoryResponseOwnerStore
)

func getVirtualPoolMemoryResponseOwnerStore() *VirtualPoolMemoryResponseOwnerStore {
	virtualPoolMemoryResponseOwnerOnce.Do(func() {
		virtualPoolMemoryResponseOwnerStore = NewVirtualPoolMemoryResponseOwnerStore()
	})
	return virtualPoolMemoryResponseOwnerStore
}

// RecordVirtualPoolResponseOwner persists the upstream response id so later
// continuation requests can be pinned to the account that owns the state.
func RecordVirtualPoolResponseOwner(c *gin.Context, responseID, candidateKey string) {
	if c == nil || strings.TrimSpace(responseID) == "" || strings.TrimSpace(candidateKey) == "" {
		return
	}
	setting := operation_setting.GetModelRetryPolicySetting().VirtualPoolSticky.Normalize()
	if !setting.Enabled {
		return
	}
	scope := virtualPoolResponseOwnerScope(c, common.GetContextKeyString(c, constant.ContextKeyOriginalModel))
	if scope == "" {
		return
	}
	store, err := VirtualPoolResponseOwnerStoreForSetting(setting)
	if err != nil {
		common.SysError("failed to open virtual pool response owner store: " + err.Error())
		return
	}
	if err := store.Save(c.Request.Context(), scope, responseID, candidateKey, time.Now(), time.Duration(setting.ConfirmedTTLSeconds)*time.Second); err != nil {
		common.SysError("failed to save virtual pool response owner: " + err.Error())
	}
}

// ResolveVirtualPoolResponseOwner looks up the candidate that produced a
// previous Responses API id. The lookup is scoped to the authenticated
// user/token/group/model so one caller cannot pin another caller's state.
func ResolveVirtualPoolResponseOwner(c *gin.Context, modelName string) (*VirtualPoolResponseOwner, error) {
	if c == nil || c.Request == nil {
		return nil, nil
	}
	setting := operation_setting.GetModelRetryPolicySetting().VirtualPoolSticky.Normalize()
	if !setting.Enabled {
		return nil, nil
	}
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	var body struct {
		PreviousResponseID string `json:"previous_response_id"`
	}
	if err := common.UnmarshalBodyReusable(c, &body); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrVirtualPoolResponseOwnerInvalid, err)
	}
	responseID := strings.TrimSpace(body.PreviousResponseID)
	if responseID == "" {
		return nil, nil
	}
	if !setting.Allows(userID, tokenID, group, modelName) {
		return nil, nil
	}
	scope := virtualPoolResponseOwnerScope(c, modelName)
	if scope == "" {
		return nil, nil
	}
	store, err := VirtualPoolResponseOwnerStoreForSetting(setting)
	if err != nil {
		return nil, err
	}
	owner, found, err := store.Lookup(c.Request.Context(), scope, responseID, time.Now())
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrVirtualPoolResponseOwnerUnknown
	}
	return &owner, nil
}

func virtualPoolResponseOwnerScope(c *gin.Context, modelName string) string {
	if c == nil {
		return ""
	}
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	tokenID := common.GetContextKeyInt(c, constant.ContextKeyTokenId)
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	if modelName == "" {
		return ""
	}
	return strings.Join([]string{
		"user=" + strconv.Itoa(userID),
		"token=" + strconv.Itoa(tokenID),
		"group=" + strings.ToLower(strings.TrimSpace(group)),
		"model=" + modelName,
		"v=1",
	}, "\x00")
}

type VirtualPoolMemoryResponseOwnerStore struct {
	mu     sync.Mutex
	owners map[string]VirtualPoolResponseOwner
}

func NewVirtualPoolMemoryResponseOwnerStore() *VirtualPoolMemoryResponseOwnerStore {
	return &VirtualPoolMemoryResponseOwnerStore{owners: make(map[string]VirtualPoolResponseOwner)}
}

func (store *VirtualPoolMemoryResponseOwnerStore) Save(
	_ context.Context,
	scope string,
	responseID string,
	candidateKey string,
	now time.Time,
	ttl time.Duration,
) error {
	scope = strings.TrimSpace(scope)
	responseID = strings.TrimSpace(responseID)
	candidateKey = strings.TrimSpace(candidateKey)
	if scope == "" || responseID == "" || candidateKey == "" {
		return ErrVirtualPoolResponseOwnerUnknown
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneExpired(now)
	store.owners[responseOwnerMemoryKey(scope, responseID)] = VirtualPoolResponseOwner{
		ResponseID:   responseID,
		CandidateKey: candidateKey,
		ExpiresAt:    now.Add(ttl),
	}
	return nil
}

func (store *VirtualPoolMemoryResponseOwnerStore) Lookup(
	_ context.Context,
	scope string,
	responseID string,
	now time.Time,
) (VirtualPoolResponseOwner, bool, error) {
	scope = strings.TrimSpace(scope)
	responseID = strings.TrimSpace(responseID)
	if scope == "" || responseID == "" {
		return VirtualPoolResponseOwner{}, false, nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneExpired(now)
	key := responseOwnerMemoryKey(scope, responseID)
	owner, exists := store.owners[key]
	if !exists || !owner.ExpiresAt.After(now) {
		delete(store.owners, key)
		return VirtualPoolResponseOwner{}, false, nil
	}
	return owner, true, nil
}

func (store *VirtualPoolMemoryResponseOwnerStore) pruneExpired(now time.Time) {
	for key, owner := range store.owners {
		if !owner.ExpiresAt.After(now) {
			delete(store.owners, key)
		}
	}
}

func responseOwnerMemoryKey(scope, responseID string) string {
	return scope + "\x00" + responseID
}

type VirtualPoolRedisResponseOwnerStore struct {
	client *redis.Client
}

func NewVirtualPoolRedisResponseOwnerStore(client *redis.Client) *VirtualPoolRedisResponseOwnerStore {
	return &VirtualPoolRedisResponseOwnerStore{client: client}
}

func (store *VirtualPoolRedisResponseOwnerStore) Save(
	ctx context.Context,
	scope string,
	responseID string,
	candidateKey string,
	now time.Time,
	ttl time.Duration,
) error {
	if store == nil || store.client == nil {
		return ErrVirtualPoolStoreUnavailable
	}
	scope = strings.TrimSpace(scope)
	responseID = strings.TrimSpace(responseID)
	candidateKey = strings.TrimSpace(candidateKey)
	if scope == "" || responseID == "" || candidateKey == "" {
		return ErrVirtualPoolResponseOwnerUnknown
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	return store.client.Set(ctx, store.key(scope, responseID), candidateKey, ttl).Err()
}

func (store *VirtualPoolRedisResponseOwnerStore) Lookup(
	ctx context.Context,
	scope string,
	responseID string,
	_ time.Time,
) (VirtualPoolResponseOwner, bool, error) {
	if store == nil || store.client == nil {
		return VirtualPoolResponseOwner{}, false, ErrVirtualPoolStoreUnavailable
	}
	scope = strings.TrimSpace(scope)
	responseID = strings.TrimSpace(responseID)
	if scope == "" || responseID == "" {
		return VirtualPoolResponseOwner{}, false, nil
	}
	candidateKey, err := store.client.Get(ctx, store.key(scope, responseID)).Result()
	if errors.Is(err, redis.Nil) {
		return VirtualPoolResponseOwner{}, false, nil
	}
	if err != nil {
		return VirtualPoolResponseOwner{}, false, err
	}
	if strings.TrimSpace(candidateKey) == "" {
		return VirtualPoolResponseOwner{}, false, nil
	}
	return VirtualPoolResponseOwner{
		ResponseID:   responseID,
		CandidateKey: candidateKey,
		ExpiresAt:    time.Now().Add(time.Hour),
	}, true, nil
}

func (store *VirtualPoolRedisResponseOwnerStore) key(scope, responseID string) string {
	return virtualPoolResponseOwnerRedisPrefix + common.Sha1([]byte(scope+"\x00"+strings.TrimSpace(responseID)))
}
