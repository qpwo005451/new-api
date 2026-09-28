package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

var ErrVirtualPoolCapacityExhausted = errVirtualPoolCapacityExhausted

// virtualRouteRotationCounters holds the round_robin position of each virtual
// model, keyed by the lowercased model name.
var virtualRouteRotationCounters sync.Map

type RetryParam struct {
	Ctx                  *gin.Context
	TokenGroup           string
	ModelName            string
	RequestPath          string
	Retry                *int
	attemptedChannelIDs  map[int]struct{}
	virtualRoute         []virtualRouteCandidate
	virtualReady         bool
	virtualErr           error
	virtualOffset        int
	virtualLimit         int
	virtualHealthy       int
	virtualSession       *VirtualPoolSession
	preparedRoute        *VirtualPoolPreparedRoute
	requiredCandidateKey string
	attempted            map[string]struct{}
	scheduled            *VirtualPoolScheduledCandidate
	scheduledTaken       bool
	scheduler            *VirtualPoolScheduler
	owner                string
	resetNextTry         bool
}

type virtualRouteCandidate struct {
	channel          *model.Channel
	virtualModel     string
	upstreamModel    string
	finalMappedModel string
	reasoningEffort  string
	group            string
	keyIndex         int
	accountIdentity  string
	capacity         int
	weight           float64
	capacityGroup    string
}

// VirtualPoolPreparedRoute is the request-scoped candidate list. Middleware
// prepares it once; the controller consumes the same instance on retries.
type VirtualPoolPreparedRoute struct {
	ModelName            string
	RequestPath          string
	TokenGroup           string
	CapacityEnabled      bool
	Candidates           []VirtualPoolCandidate
	Limit                int
	Offset               int
	Healthy              int
	Session              *VirtualPoolSession
	Scheduled            *VirtualPoolScheduledCandidate
	RequiredCandidateKey string
	scheduler            *VirtualPoolScheduler
	owner                string
}

// VirtualPoolCandidate is one immutable execution identity for a virtual model
// attempt. AccountIdentity includes a bound multi-key index when applicable.
type VirtualPoolCandidate struct {
	Channel          *model.Channel
	VirtualModel     string
	UpstreamModel    string
	FinalMappedModel string
	ReasoningEffort  string
	Group            string
	KeyIndex         int
	AccountIdentity  string
	Capacity         int
	Weight           float64
	CapacityGroup    string
}

func (candidate VirtualPoolCandidate) AttemptKey() string {
	return strings.Join([]string{
		candidate.AccountIdentity,
		strings.ToLower(strings.TrimSpace(candidate.FinalMappedModel)),
		strings.ToLower(strings.TrimSpace(candidate.ReasoningEffort)),
		strings.ToLower(strings.TrimSpace(candidate.Group)),
	}, "|")
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

// MarkAttemptedChannel records that a relay attempt already ran on a channel.
// Later in-request retries on policy models must prefer fresh channels over
// re-hitting one that just failed (see selectSatisfiedChannelWithModelHealth).
func (p *RetryParam) MarkAttemptedChannel(channelID int) {
	if p == nil || channelID <= 0 {
		return
	}
	if p.attemptedChannelIDs == nil {
		p.attemptedChannelIDs = make(map[int]struct{})
	}
	p.attemptedChannelIDs[channelID] = struct{}{}
}

func (p *RetryParam) RetryLimit(defaultLimit int) int {
	if err := p.prepareVirtualRoute(); err == nil && p.virtualReady {
		if p.virtualLimit == 0 {
			return 0
		}
		return p.virtualLimit - 1
	}
	return defaultLimit
}

func (p *RetryParam) prepareVirtualRoute() error {
	if p.virtualReady {
		return p.virtualErr
	}
	route := operation_setting.GetVirtualModelRoute(p.ModelName)
	if !route.HasPool() {
		return nil
	}

	p.virtualReady = true
	if p.preparedRoute == nil {
		p.preparedRoute = &VirtualPoolPreparedRoute{
			ModelName:            p.ModelName,
			RequestPath:          p.RequestPath,
			TokenGroup:           p.TokenGroup,
			RequiredCandidateKey: p.requiredCandidateKey,
		}
	}
	groups := []string{p.TokenGroup}
	if p.TokenGroup == "auto" {
		userGroup := common.GetContextKeyString(p.Ctx, constant.ContextKeyUserGroup)
		groups = GetRequestAutoGroups(p.Ctx, userGroup)
		if len(groups) == 0 {
			p.virtualErr = errors.New("auto groups is not enabled")
			return p.virtualErr
		}
	}
	seenCandidates := make(map[string]struct{})
	for _, entry := range virtualRoutePool(route, getRequestReasoningEffort(p.Ctx)) {
		for _, group := range groups {
			channels, err := model.GetOrderedSatisfiedChannels(group, entry.model, p.RequestPath)
			if err != nil {
				p.virtualErr = err
				return err
			}
			for _, channel := range channels {
				if entry.channelId != 0 && channel.Id != entry.channelId {
					continue
				}
				keyIndex, accountIdentity, ok := virtualPoolChannelAccount(channel, p.virtualSession)
				if !ok {
					continue
				}
				attemptIdentity := strings.Join([]string{
					accountIdentity,
					strings.ToLower(strings.TrimSpace(entry.model)),
					strings.ToLower(strings.TrimSpace(entry.reasoningEffort)),
					strings.ToLower(strings.TrimSpace(group)),
				}, "|")
				if _, exists := seenCandidates[attemptIdentity]; exists {
					continue
				}
				seenCandidates[attemptIdentity] = struct{}{}
				finalMappedModel := resolveVirtualModelMapping(entry.model, channel.GetModelMapping())
				candidate := VirtualPoolCandidate{
					Channel:          channel,
					VirtualModel:     p.ModelName,
					UpstreamModel:    entry.model,
					FinalMappedModel: finalMappedModel,
					ReasoningEffort:  entry.reasoningEffort,
					Group:            group,
					KeyIndex:         keyIndex,
					AccountIdentity:  accountIdentity,
					Capacity:         entry.capacity,
					Weight:           entry.weight,
					CapacityGroup:    entry.capacityGroup,
				}
				p.virtualRoute = append(p.virtualRoute, virtualRouteCandidate{
					channel:          channel,
					virtualModel:     p.ModelName,
					upstreamModel:    entry.model,
					finalMappedModel: finalMappedModel,
					reasoningEffort:  entry.reasoningEffort,
					group:            group,
					keyIndex:         keyIndex,
					accountIdentity:  accountIdentity,
					capacity:         entry.capacity,
					weight:           entry.weight,
					capacityGroup:    entry.capacityGroup,
				})
				p.preparedRoute.Candidates = append(p.preparedRoute.Candidates, candidate)
			}
		}
	}
	if route.Health.Enabled {
		reordered, healthy, err := moveCoolingCandidatesLast(serviceContext(p.Ctx), p.virtualRoute, route.Health.DisableModel)
		if err != nil {
			p.virtualErr = err
			return err
		}
		p.virtualRoute = reordered
		p.virtualHealthy = healthy
		if len(p.preparedRoute.Candidates) > 0 {
			reordered := make([]VirtualPoolCandidate, 0, len(p.virtualRoute))
			for _, candidate := range p.virtualRoute {
				for _, prepared := range p.preparedRoute.Candidates {
					if prepared.AttemptKey() == candidate.attemptKey() {
						reordered = append(reordered, prepared)
						break
					}
				}
			}
			if len(reordered) == len(p.virtualRoute) {
				p.preparedRoute.Candidates = reordered
			}
		}
	} else {
		p.virtualHealthy = 0
	}
	p.virtualLimit = len(p.virtualRoute)
	if route.MaxAttempts > 0 && route.MaxAttempts < p.virtualLimit {
		p.virtualLimit = route.MaxAttempts
	}
	p.preparedRoute.Limit = p.virtualLimit
	p.preparedRoute.Healthy = p.virtualHealthy
	startScope := p.virtualLimit
	if p.virtualHealthy > 0 {
		startScope = p.virtualHealthy
	}
	p.virtualOffset = virtualRouteStartIndex(p.ModelName, route.RotationMode(), startScope)
	p.preparedRoute.Offset = p.virtualOffset
	p.preparedRoute.CapacityEnabled = VirtualRouteCapacityEnabled(route)
	if err := p.prepareVirtualPoolScheduling(); err != nil {
		return err
	}
	return nil
}

func resolveVirtualModelMapping(modelName string, modelMapping string) string {
	current := strings.TrimSpace(modelName)
	if current == "" || strings.TrimSpace(modelMapping) == "" || strings.TrimSpace(modelMapping) == "{}" {
		return strings.ToLower(current)
	}
	mapping := make(map[string]string)
	if err := common.UnmarshalJsonStr(modelMapping, &mapping); err != nil {
		return strings.ToLower(current)
	}
	visited := map[string]struct{}{current: {}}
	for {
		next := strings.TrimSpace(mapping[current])
		if next == "" {
			return strings.ToLower(current)
		}
		if _, exists := visited[next]; exists {
			return strings.ToLower(current)
		}
		visited[next] = struct{}{}
		current = next
	}
}

func (p *RetryParam) prepareVirtualPoolScheduling() error {
	if p == nil || p.preparedRoute == nil || len(p.preparedRoute.Candidates) == 0 {
		return nil
	}
	setting := operation_setting.GetModelRetryPolicySetting().VirtualPoolSticky.Normalize()
	if !setting.Enabled && !p.preparedRoute.CapacityEnabled {
		return nil
	}
	if err := p.PrepareVirtualPoolAttempt(p.Ctx); err != nil {
		return err
	}
	p.preparedRoute.Scheduled = p.scheduled
	p.preparedRoute.scheduler = p.scheduler
	p.preparedRoute.owner = p.owner
	return nil
}

func (p *RetryParam) ConfirmVirtualPoolAttempt(c *gin.Context) {
	if p == nil || p.scheduled == nil || p.scheduler == nil || p.scheduled.LeaseLost() {
		return
	}
	_ = p.scheduler.Confirm(serviceContext(c), p.scheduled, time.Now())
}

// VirtualPoolLeaseLost reports whether background renewal lost ownership while
// the current attempt was running. Once true, the attempt must be treated as
// unknown and must not be confirmed or replayed.
func (p *RetryParam) VirtualPoolLeaseLost() bool {
	return p != nil && p.scheduled != nil && p.scheduled.LeaseLost()
}

func (p *RetryParam) ReleaseVirtualPoolAttempt(c *gin.Context, failed bool) {
	if p == nil || p.scheduled == nil || p.scheduler == nil {
		return
	}
	scheduled := p.scheduled
	p.scheduled = nil
	p.scheduledTaken = false
	if p.preparedRoute != nil && p.preparedRoute.Scheduled == scheduled {
		p.preparedRoute.Scheduled = nil
		p.preparedRoute.scheduler = nil
		p.preparedRoute.owner = ""
	}
	defer ClearVirtualPoolAttemptContext(c)
	if failed {
		_ = p.scheduler.Abort(serviceContext(c), scheduled, time.Now())
		return
	}
	_ = p.scheduler.Release(serviceContext(c), scheduled)
}

// PrepareVirtualPoolAttempt obtains one capacity lease and binding for the
// next attempt. The initial attempt may already have been prepared by the
// middleware; later retries call this again after releasing the failed one.
func (p *RetryParam) PrepareVirtualPoolAttempt(c *gin.Context) error {
	if p == nil || p.preparedRoute == nil || len(p.preparedRoute.Candidates) == 0 {
		return nil
	}
	if p.scheduled != nil && !p.scheduledTaken {
		if !p.scheduled.LeaseLost() {
			return nil
		}
		_ = p.scheduler.Abort(serviceContext(c), p.scheduled, time.Now())
		p.scheduled = nil
		ClearVirtualPoolAttemptContext(c)
	}
	setting := operation_setting.GetModelRetryPolicySetting().VirtualPoolSticky.Normalize()
	if !setting.Enabled && !p.preparedRoute.CapacityEnabled {
		return nil
	}
	bindings, capacity, err := virtualPoolStores(setting)
	if err != nil {
		return err
	}
	scheduler := NewVirtualPoolScheduler(bindings, capacity)
	scheduler.confirmedTTL = time.Duration(setting.ConfirmedTTLSeconds) * time.Second
	owner := fmt.Sprintf("attempt:%d:%d", p.GetRetry(), time.Now().UnixNano())
	if p.virtualSession != nil {
		owner = fmt.Sprintf("%s:%d:%d", p.virtualSession.SessionDigest, p.GetRetry(), time.Now().UnixNano())
	}
	available := make([]VirtualPoolCandidate, 0, len(p.preparedRoute.Candidates))
	for _, candidate := range p.preparedRoute.Candidates {
		if p.requiredCandidateKey != "" && candidate.AttemptKey() != p.requiredCandidateKey {
			continue
		}
		if _, attempted := p.attempted[candidate.AttemptKey()]; attempted {
			continue
		}
		available = append(available, candidate)
	}
	if len(available) == 0 {
		if p.requiredCandidateKey != "" {
			return ErrVirtualPoolResponseOwnerUnknown
		}
		return nil
	}
	waitDeadline := time.Now().Add(time.Duration(setting.CapacityWaitMillis) * time.Millisecond)
	for {
		remaining := time.Until(waitDeadline)
		if remaining <= 0 {
			break
		}
		claimWait := time.Duration(setting.ClaimWaitMillis) * time.Millisecond
		if claimWait > remaining {
			claimWait = remaining
		}
		options := VirtualPoolSchedulerOptions{
			PendingTTL:        time.Duration(setting.PendingLeaseSeconds) * time.Second,
			ConfirmedTTL:      time.Duration(setting.ConfirmedTTLSeconds) * time.Second,
			CapacityLease:     time.Duration(setting.CapacityLeaseSeconds) * time.Second,
			ClaimWait:         claimWait,
			CapacityWait:      remaining,
			PendingRenewEvery: time.Duration(setting.PendingRenewSeconds) * time.Second,
			BusyEscape:        setting.BusyEscape,
			CandidateOffset:   p.preparedRoute.Offset,
		}
		var scheduled *VirtualPoolScheduledCandidate
		var selectErr error
		if p.virtualSession == nil {
			scheduled, selectErr = scheduler.SelectCapacityWithOptions(
				serviceContext(c),
				owner,
				available,
				time.Now(),
				options,
			)
		} else {
			scheduled, selectErr = scheduler.SelectWithOptions(
				serviceContext(c),
				p.virtualSession.CacheKey,
				owner,
				available,
				time.Now(),
				options,
			)
		}
		if selectErr != nil {
			return selectErr
		}
		if scheduled != nil {
			attemptCtx, cancelAttempt := context.WithCancelCause(serviceContext(c))
			scheduled.setCancelAttempt(cancelAttempt)
			SetVirtualPoolAttemptContext(c, attemptCtx)
			p.scheduled = scheduled
			p.scheduledTaken = false
			p.scheduler = scheduler
			p.owner = owner
			p.preparedRoute.Scheduled = scheduled
			p.preparedRoute.scheduler = scheduler
			p.preparedRoute.owner = owner
			return nil
		}
	}
	return errVirtualPoolCapacityExhausted
}

// CleanupPreparedVirtualPoolAttempt releases a scheduled attempt that never
// reached the controller's normal settlement path. It is safe to call after a
// successful attempt because ReleaseVirtualPoolAttempt clears the prepared
// route reference first.
func (p *RetryParam) CleanupPreparedVirtualPoolAttempt(c *gin.Context) {
	if p == nil || p.preparedRoute == nil || p.preparedRoute.Scheduled == nil || p.preparedRoute.scheduler == nil {
		return
	}
	scheduled := p.preparedRoute.Scheduled
	scheduler := p.preparedRoute.scheduler
	p.preparedRoute.Scheduled = nil
	p.preparedRoute.scheduler = nil
	p.preparedRoute.owner = ""
	_ = scheduler.Abort(serviceContext(c), scheduled, time.Now())
}

func serviceContext(c *gin.Context) context.Context {
	if c != nil && c.Request != nil {
		return c.Request.Context()
	}
	return context.Background()
}

func virtualPoolStores(setting operation_setting.VirtualPoolStickySetting) (VirtualPoolBindingStore, VirtualPoolCapacityStore, error) {
	if setting.RedisRequiredForReady && (!common.RedisEnabled || common.RDB == nil) {
		return nil, nil, ErrVirtualPoolStoreUnavailable
	}
	if setting.BindingMode == operation_setting.VirtualPoolBindingModeRedis {
		if !common.RedisEnabled || common.RDB == nil {
			return nil, nil, ErrVirtualPoolStoreUnavailable
		}
		return NewVirtualPoolRedisBindingStore(common.RDB), NewVirtualPoolRedisCapacityStore(common.RDB), nil
	}
	bindings, capacity := getVirtualPoolMemoryStores()
	return bindings, capacity, nil
}

func virtualPoolChannelAccount(channel *model.Channel, session *VirtualPoolSession) (int, string, bool) {
	if channel == nil {
		return 0, "", false
	}
	if !channel.ChannelInfo.IsMultiKey {
		return 0, strconv.Itoa(channel.Id), true
	}
	setting := operation_setting.GetModelRetryPolicySetting().VirtualPoolSticky.Normalize()
	if session == nil || setting.MultiKeyPolicy != operation_setting.VirtualPoolMultiKeyPolicyBindIndex {
		return 0, "", false
	}
	keyIndex, ok := virtualPoolSessionKeyIndex(session, channel)
	if !ok {
		return 0, "", false
	}
	if _, _, err := channel.GetEnabledKeyByIndex(keyIndex); err != nil {
		return 0, "", false
	}
	return keyIndex, fmt.Sprintf("%d:%d", channel.Id, keyIndex), true
}

func virtualPoolSessionKeyIndex(session *VirtualPoolSession, channel *model.Channel) (int, bool) {
	if session == nil || channel == nil || channel.Id <= 0 {
		return 0, false
	}
	keys := channel.GetKeys()
	if len(keys) == 0 {
		return 0, false
	}
	statusList := channel.ChannelInfo.MultiKeyStatusList
	enabledIndexes := make([]int, 0, len(keys))
	for index := range keys {
		if statusList != nil {
			if status, exists := statusList[index]; exists && status != common.ChannelStatusEnabled {
				continue
			}
		}
		enabledIndexes = append(enabledIndexes, index)
	}
	if len(enabledIndexes) == 0 {
		return 0, false
	}
	raw := common.Sha1([]byte(session.SessionDigest + "\x00" + strconv.Itoa(channel.Id)))
	var value uint64
	for index := 0; index < len(raw) && index < 16; index++ {
		nibble, err := strconv.ParseUint(raw[index:index+1], 16, 4)
		if err != nil {
			break
		}
		value = value<<4 | nibble
	}
	return enabledIndexes[int(value%uint64(len(enabledIndexes)))], true
}

// moveCoolingCandidatesLast keeps the pool order but moves entries that are
// cooling down behind the healthy ones. Strict disable mode removes disabled
// entries for the whole request instead of using them as a last resort. It
// returns how many entries stayed in the healthy part.
func moveCoolingCandidatesLast(
	ctx context.Context,
	candidates []virtualRouteCandidate,
	disableModel bool,
) ([]virtualRouteCandidate, int, error) {
	now := time.Now()
	healthy := make([]virtualRouteCandidate, 0, len(candidates))
	cooling := make([]virtualRouteCandidate, 0, len(candidates))
	store, err := virtualRouteHealthStore()
	if err != nil {
		return nil, 0, err
	}
	for _, candidate := range candidates {
		key := virtualRouteHealthKey(candidate.virtualModel, candidate.channel.Id, candidate.upstreamModel)
		cooldown, err := store.IsCoolingDown(ctx, key, now)
		if err != nil {
			return nil, 0, err
		}
		if cooldown {
			if disableModel {
				continue
			}
			cooling = append(cooling, candidate)
			continue
		}
		healthy = append(healthy, candidate)
	}
	if disableModel {
		return healthy, len(healthy), nil
	}
	return append(healthy, cooling...), len(healthy), nil
}

// virtualRouteStartIndex picks the pool position the first attempt starts at.
// Ordered routes always start at the head; random and round_robin spread the
// pool across requests.
func virtualRouteStartIndex(modelName string, rotation string, poolSize int) int {
	if poolSize <= 1 {
		return 0
	}
	switch rotation {
	case operation_setting.VirtualModelRouteRotationRandom:
		return rand.Intn(poolSize)
	case operation_setting.VirtualModelRouteRotationRoundRobin:
		counter, _ := virtualRouteRotationCounters.LoadOrStore(strings.ToLower(strings.TrimSpace(modelName)), new(uint64))
		position := atomic.AddUint64(counter.(*uint64), 1) - 1
		return int(position % uint64(poolSize))
	default:
		return 0
	}
}

func (p *RetryParam) UsesVirtualRoute() bool {
	return operation_setting.GetVirtualModelRoute(p.ModelName).HasPool()
}

// SetVirtualPoolSession attaches the request-scoped sticky identity before the
// virtual route is prepared.
func (p *RetryParam) SetVirtualPoolSession(session *VirtualPoolSession) {
	p.virtualSession = session
	if p.preparedRoute != nil {
		p.preparedRoute.Session = session
	}
}

// SetRequiredVirtualPoolCandidate pins this request to one pool candidate.
// It is used for provider-private continuation state such as
// previous_response_id, where falling back to another upstream would be wrong.
func (p *RetryParam) SetRequiredVirtualPoolCandidate(candidateKey string) {
	p.requiredCandidateKey = strings.TrimSpace(candidateKey)
	if p.preparedRoute != nil {
		p.preparedRoute.RequiredCandidateKey = p.requiredCandidateKey
	}
}

// PreparedVirtualPoolRoute returns the request-scoped candidate list, if the
// current retry parameter represents a virtual pool route.
func (p *RetryParam) PreparedVirtualPoolRoute() *VirtualPoolPreparedRoute {
	if p == nil || p.preparedRoute == nil {
		return nil
	}
	return p.preparedRoute
}

// PeekPreparedVirtualPoolCandidate returns the candidate already selected by
// middleware without consuming it. The controller consumes it through
// NextPreparedVirtualPoolCandidate on the first relay attempt.
func (p *RetryParam) PeekPreparedVirtualPoolCandidate() (VirtualPoolCandidate, bool) {
	if p == nil || p.preparedRoute == nil {
		return VirtualPoolCandidate{}, false
	}
	if p.scheduled != nil {
		return p.scheduled.Candidate, true
	}
	if len(p.preparedRoute.Candidates) == 0 || p.preparedRoute.Limit <= 0 {
		return VirtualPoolCandidate{}, false
	}
	index := p.preparedRoute.Offset
	if p.preparedRoute.Healthy > 0 && p.preparedRoute.Healthy < len(p.preparedRoute.Candidates) {
		index %= p.preparedRoute.Healthy
	} else if len(p.preparedRoute.Candidates) > 0 {
		index %= len(p.preparedRoute.Candidates)
	}
	return p.preparedRoute.Candidates[index], true
}

func (p *RetryParam) PeekPreparedVirtualPoolScheduled() (*VirtualPoolScheduledCandidate, bool) {
	if p == nil || p.preparedRoute == nil || p.preparedRoute.Scheduled == nil {
		return nil, false
	}
	return p.preparedRoute.Scheduled, true
}

// NextPreparedVirtualPoolCandidate advances through the prepared route using
// the attempt set and current retry index.
func (p *RetryParam) NextPreparedVirtualPoolCandidate() (VirtualPoolCandidate, bool) {
	if p == nil || p.preparedRoute == nil {
		return VirtualPoolCandidate{}, false
	}
	if len(p.virtualRoute) == 0 {
		for _, candidate := range p.preparedRoute.Candidates {
			p.virtualRoute = append(p.virtualRoute, virtualRouteCandidate{
				channel:          candidate.Channel,
				virtualModel:     candidate.VirtualModel,
				upstreamModel:    candidate.UpstreamModel,
				finalMappedModel: candidate.FinalMappedModel,
				reasoningEffort:  candidate.ReasoningEffort,
				group:            candidate.Group,
				keyIndex:         candidate.KeyIndex,
				accountIdentity:  candidate.AccountIdentity,
				capacity:         candidate.Capacity,
				weight:           candidate.Weight,
				capacityGroup:    candidate.CapacityGroup,
			})
		}
		p.virtualLimit = p.preparedRoute.Limit
		p.virtualHealthy = p.preparedRoute.Healthy
		p.virtualOffset = p.preparedRoute.Offset
	}
	if p.scheduled != nil && !p.scheduledTaken {
		p.scheduledTaken = true
		selected := p.scheduled.Candidate
		if p.attempted == nil {
			p.attempted = make(map[string]struct{}, p.virtualLimit)
		}
		p.attempted[selected.AttemptKey()] = struct{}{}
		return selected, true
	}
	selected, ok := p.nextVirtualCandidate()
	if !ok {
		return VirtualPoolCandidate{}, false
	}
	return VirtualPoolCandidate{
		Channel:          selected.channel,
		VirtualModel:     selected.virtualModel,
		UpstreamModel:    selected.upstreamModel,
		FinalMappedModel: selected.finalMappedModel,
		ReasoningEffort:  selected.reasoningEffort,
		Group:            selected.group,
		KeyIndex:         selected.keyIndex,
		AccountIdentity:  selected.accountIdentity,
		Capacity:         selected.capacity,
		Weight:           selected.weight,
		CapacityGroup:    selected.capacityGroup,
	}, true
}

// AdoptPreparedVirtualPoolRoute reuses a route prepared by middleware and does
// not advance rotation or rebuild candidates.
func (p *RetryParam) AdoptPreparedVirtualPoolRoute(prepared *VirtualPoolPreparedRoute) {
	if p == nil || prepared == nil {
		return
	}
	p.preparedRoute = prepared
	p.virtualSession = prepared.Session
	p.requiredCandidateKey = prepared.RequiredCandidateKey
	p.scheduled = prepared.Scheduled
	p.scheduledTaken = false
	p.scheduler = prepared.scheduler
	p.owner = prepared.owner
	p.virtualReady = true
	p.virtualRoute = make([]virtualRouteCandidate, 0, len(prepared.Candidates))
	for _, candidate := range prepared.Candidates {
		p.virtualRoute = append(p.virtualRoute, virtualRouteCandidate{
			channel:          candidate.Channel,
			virtualModel:     candidate.VirtualModel,
			upstreamModel:    candidate.UpstreamModel,
			finalMappedModel: candidate.FinalMappedModel,
			reasoningEffort:  candidate.ReasoningEffort,
			group:            candidate.Group,
			keyIndex:         candidate.KeyIndex,
			accountIdentity:  candidate.AccountIdentity,
			capacity:         candidate.Capacity,
			weight:           candidate.Weight,
			capacityGroup:    candidate.CapacityGroup,
		})
	}
	p.virtualLimit = prepared.Limit
	p.virtualHealthy = prepared.Healthy
	p.virtualOffset = prepared.Offset
}

func (p *RetryParam) getVirtualRouteChannel() (*model.Channel, string, bool, error) {
	if err := p.prepareVirtualRoute(); err != nil {
		return nil, p.TokenGroup, true, err
	}
	if !p.virtualReady {
		return nil, p.TokenGroup, false, nil
	}
	candidate, ok := p.nextVirtualRouteChannelCandidate()
	if !ok {
		if p.requiredCandidateKey != "" {
			return nil, p.TokenGroup, true, ErrVirtualPoolResponseOwnerUnknown
		}
		return nil, p.TokenGroup, true, model.ErrPriorityFallbackExhausted
	}
	common.SetContextKey(p.Ctx, constant.ContextKeyVirtualUpstreamModel, candidate.upstreamModel)
	common.SetContextKey(p.Ctx, constant.ContextKeyVirtualReasoningEffort, candidate.reasoningEffort)
	if p.TokenGroup == "auto" {
		common.SetContextKey(p.Ctx, constant.ContextKeyAutoGroup, candidate.group)
	}
	return candidate.channel, candidate.group, true, nil
}

func (p *RetryParam) nextVirtualRouteChannelCandidate() (virtualRouteCandidate, bool) {
	if p.scheduled != nil && !p.scheduledTaken {
		p.scheduledTaken = true
		selected := p.scheduled.Candidate
		if p.attempted == nil {
			p.attempted = make(map[string]struct{}, p.virtualLimit)
		}
		p.attempted[selected.AttemptKey()] = struct{}{}
		return virtualRouteCandidate{
			channel:          selected.Channel,
			virtualModel:     selected.VirtualModel,
			upstreamModel:    selected.UpstreamModel,
			finalMappedModel: selected.FinalMappedModel,
			reasoningEffort:  selected.ReasoningEffort,
			group:            selected.Group,
			keyIndex:         selected.KeyIndex,
			accountIdentity:  selected.AccountIdentity,
			capacity:         selected.Capacity,
			weight:           selected.Weight,
			capacityGroup:    selected.CapacityGroup,
		}, true
	}
	return p.nextVirtualCandidate()
}

func (p *RetryParam) nextVirtualCandidate() (virtualRouteCandidate, bool) {
	if len(p.virtualRoute) == 0 || p.virtualLimit <= 0 {
		return virtualRouteCandidate{}, false
	}
	if p.attempted == nil {
		p.attempted = make(map[string]struct{}, p.virtualLimit)
	}
	for checked := 0; checked < len(p.virtualRoute); checked++ {
		if len(p.attempted) >= p.virtualLimit {
			return virtualRouteCandidate{}, false
		}
		index := p.virtualCandidateIndex(p.GetRetry() + checked)
		if index < 0 || index >= len(p.virtualRoute) {
			continue
		}
		candidate := p.virtualRoute[index]
		identity := candidate.attemptKey()
		if _, exists := p.attempted[identity]; exists {
			continue
		}
		p.attempted[identity] = struct{}{}
		return candidate, true
	}
	return virtualRouteCandidate{}, false
}

func (candidate virtualRouteCandidate) attemptKey() string {
	return strings.Join([]string{
		candidate.accountIdentity,
		strings.ToLower(strings.TrimSpace(candidate.finalMappedModel)),
		strings.ToLower(strings.TrimSpace(candidate.reasoningEffort)),
		strings.ToLower(strings.TrimSpace(candidate.group)),
	}, "|")
}

// virtualCandidateIndex walks the healthy part of the pool first and only then
// the entries that are cooling down.
func (p *RetryParam) virtualCandidateIndex(retry int) int {
	if p.virtualHealthy <= 0 || p.virtualHealthy >= len(p.virtualRoute) {
		return (p.virtualOffset + retry) % len(p.virtualRoute)
	}
	if retry < p.virtualHealthy {
		return (p.virtualOffset + retry) % p.virtualHealthy
	}
	return retry
}

func getRequestReasoningEffort(c *gin.Context) string {
	if c == nil || c.Request == nil || !strings.HasPrefix(c.Request.Header.Get("Content-Type"), "application/json") {
		return ""
	}
	var body struct {
		Reasoning *struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if err := common.UnmarshalBodyReusable(c, &body); err != nil {
		return ""
	}
	if body.Reasoning != nil && strings.TrimSpace(body.Reasoning.Effort) != "" {
		return strings.ToLower(strings.TrimSpace(body.Reasoning.Effort))
	}
	return strings.ToLower(strings.TrimSpace(body.ReasoningEffort))
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// 尝试获取一个满足要求的随机渠道。
//
// For "auto" tokenGroup with cross-group Retry enabled:
// 对于启用了跨分组重试的 "auto" tokenGroup：
//
//   - Each group will exhaust all its priorities before moving to the next group.
//     每个分组会用完所有优先级后才会切换到下一个分组。
//
//   - Uses ContextKeyAutoGroupIndex to track current group index.
//     使用 ContextKeyAutoGroupIndex 跟踪当前分组索引。
//
//   - Uses ContextKeyAutoGroupRetryIndex to track the global Retry count when current group started.
//     使用 ContextKeyAutoGroupRetryIndex 跟踪当前分组开始时的全局重试次数。
//
//   - priorityRetry = Retry - startRetryIndex, represents the priority level within current group.
//     priorityRetry = Retry - startRetryIndex，表示当前分组内的优先级级别。
//
//   - When GetRandomSatisfiedChannel returns nil (priorities exhausted), moves to next group.
//     当 GetRandomSatisfiedChannel 返回 nil（优先级用完）时，切换到下一个分组。
//
// Example flow (2 groups, each with 2 priorities, RetryTimes=3):
// 示例流程（2个分组，每个有2个优先级，RetryTimes=3）：
//
//	Retry=0: GroupA, priority0 (startRetryIndex=0, priorityRetry=0)
//	         分组A, 优先级0
//
//	Retry=1: GroupA, priority1 (startRetryIndex=0, priorityRetry=1)
//	         分组A, 优先级1
//
//	Retry=2: GroupA exhausted → GroupB, priority0 (startRetryIndex=2, priorityRetry=0)
//	         分组A用完 → 分组B, 优先级0
//
//	Retry=3: GroupB, priority1 (startRetryIndex=2, priorityRetry=1)
//	         分组B, 优先级1
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)

	if virtualChannel, virtualGroup, handled, virtualErr := param.getVirtualRouteChannel(); handled {
		return virtualChannel, virtualGroup, virtualErr
	}

	if param.TokenGroup == "auto" {
		autoGroups := GetRequestAutoGroups(param.Ctx, userGroup)
		if len(autoGroups) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}

		// startGroupIndex: the group index to start searching from
		// startGroupIndex: 开始搜索的分组索引
		startGroupIndex := 0
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)

		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}

		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			// Calculate priorityRetry for current group
			// 计算当前分组的 priorityRetry
			priorityRetry := param.GetRetry()
			// If moved to a new group, reset priorityRetry and update startRetryIndex
			// 如果切换到新分组，重置 priorityRetry 并更新 startRetryIndex
			if i > startGroupIndex {
				priorityRetry = 0
			}
			logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)

			var selectErr error
			channel, selectErr = selectSatisfiedChannelWithModelHealth(param, autoGroup, priorityRetry)
			if selectErr != nil {
				return nil, autoGroup, selectErr
			}
			if channel == nil {
				// Current group has no available channel for this model, try next group
				// 当前分组没有该模型的可用渠道，尝试下一个分组
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
				// 重置状态以尝试下一个分组
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, 0)
				// Reset retry counter so outer loop can continue for next group
				// 重置重试计数器，以便外层循环可以为下一个分组继续
				param.SetRetry(0)
				continue
			}
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			selectGroup = autoGroup
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

			// Prepare state for next retry
			// 为下一次重试准备状态
			if crossGroupRetry && priorityRetry >= common.RetryTimes {
				// Current group has exhausted all retries, prepare to switch to next group
				// This request still uses current group, but next retry will use next group
				// 当前分组已用完所有重试次数，准备切换到下一个分组
				// 本次请求仍使用当前分组，但下次重试将使用下一个分组
				logger.LogDebug(param.Ctx, "Current group %s retries exhausted (priorityRetry=%d >= RetryTimes=%d), preparing switch to next group for next retry", autoGroup, priorityRetry, common.RetryTimes)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				// Reset retry counter so outer loop can continue for next group
				// 重置重试计数器，以便外层循环可以为下一个分组继续
				param.SetRetry(0)
				param.ResetRetryNextTry()
			} else {
				// Stay in current group, save current state
				// 保持在当前分组，保存当前状态
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			}
			break
		}
	} else {
		channel, err = selectSatisfiedChannelWithModelHealth(param, param.TokenGroup, param.GetRetry())
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	return channel, selectGroup, nil
}

func selectSatisfiedChannelWithModelHealth(param *RetryParam, group string, retry int) (*model.Channel, error) {
	if param == nil {
		return nil, nil
	}
	if _, enabled := operation_setting.MatchModelHealthPolicy(param.ModelName, group); !enabled {
		return model.GetRandomSatisfiedChannel(group, param.ModelName, retry, param.RequestPath)
	}
	channels, err := model.GetSatisfiedChannelsInPriorityOrder(group, param.ModelName, param.RequestPath)
	if err != nil {
		return nil, err
	}
	channels, err = FilterModelHealthCandidates(channels, param.ModelName, group)
	if err != nil {
		return nil, err
	}
	// Once a cooldown is active the generic filter above already removed the
	// failing channel. Between threshold and cooldown there is a window where
	// the model is still selectable, and a same-priority retry would otherwise
	// be able to pick the channel that just failed again. Mirror the virtual
	// route candidate walk: retries take the first untried entry of the
	// priority-ordered list, so a healthy same-tier sibling is preferred over
	// descending to the next priority level. When every entry was already
	// attempted in this request, fall back to the normal tier/weight selection.
	if len(param.attemptedChannelIDs) > 0 {
		for _, channel := range channels {
			if _, attempted := param.attemptedChannelIDs[channel.Id]; !attempted {
				return channel, nil
			}
		}
	}
	return model.SelectSatisfiedChannelFromCandidates(channels, retry)
}
