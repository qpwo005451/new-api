package service

import (
	"context"
	"net/http"
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

const virtualRouteHealthRedisPrefix = "new-api:virtual-sticky:health:v1:"

type VirtualRouteHealthStore interface {
	IsCoolingDown(ctx context.Context, key string, now time.Time) (bool, error)
	RecordFailure(ctx context.Context, key string, health operation_setting.VirtualModelRouteHealth, now time.Time) error
	RecordSuccess(ctx context.Context, key string) error
}

var virtualRouteHealthMemoryStore = NewVirtualRouteMemoryHealthStore()

func virtualRouteHealthStoreForSetting(setting operation_setting.VirtualPoolStickySetting) (VirtualRouteHealthStore, error) {
	if !setting.Enabled || setting.BindingMode != operation_setting.VirtualPoolBindingModeRedis {
		return virtualRouteHealthMemoryStore, nil
	}
	if !common.RedisEnabled || common.RDB == nil {
		if setting.RedisRequiredForReady {
			return nil, ErrVirtualPoolStoreUnavailable
		}
		common.SysError("virtual route health Redis is unavailable; falling back to process-local health")
		return virtualRouteHealthMemoryStore, nil
	}
	return NewVirtualPoolRedisHealthStore(common.RDB), nil
}

type virtualRouteHealthEntry struct {
	mutex             sync.Mutex
	consecutiveErrors int
	cooldownUntil     time.Time
}

func virtualRouteHealthKey(virtualModel string, channelID int, upstreamModel string) string {
	return strings.ToLower(strings.TrimSpace(virtualModel)) + "|" +
		strconv.Itoa(channelID) + "|" +
		strings.ToLower(strings.TrimSpace(upstreamModel))
}

type VirtualRouteMemoryHealthStore struct {
	mutex   sync.Mutex
	entries map[string]*virtualRouteHealthEntry
}

func NewVirtualRouteMemoryHealthStore() *VirtualRouteMemoryHealthStore {
	return &VirtualRouteMemoryHealthStore{entries: make(map[string]*virtualRouteHealthEntry)}
}

func (store *VirtualRouteMemoryHealthStore) entry(key string) *virtualRouteHealthEntry {
	if store == nil {
		return &virtualRouteHealthEntry{}
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	entry := store.entries[key]
	if entry == nil {
		entry = &virtualRouteHealthEntry{}
		store.entries[key] = entry
	}
	return entry
}

func (store *VirtualRouteMemoryHealthStore) IsCoolingDown(_ context.Context, key string, now time.Time) (bool, error) {
	if store == nil {
		return false, nil
	}
	return store.entry(key).isCoolingDown(now), nil
}

func (store *VirtualRouteMemoryHealthStore) RecordFailure(
	_ context.Context,
	key string,
	health operation_setting.VirtualModelRouteHealth,
	now time.Time,
) error {
	if store == nil {
		return nil
	}
	store.entry(key).recordFailure(health, now)
	return nil
}

func (store *VirtualRouteMemoryHealthStore) RecordSuccess(_ context.Context, key string) error {
	if store == nil {
		return nil
	}
	store.entry(key).recordSuccess()
	return nil
}

// RecordVirtualRouteFailure cools the pool entry that just failed down, so the
// healthy entries of the pool are preferred for the next requests.
func RecordVirtualRouteFailure(c *gin.Context, channelID int, virtualModel string, statusCode int) {
	key, health, ok := virtualRouteHealthTarget(c, channelID, virtualModel)
	if !ok || !IsVirtualRouteUnavailableStatus(statusCode) {
		return
	}
	store, err := virtualRouteHealthStore()
	if err != nil {
		common.SysError("failed to open virtual route health store: " + err.Error())
		return
	}
	if err := store.RecordFailure(serviceContext(c), key, health, time.Now()); err != nil {
		common.SysError("failed to record virtual route health failure: " + err.Error())
	}
}

// RecordVirtualRouteSuccess clears the cooldown of the pool entry that served
// the request, so a recovered entry returns to the healthy part of the pool.
func RecordVirtualRouteSuccess(c *gin.Context, channelID int, virtualModel string) {
	key, _, ok := virtualRouteHealthTarget(c, channelID, virtualModel)
	if !ok {
		return
	}
	store, err := virtualRouteHealthStore()
	if err != nil {
		common.SysError("failed to open virtual route health store: " + err.Error())
		return
	}
	if err := store.RecordSuccess(serviceContext(c), key); err != nil {
		common.SysError("failed to record virtual route health success: " + err.Error())
	}
}

func virtualRouteHealthStore() (VirtualRouteHealthStore, error) {
	setting := operation_setting.GetModelRetryPolicySetting().VirtualPoolSticky.Normalize()
	return virtualRouteHealthStoreForSetting(setting)
}

func virtualRouteHealthTarget(c *gin.Context, channelID int, virtualModel string) (string, operation_setting.VirtualModelRouteHealth, bool) {
	route := operation_setting.GetVirtualModelRoute(virtualModel)
	if !route.Health.Enabled {
		return "", operation_setting.VirtualModelRouteHealth{}, false
	}
	upstreamModel := common.GetContextKeyString(c, constant.ContextKeyVirtualUpstreamModel)
	if upstreamModel == "" {
		return "", operation_setting.VirtualModelRouteHealth{}, false
	}
	return virtualRouteHealthKey(virtualModel, channelID, upstreamModel), route.Health.Normalize(), true
}

// isCoolingDown reports whether the entry should be tried only after the
// healthy entries of the pool.
func (entry *virtualRouteHealthEntry) isCoolingDown(now time.Time) bool {
	entry.mutex.Lock()
	defer entry.mutex.Unlock()
	return now.Before(entry.cooldownUntil)
}

func (entry *virtualRouteHealthEntry) recordFailure(health operation_setting.VirtualModelRouteHealth, now time.Time) {
	entry.mutex.Lock()
	defer entry.mutex.Unlock()

	entry.consecutiveErrors++
	cooldown := virtualRouteHealthCooldown(entry.consecutiveErrors, health)
	if cooldown > 0 {
		entry.cooldownUntil = now.Add(cooldown)
	}
}

func (entry *virtualRouteHealthEntry) recordSuccess() {
	entry.mutex.Lock()
	defer entry.mutex.Unlock()

	entry.consecutiveErrors = 0
	entry.cooldownUntil = time.Time{}
}

func virtualRouteHealthCooldown(consecutiveErrors int, health operation_setting.VirtualModelRouteHealth) time.Duration {
	health = health.Normalize()
	if !health.Enabled || consecutiveErrors < health.FailureThreshold {
		return 0
	}
	cooldown := time.Duration(health.CooldownSeconds) * time.Second
	maxCooldown := time.Duration(health.MaxCooldownSeconds) * time.Second
	for step := consecutiveErrors - health.FailureThreshold; step > 0; step-- {
		cooldown *= 2
		if cooldown >= maxCooldown {
			break
		}
	}
	if cooldown > maxCooldown {
		cooldown = maxCooldown
	}
	return cooldown
}

// IsVirtualRouteUnavailableStatus reports whether an upstream status means the
// pool entry stopped serving, as opposed to a client-side request problem.
func IsVirtualRouteUnavailableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusNotFound ||
		statusCode >= http.StatusInternalServerError
}

type VirtualPoolRedisHealthStore struct {
	client *redis.Client
}

func NewVirtualPoolRedisHealthStore(client *redis.Client) *VirtualPoolRedisHealthStore {
	return &VirtualPoolRedisHealthStore{client: client}
}

func (store *VirtualPoolRedisHealthStore) IsCoolingDown(
	ctx context.Context,
	key string,
	_ time.Time,
) (bool, error) {
	if store == nil || store.client == nil {
		return false, ErrVirtualPoolStoreUnavailable
	}
	cooling, err := store.client.Eval(ctx, virtualPoolHealthCooldownScript, []string{store.key(key)}).Int64()
	if err != nil {
		return false, err
	}
	return cooling == 1, nil
}

func (store *VirtualPoolRedisHealthStore) RecordFailure(
	ctx context.Context,
	key string,
	health operation_setting.VirtualModelRouteHealth,
	_ time.Time,
) error {
	if store == nil || store.client == nil {
		return ErrVirtualPoolStoreUnavailable
	}
	health = health.Normalize()
	ttl := time.Duration(health.MaxCooldownSeconds) * time.Second
	if ttl < time.Hour {
		ttl = time.Hour
	}
	return store.client.Eval(ctx, virtualPoolHealthFailureScript, []string{store.key(key)},
		health.FailureThreshold,
		int64(health.CooldownSeconds)*1000,
		int64(health.MaxCooldownSeconds)*1000,
		ttl.Milliseconds(),
	).Err()
}

func (store *VirtualPoolRedisHealthStore) RecordSuccess(ctx context.Context, key string) error {
	if store == nil || store.client == nil {
		return ErrVirtualPoolStoreUnavailable
	}
	return store.client.Del(ctx, store.key(key)).Err()
}

func (store *VirtualPoolRedisHealthStore) key(key string) string {
	return virtualRouteHealthRedisPrefix + strings.TrimSpace(key)
}

const virtualPoolHealthCooldownScript = `
local now = redis.call('TIME')
local now_ms = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
local cooldown_until = tonumber(redis.call('HGET', KEYS[1], 'cooldown_until') or '0')
if cooldown_until > now_ms then
  return 1
end
return 0
`

const virtualPoolHealthFailureScript = `
local now = redis.call('TIME')
local now_ms = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
local consecutive = tonumber(redis.call('HGET', KEYS[1], 'consecutive_errors') or '0') + 1
redis.call('HSET', KEYS[1], 'consecutive_errors', consecutive)
local threshold = tonumber(ARGV[1])
if consecutive >= threshold then
  local cooldown = tonumber(ARGV[2])
  local maximum = tonumber(ARGV[3])
  local step = consecutive - threshold
  while step > 0 do
    cooldown = cooldown * 2
    if cooldown >= maximum then
      cooldown = maximum
      break
    end
    step = step - 1
  end
  if cooldown > maximum then
    cooldown = maximum
  end
  redis.call('HSET', KEYS[1], 'cooldown_until', now_ms + cooldown)
end
redis.call('PEXPIRE', KEYS[1], ARGV[4])
return consecutive
`
