package service

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
)

const virtualPoolCapacityRedisPrefix = "new-api:virtual-sticky:capacity:v1:"

var (
	errVirtualPoolCapacityExhausted = errors.New("virtual pool capacity is exhausted")
	ErrVirtualPoolLeaseLost         = errors.New("virtual pool lease was lost during the attempt")
)

const virtualPoolCapacityAcquireScript = `
local now = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
local generation = tonumber(redis.call('HGET', KEYS[1], 'generation') or '0')
local active = 0
local max_expiry = 0
local fields = redis.call('HKEYS', KEYS[1])
for _, field in ipairs(fields) do
  if string.sub(field, 1, 6) == 'owner:' then
    local owner_generation = string.sub(field, 7)
    local expiry = tonumber(redis.call('HGET', KEYS[1], 'expires:' .. owner_generation) or '0')
    if expiry <= now then
      redis.call('HDEL', KEYS[1], field)
      redis.call('HDEL', KEYS[1], 'expires:' .. owner_generation)
    else
      active = active + 1
      if expiry > max_expiry then
        max_expiry = expiry
      end
    end
  end
end
if active >= limit then
  return {0, generation}
end
generation = generation + 1
local expires = now + tonumber(ARGV[3])
redis.call('HSET', KEYS[1], 'generation', generation)
redis.call('HSET', KEYS[1], 'owner:' .. generation, ARGV[4])
redis.call('HSET', KEYS[1], 'expires:' .. generation, expires)
if expires > max_expiry then
  max_expiry = expires
end
redis.call('PEXPIREAT', KEYS[1], math.ceil(max_expiry * 1000))
return {1, generation}
`

const virtualPoolCapacityReleaseScript = `
local generation = tonumber(ARGV[1])
local owner_field = 'owner:' .. generation
local expiry_field = 'expires:' .. generation
if redis.call('HGET', KEYS[1], owner_field) ~= ARGV[3] or tonumber(redis.call('HGET', KEYS[1], expiry_field) or '0') <= tonumber(ARGV[2]) then
  return 0
end
redis.call('HDEL', KEYS[1], owner_field)
redis.call('HDEL', KEYS[1], expiry_field)
local max_expiry = 0
local fields = redis.call('HKEYS', KEYS[1])
for _, field in ipairs(fields) do
  if string.sub(field, 1, 6) == 'owner:' then
    local owner_generation = string.sub(field, 7)
    local expiry = tonumber(redis.call('HGET', KEYS[1], 'expires:' .. owner_generation) or '0')
    if expiry <= tonumber(ARGV[2]) then
      redis.call('HDEL', KEYS[1], field)
      redis.call('HDEL', KEYS[1], 'expires:' .. owner_generation)
    elseif expiry > max_expiry then
      max_expiry = expiry
    end
  end
end
if max_expiry == 0 then
  redis.call('PEXPIREAT', KEYS[1], math.ceil((tonumber(ARGV[2]) + 604800000) * 1000))
else
  redis.call('PEXPIREAT', KEYS[1], math.ceil(max_expiry * 1000))
end
return 1
`

const virtualPoolCapacityRenewScript = `
local generation = tonumber(ARGV[1])
local owner_field = 'owner:' .. generation
local expiry_field = 'expires:' .. generation
local now = tonumber(ARGV[2])
if redis.call('HGET', KEYS[1], owner_field) ~= ARGV[3] or tonumber(redis.call('HGET', KEYS[1], expiry_field) or '0') <= now then
  return 0
end
local expires = now + tonumber(ARGV[4])
redis.call('HSET', KEYS[1], expiry_field, expires)
redis.call('PEXPIREAT', KEYS[1], math.ceil(expires * 1000))
return 1
`

const virtualPoolCapacityActiveScript = `
local now = tonumber(ARGV[1])
local active = 0
local fields = redis.call('HKEYS', KEYS[1])
for _, field in ipairs(fields) do
  if string.sub(field, 1, 6) == 'owner:' then
    local owner_generation = string.sub(field, 7)
    local expiry = tonumber(redis.call('HGET', KEYS[1], 'expires:' .. owner_generation) or '0')
    if expiry <= now then
      redis.call('HDEL', KEYS[1], field)
      redis.call('HDEL', KEYS[1], 'expires:' .. owner_generation)
    else
      active = active + 1
    end
  end
end
return active
`

type VirtualPoolCapacityLease struct {
	Key        string
	Owner      string
	Generation int64
}

type VirtualPoolCapacityStore interface {
	Acquire(ctx context.Context, key, owner string, limit int, now time.Time, lease time.Duration) (VirtualPoolCapacityLease, bool, error)
	Active(ctx context.Context, key string, now time.Time) (int, error)
	Renew(ctx context.Context, key, owner string, generation int64, now time.Time, lease time.Duration) bool
	Release(ctx context.Context, key, owner string, generation int64, now time.Time) bool
}

type VirtualPoolMemoryCapacityStore struct {
	mu      sync.Mutex
	entries map[string]virtualPoolMemoryCapacity
}

type virtualPoolMemoryCapacity struct {
	generation int64
	owners     map[int64]virtualPoolMemoryCapacityOwner
}

type virtualPoolMemoryCapacityOwner struct {
	owner     string
	expiresAt time.Time
}

func (capacity *virtualPoolMemoryCapacity) pruneExpired(now time.Time) {
	for generation, owner := range capacity.owners {
		if !owner.expiresAt.After(now) {
			delete(capacity.owners, generation)
		}
	}
}

func NewVirtualPoolMemoryCapacityStore() *VirtualPoolMemoryCapacityStore {
	return &VirtualPoolMemoryCapacityStore{entries: make(map[string]virtualPoolMemoryCapacity)}
}

func (store *VirtualPoolMemoryCapacityStore) Acquire(
	_ context.Context,
	key string,
	owner string,
	limit int,
	now time.Time,
	lease time.Duration,
) (VirtualPoolCapacityLease, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if limit <= 0 {
		return VirtualPoolCapacityLease{}, false, nil
	}
	current, exists := store.entries[key]
	if !exists {
		current = virtualPoolMemoryCapacity{owners: make(map[int64]virtualPoolMemoryCapacityOwner)}
	}
	current.pruneExpired(now)
	if len(current.owners) >= limit {
		return VirtualPoolCapacityLease{}, false, nil
	}
	current.generation++
	if current.owners == nil {
		current.owners = make(map[int64]virtualPoolMemoryCapacityOwner)
	}
	current.owners[current.generation] = virtualPoolMemoryCapacityOwner{
		owner:     owner,
		expiresAt: now.Add(lease),
	}
	store.entries[key] = current
	return VirtualPoolCapacityLease{Key: key, Owner: owner, Generation: current.generation}, true, nil
}

func (store *VirtualPoolMemoryCapacityStore) Release(
	_ context.Context,
	key string,
	owner string,
	generation int64,
	now time.Time,
) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	current, exists := store.entries[key]
	if !exists {
		return false
	}
	current.pruneExpired(now)
	currentOwner, exists := current.owners[generation]
	if !exists || currentOwner.owner != owner {
		return false
	}
	delete(current.owners, generation)
	store.entries[key] = current
	return true
}

func (store *VirtualPoolMemoryCapacityStore) Active(
	_ context.Context,
	key string,
	now time.Time,
) (int, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	current, exists := store.entries[key]
	if !exists {
		return 0, nil
	}
	current.pruneExpired(now)
	store.entries[key] = current
	return len(current.owners), nil
}

func (store *VirtualPoolMemoryCapacityStore) Renew(
	_ context.Context,
	key string,
	owner string,
	generation int64,
	now time.Time,
	lease time.Duration,
) bool {
	store.mu.Lock()
	defer store.mu.Unlock()

	current, exists := store.entries[key]
	if !exists {
		return false
	}
	current.pruneExpired(now)
	currentOwner, exists := current.owners[generation]
	if !exists || currentOwner.owner != owner {
		return false
	}
	currentOwner.expiresAt = now.Add(lease)
	current.owners[generation] = currentOwner
	store.entries[key] = current
	return true
}

type VirtualPoolRedisCapacityStore struct {
	client *redis.Client
}

func NewVirtualPoolRedisCapacityStore(client *redis.Client) *VirtualPoolRedisCapacityStore {
	return &VirtualPoolRedisCapacityStore{client: client}
}

func (store *VirtualPoolRedisCapacityStore) Acquire(
	ctx context.Context,
	key string,
	owner string,
	limit int,
	now time.Time,
	lease time.Duration,
) (VirtualPoolCapacityLease, bool, error) {
	if store == nil || store.client == nil {
		return VirtualPoolCapacityLease{}, false, ErrVirtualPoolStoreUnavailable
	}
	if limit <= 0 {
		return VirtualPoolCapacityLease{}, false, nil
	}
	result, err := store.client.Eval(ctx, virtualPoolCapacityAcquireScript, []string{store.key(key)},
		now.UnixMilli(),
		limit,
		lease.Milliseconds(),
		owner,
	).Result()
	if err != nil {
		return VirtualPoolCapacityLease{}, false, err
	}
	values, err := redisStringSlice(result)
	if err != nil || len(values) < 2 {
		return VirtualPoolCapacityLease{}, false, ErrVirtualPoolStoreUnavailable
	}
	acquired, _ := strconv.ParseInt(values[0], 10, 64)
	generation, _ := strconv.ParseInt(values[1], 10, 64)
	return VirtualPoolCapacityLease{Key: key, Owner: owner, Generation: generation}, acquired == 1, nil
}

func (store *VirtualPoolRedisCapacityStore) Release(
	ctx context.Context,
	key string,
	owner string,
	generation int64,
	now time.Time,
) bool {
	if store == nil || store.client == nil {
		return false
	}
	result, err := store.client.Eval(ctx, virtualPoolCapacityReleaseScript, []string{store.key(key)},
		generation,
		now.UnixMilli(),
		owner,
	).Int64()
	return err == nil && result == 1
}

func (store *VirtualPoolRedisCapacityStore) Active(
	ctx context.Context,
	key string,
	now time.Time,
) (int, error) {
	if store == nil || store.client == nil {
		return 0, ErrVirtualPoolStoreUnavailable
	}
	result, err := store.client.Eval(ctx, virtualPoolCapacityActiveScript, []string{store.key(key)}, now.UnixMilli()).Int64()
	if err != nil {
		return 0, err
	}
	return int(result), nil
}

func (store *VirtualPoolRedisCapacityStore) Renew(
	ctx context.Context,
	key string,
	owner string,
	generation int64,
	now time.Time,
	lease time.Duration,
) bool {
	if store == nil || store.client == nil {
		return false
	}
	result, err := store.client.Eval(ctx, virtualPoolCapacityRenewScript, []string{store.key(key)},
		generation,
		now.UnixMilli(),
		owner,
		lease.Milliseconds(),
	).Int64()
	return err == nil && result == 1
}

func (store *VirtualPoolRedisCapacityStore) key(key string) string {
	return virtualPoolCapacityRedisPrefix + strings.TrimSpace(key)
}

type VirtualPoolScheduledCandidate struct {
	SessionKey        string
	Candidate         VirtualPoolCandidate
	Binding           VirtualPoolBinding
	CapacityLease     VirtualPoolCapacityLease
	CapacityKey       string
	CapacityAcquired  bool
	PendingTTL        time.Duration
	ConfirmedTTL      time.Duration
	CapacityLeaseTTL  time.Duration
	PendingRenewEvery time.Duration
	stopRenewal       chan struct{}
	renewalDone       chan struct{}
	attemptMu         sync.Mutex
	cancelAttempt     context.CancelCauseFunc
	leaseLost         atomic.Bool
}

type VirtualPoolScheduler struct {
	bindings     VirtualPoolBindingStore
	capacity     VirtualPoolCapacityStore
	confirmedTTL time.Duration
}

// LeaseLost reports whether background renewal lost either the binding or the
// capacity lease. The attempt context is cancelled at the same time so an
// in-flight upstream request stops rather than outliving its ownership.
func (scheduled *VirtualPoolScheduledCandidate) LeaseLost() bool {
	return scheduled != nil && scheduled.leaseLost.Load()
}

func (scheduled *VirtualPoolScheduledCandidate) markLeaseLost() {
	if scheduled == nil || scheduled.leaseLost.Swap(true) {
		return
	}
	scheduled.attemptMu.Lock()
	defer scheduled.attemptMu.Unlock()
	if scheduled.cancelAttempt != nil {
		scheduled.cancelAttempt(ErrVirtualPoolLeaseLost)
	}
}

func (scheduled *VirtualPoolScheduledCandidate) setCancelAttempt(cancel context.CancelCauseFunc) {
	if scheduled == nil {
		return
	}
	scheduled.attemptMu.Lock()
	defer scheduled.attemptMu.Unlock()
	scheduled.cancelAttempt = cancel
	if scheduled.leaseLost.Load() && cancel != nil {
		cancel(ErrVirtualPoolLeaseLost)
	}
}

var (
	virtualPoolMemoryStoresOnce    sync.Once
	virtualPoolMemoryBindings      *VirtualPoolMemoryBindingStore
	virtualPoolMemoryCapacityStore *VirtualPoolMemoryCapacityStore
)

func getVirtualPoolMemoryStores() (*VirtualPoolMemoryBindingStore, *VirtualPoolMemoryCapacityStore) {
	virtualPoolMemoryStoresOnce.Do(func() {
		virtualPoolMemoryBindings = NewVirtualPoolMemoryBindingStore()
		virtualPoolMemoryCapacityStore = NewVirtualPoolMemoryCapacityStore()
	})
	return virtualPoolMemoryBindings, virtualPoolMemoryCapacityStore
}

type VirtualPoolSchedulerOptions struct {
	PendingTTL        time.Duration
	ConfirmedTTL      time.Duration
	CapacityLease     time.Duration
	ClaimWait         time.Duration
	PendingRenewEvery time.Duration
	BusyEscape        bool
}

func NewVirtualPoolScheduler(bindings VirtualPoolBindingStore, capacity VirtualPoolCapacityStore) *VirtualPoolScheduler {
	return &VirtualPoolScheduler{bindings: bindings, capacity: capacity, confirmedTTL: time.Hour}
}

func (scheduler *VirtualPoolScheduler) Select(
	ctx context.Context,
	sessionKey string,
	owner string,
	candidates []VirtualPoolCandidate,
	now time.Time,
) (*VirtualPoolScheduledCandidate, error) {
	return scheduler.SelectWithOptions(ctx, sessionKey, owner, candidates, now, VirtualPoolSchedulerOptions{
		PendingTTL:    time.Minute,
		ConfirmedTTL:  time.Hour,
		CapacityLease: time.Hour,
	})
}

func (scheduler *VirtualPoolScheduler) SelectWithOptions(
	ctx context.Context,
	sessionKey string,
	owner string,
	candidates []VirtualPoolCandidate,
	now time.Time,
	options VirtualPoolSchedulerOptions,
) (*VirtualPoolScheduledCandidate, error) {
	if scheduler == nil || scheduler.bindings == nil || scheduler.capacity == nil || len(candidates) == 0 {
		return nil, nil
	}
	deadline := now
	if options.ClaimWait > 0 {
		deadline = now.Add(options.ClaimWait)
	}
	for {
		scheduled, err := scheduler.selectOnce(ctx, sessionKey, owner, candidates, now, options)
		if err != nil || scheduled != nil || options.ClaimWait <= 0 || !now.Before(deadline) {
			return scheduled, err
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		}
		now = time.Now()
	}
}

func (scheduler *VirtualPoolScheduler) selectOnce(
	ctx context.Context,
	sessionKey string,
	owner string,
	candidates []VirtualPoolCandidate,
	now time.Time,
	options VirtualPoolSchedulerOptions,
) (*VirtualPoolScheduledCandidate, error) {
	if options.PendingTTL <= 0 {
		options.PendingTTL = time.Minute
	}
	if options.ConfirmedTTL <= 0 {
		options.ConfirmedTTL = time.Hour
	}
	if options.CapacityLease <= 0 {
		options.CapacityLease = time.Hour
	}
	ordered, err := scheduler.candidatesByLoad(ctx, candidates, now)
	if err != nil {
		return nil, err
	}
	for _, claimedCandidate := range ordered {
		candidate := claimedCandidate.candidate
		binding, claimed, err := scheduler.bindings.Claim(ctx, sessionKey, candidate.AttemptKey(), owner, now, options.PendingTTL, options.ConfirmedTTL)
		if err != nil {
			return nil, err
		}
		if !claimed {
			for _, boundCandidate := range ordered {
				if boundCandidate.candidate.AttemptKey() != binding.CandidateKey {
					continue
				}
				existing := boundCandidate.candidate
				capacityKey := virtualPoolCapacityKey(existing)
				lease, acquired, acquireErr := scheduler.capacity.Acquire(ctx, capacityKey, owner, virtualPoolCandidateCapacity(existing), now, options.CapacityLease)
				if acquireErr != nil {
					return nil, acquireErr
				}
				if !acquired {
					if options.BusyEscape && scheduler.bindings.Invalidate(ctx, sessionKey, binding.Generation, now) {
						return scheduler.selectOnce(ctx, sessionKey, owner, candidates, now, options)
					}
					return nil, nil
				}
				scheduled := &VirtualPoolScheduledCandidate{
					SessionKey:        sessionKey,
					Candidate:         existing,
					Binding:           binding,
					CapacityLease:     lease,
					CapacityKey:       capacityKey,
					CapacityAcquired:  true,
					PendingTTL:        options.PendingTTL,
					ConfirmedTTL:      options.ConfirmedTTL,
					CapacityLeaseTTL:  options.CapacityLease,
					PendingRenewEvery: options.PendingRenewEvery,
				}
				scheduler.startRenewal(ctx, scheduled)
				return scheduled, nil
			}
			if scheduler.bindings.Invalidate(ctx, sessionKey, binding.Generation, now) {
				return scheduler.selectOnce(ctx, sessionKey, owner, candidates, now, options)
			}
			return nil, nil
		}
		capacityKey := virtualPoolCapacityKey(candidate)
		lease, acquired, acquireErr := scheduler.capacity.Acquire(ctx, capacityKey, owner, virtualPoolCandidateCapacity(candidate), now, options.CapacityLease)
		if acquireErr != nil {
			_ = scheduler.bindings.Release(ctx, sessionKey, owner, binding.Generation, now)
			return nil, acquireErr
		}
		if !acquired {
			_ = scheduler.bindings.Release(ctx, sessionKey, owner, binding.Generation, now)
			continue
		}
		scheduled := &VirtualPoolScheduledCandidate{
			SessionKey:        sessionKey,
			Candidate:         candidate,
			Binding:           binding,
			CapacityLease:     lease,
			CapacityKey:       capacityKey,
			CapacityAcquired:  true,
			PendingTTL:        options.PendingTTL,
			ConfirmedTTL:      options.ConfirmedTTL,
			CapacityLeaseTTL:  options.CapacityLease,
			PendingRenewEvery: options.PendingRenewEvery,
		}
		scheduler.startRenewal(ctx, scheduled)
		return scheduled, nil
	}
	return nil, nil
}

type virtualPoolRankedCandidate struct {
	candidate VirtualPoolCandidate
	load      float64
	order     int
}

func (scheduler *VirtualPoolScheduler) candidatesByLoad(
	ctx context.Context,
	candidates []VirtualPoolCandidate,
	now time.Time,
) ([]virtualPoolRankedCandidate, error) {
	ranked := make([]virtualPoolRankedCandidate, 0, len(candidates))
	for index, candidate := range candidates {
		active, err := scheduler.capacity.Active(ctx, virtualPoolCapacityKey(candidate), now)
		if err != nil {
			return nil, err
		}
		weight := candidate.Weight
		if weight <= 0 {
			weight = 1
		}
		ranked = append(ranked, virtualPoolRankedCandidate{
			candidate: candidate,
			load:      (float64(active) + 1) / (float64(virtualPoolCandidateCapacity(candidate)) * weight),
			order:     index,
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].load == ranked[j].load {
			return ranked[i].order < ranked[j].order
		}
		return ranked[i].load < ranked[j].load
	})
	return ranked, nil
}

func (scheduler *VirtualPoolScheduler) Confirm(ctx context.Context, scheduled *VirtualPoolScheduledCandidate, now time.Time) bool {
	if scheduler == nil || scheduled == nil || scheduled.Binding.Pending == false || scheduled.LeaseLost() {
		return false
	}
	confirmed := scheduler.bindings.Confirm(
		ctx,
		scheduled.SessionKey,
		scheduled.Binding.Owner,
		scheduled.Binding.Generation,
		scheduled.Binding.CandidateKey,
		now,
		scheduler.confirmedTTL,
	)
	return confirmed
}

func (scheduler *VirtualPoolScheduler) Release(ctx context.Context, scheduled *VirtualPoolScheduledCandidate) error {
	if scheduler == nil || scheduled == nil {
		return nil
	}
	scheduler.stopRenewal(scheduled)
	if scheduled.CapacityAcquired {
		scheduler.capacity.Release(
			context.WithoutCancel(ctx),
			scheduled.CapacityKey,
			scheduled.CapacityLease.Owner,
			scheduled.CapacityLease.Generation,
			time.Now(),
		)
	}
	return nil
}

func (scheduler *VirtualPoolScheduler) Abort(ctx context.Context, scheduled *VirtualPoolScheduledCandidate, now time.Time) error {
	if scheduler == nil || scheduled == nil {
		return nil
	}
	scheduler.stopRenewal(scheduled)
	if scheduled.Binding.Pending {
		scheduler.bindings.Release(
			context.WithoutCancel(ctx),
			scheduled.SessionKey,
			scheduled.Binding.Owner,
			scheduled.Binding.Generation,
			now,
		)
	}
	return scheduler.Release(ctx, scheduled)
}

func (scheduler *VirtualPoolScheduler) startRenewal(ctx context.Context, scheduled *VirtualPoolScheduledCandidate) {
	if scheduler == nil || scheduled == nil || scheduled.stopRenewal != nil {
		return
	}
	bindingEvery := scheduled.PendingRenewEvery
	if halfConfirmed := scheduled.ConfirmedTTL / 2; halfConfirmed > 0 && (bindingEvery <= 0 || halfConfirmed < bindingEvery) {
		bindingEvery = halfConfirmed
	}
	if bindingEvery <= 0 {
		bindingEvery = scheduled.ConfirmedTTL / 2
	}
	if bindingEvery <= 0 {
		bindingEvery = time.Minute
	}
	capacityEvery := scheduled.CapacityLeaseTTL / 2
	if capacityEvery <= 0 {
		capacityEvery = time.Minute
	}
	interval := bindingEvery
	if capacityEvery < interval {
		interval = capacityEvery
	}
	if interval <= 0 {
		return
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	scheduled.stopRenewal = stop
	scheduled.renewalDone = done
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				scheduler.renewScheduled(ctx, scheduled, time.Now())
			case <-stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (scheduler *VirtualPoolScheduler) renewScheduled(ctx context.Context, scheduled *VirtualPoolScheduledCandidate, now time.Time) {
	if scheduler == nil || scheduled == nil {
		return
	}
	if scheduled.CapacityAcquired {
		if !scheduler.capacity.Renew(
			ctx,
			scheduled.CapacityKey,
			scheduled.CapacityLease.Owner,
			scheduled.CapacityLease.Generation,
			now,
			scheduled.CapacityLeaseTTL,
		) {
			scheduled.markLeaseLost()
			return
		}
	}
	if !scheduler.bindings.Renew(
		ctx,
		scheduled.SessionKey,
		scheduled.Binding.Owner,
		scheduled.Binding.Generation,
		now,
		scheduled.ConfirmedTTL,
	) {
		if !scheduler.bindings.RenewPending(
			ctx,
			scheduled.SessionKey,
			scheduled.Binding.Owner,
			scheduled.Binding.Generation,
			now,
			scheduled.PendingTTL,
		) {
			scheduled.markLeaseLost()
		}
	}
}

func (scheduler *VirtualPoolScheduler) stopRenewal(scheduled *VirtualPoolScheduledCandidate) {
	if scheduler == nil || scheduled == nil || scheduled.stopRenewal == nil {
		return
	}
	close(scheduled.stopRenewal)
	<-scheduled.renewalDone
	scheduled.stopRenewal = nil
	scheduled.renewalDone = nil
}

func virtualPoolCandidateCapacity(candidate VirtualPoolCandidate) int {
	if candidate.Capacity <= 0 {
		return 1
	}
	return candidate.Capacity
}

func virtualPoolCapacityKey(candidate VirtualPoolCandidate) string {
	group := strings.TrimSpace(candidate.CapacityGroup)
	if group != "" {
		return "group:" + strings.ToLower(group)
	}
	return candidate.AccountIdentity + "|" + strings.ToLower(strings.TrimSpace(candidate.FinalMappedModel))
}
