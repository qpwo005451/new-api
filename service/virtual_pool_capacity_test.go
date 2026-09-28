package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newVirtualPoolIntegrationCapacityRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	addr := strings.TrimSpace(os.Getenv("VIRTUAL_POOL_TEST_REDIS_ADDR"))
	if addr == "" {
		t.Skip("VIRTUAL_POOL_TEST_REDIS_ADDR is not configured")
	}
	host := addr
	if index := strings.LastIndex(addr, ":"); index >= 0 {
		host = strings.Trim(addr[:index], "[]")
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		t.Fatalf("VIRTUAL_POOL_TEST_REDIS_ADDR must use a loopback host, got %q", addr)
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestVirtualPoolRedisIntegrationEnforcesCapacityAcrossClients(t *testing.T) {
	clientA := newVirtualPoolIntegrationCapacityRedisClient(t)
	clientB := redis.NewClient(&redis.Options{Addr: clientA.Options().Addr})
	t.Cleanup(func() { _ = clientB.Close() })
	storeA := NewVirtualPoolRedisCapacityStore(clientA)
	storeB := NewVirtualPoolRedisCapacityStore(clientB)
	ctx := context.Background()
	now := time.Now()
	accountKey := fmt.Sprintf("integration-account-%d", now.UnixNano())
	t.Cleanup(func() { _ = clientA.Del(context.Background(), storeA.key(accountKey)).Err() })

	first, acquired, err := storeA.Acquire(ctx, accountKey, "owner-a", 1, now, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	_, acquired, err = storeB.Acquire(ctx, accountKey, "owner-b", 1, now.Add(time.Second), time.Minute)
	require.NoError(t, err)
	assert.False(t, acquired)
	require.True(t, storeB.Release(ctx, accountKey, "owner-a", first.Generation, now.Add(2*time.Second)))
	_, acquired, err = storeA.Acquire(ctx, accountKey, "owner-b", 1, now.Add(3*time.Second), time.Minute)
	require.NoError(t, err)
	assert.True(t, acquired)
}

func TestVirtualPoolMemoryCapacityEnforcesLimitAndFencesRelease(t *testing.T) {
	store := NewVirtualPoolMemoryCapacityStore()
	ctx := context.Background()
	now := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)

	first, acquired, err := store.Acquire(ctx, "account", "owner-a", 1, now, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	assert.Equal(t, int64(1), first.Generation)

	_, acquired, err = store.Acquire(ctx, "account", "owner-b", 1, now, time.Minute)
	require.NoError(t, err)
	assert.False(t, acquired)

	require.True(t, store.Release(ctx, "account", "owner-a", first.Generation, now.Add(time.Second)))
	_, acquired, err = store.Acquire(ctx, "account", "owner-b", 1, now.Add(2*time.Second), time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)

	assert.False(t, store.Release(ctx, "account", "owner-a", first.Generation, now.Add(3*time.Second)),
		"a stale release must not free the new owner's slot")
}

func TestVirtualPoolCapacityExpiresOwnersIndependently(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.September, 28, 12, 30, 0, 0, time.UTC)

	memory := NewVirtualPoolMemoryCapacityStore()
	first, acquired, err := memory.Acquire(ctx, "account", "owner-a", 1, now, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	second, acquired, err := memory.Acquire(ctx, "account", "owner-b", 2, now.Add(30*time.Second), time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)

	_, acquired, err = memory.Acquire(ctx, "account", "owner-c", 2, now.Add(70*time.Second), time.Minute)
	require.NoError(t, err)
	assert.True(t, acquired, "owner-a's expired slot must be reclaimed while owner-b remains active")
	assert.False(t, memory.Release(ctx, "account", "owner-a", first.Generation, now.Add(70*time.Second)))
	assert.True(t, memory.Release(ctx, "account", "owner-b", second.Generation, now.Add(70*time.Second)))

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisStore := NewVirtualPoolRedisCapacityStore(client)
	first, acquired, err = redisStore.Acquire(ctx, "account", "owner-a", 1, now, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)
	second, acquired, err = redisStore.Acquire(ctx, "account", "owner-b", 2, now.Add(30*time.Second), time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)

	_, acquired, err = redisStore.Acquire(ctx, "account", "owner-c", 2, now.Add(70*time.Second), time.Minute)
	require.NoError(t, err)
	assert.True(t, acquired, "Redis must prune each owner by its own expiry")
	assert.False(t, redisStore.Release(ctx, "account", "owner-a", first.Generation, now.Add(70*time.Second)))
	assert.True(t, redisStore.Release(ctx, "account", "owner-b", second.Generation, now.Add(70*time.Second)))
}

func TestVirtualPoolRedisCapacityIsAtomicAcrossClients(t *testing.T) {
	server := miniredis.RunT(t)
	clientA := redis.NewClient(&redis.Options{Addr: server.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = clientA.Close()
		_ = clientB.Close()
	})
	storeA := NewVirtualPoolRedisCapacityStore(clientA)
	storeB := NewVirtualPoolRedisCapacityStore(clientB)
	ctx := context.Background()
	now := time.Date(2026, time.September, 28, 13, 0, 0, 0, time.UTC)

	first, acquired, err := storeA.Acquire(ctx, "account", "owner-a", 1, now, time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)

	_, acquired, err = storeB.Acquire(ctx, "account", "owner-b", 1, now.Add(time.Second), time.Minute)
	require.NoError(t, err)
	assert.False(t, acquired, "two clients must share one atomic capacity counter")

	require.True(t, storeB.Release(ctx, "account", "owner-a", first.Generation, now.Add(2*time.Second)))
	_, acquired, err = storeA.Acquire(ctx, "account", "owner-b", 1, now.Add(3*time.Second), time.Minute)
	require.NoError(t, err)
	assert.True(t, acquired)
}

func TestVirtualPoolSchedulerPrefersBoundCandidateAndSharedCapacity(t *testing.T) {
	now := time.Date(2026, time.September, 28, 14, 0, 0, 0, time.UTC)
	store := NewVirtualPoolMemoryBindingStore()
	capacity := NewVirtualPoolMemoryCapacityStore()
	scheduler := NewVirtualPoolScheduler(store, capacity)
	candidates := []VirtualPoolCandidate{
		{
			AccountIdentity:  "account-a",
			FinalMappedModel: "model",
			Capacity:         2,
			Weight:           1,
			CapacityGroup:    "shared",
		},
		{
			AccountIdentity:  "account-b",
			FinalMappedModel: "model",
			Capacity:         2,
			Weight:           1,
			CapacityGroup:    "shared",
		},
	}

	first, err := scheduler.Select(context.Background(), "session", "owner-a", candidates, now)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NotEmpty(t, first.CapacityKey)

	second, err := scheduler.Select(context.Background(), "session", "owner-b", candidates, now.Add(time.Second))
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, first.Candidate.AccountIdentity, second.Candidate.AccountIdentity,
		"an existing session binding wins while capacity remains")

	require.NoError(t, scheduler.Release(context.Background(), second))
}

func TestVirtualPoolSchedulerUsesWeightedLoadInsteadOfCandidateOrder(t *testing.T) {
	now := time.Date(2026, time.September, 28, 14, 30, 0, 0, time.UTC)
	store := NewVirtualPoolMemoryBindingStore()
	capacity := NewVirtualPoolMemoryCapacityStore()
	scheduler := NewVirtualPoolScheduler(store, capacity)
	candidates := []VirtualPoolCandidate{
		{
			AccountIdentity:  "account-a",
			FinalMappedModel: "model",
			Capacity:         10,
			Weight:           1,
		},
		{
			AccountIdentity:  "account-b",
			FinalMappedModel: "model",
			Capacity:         10,
			Weight:           10,
		},
	}

	first, err := scheduler.Select(context.Background(), "session-a", "owner-a", candidates, now)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, "account-b", first.Candidate.AccountIdentity,
		"the higher-weight candidate must win despite appearing later in the pool")
	require.NoError(t, scheduler.Release(context.Background(), first))
}

func TestVirtualPoolSchedulerRenewsCapacityLeaseForLongRunningAttempt(t *testing.T) {
	now := time.Date(2026, time.September, 28, 14, 45, 0, 0, time.UTC)
	store := NewVirtualPoolMemoryBindingStore()
	capacity := NewVirtualPoolMemoryCapacityStore()
	scheduler := NewVirtualPoolScheduler(store, capacity)
	candidates := []VirtualPoolCandidate{{
		AccountIdentity:  "account-a",
		FinalMappedModel: "model",
		Capacity:         2,
	}}

	scheduled, err := scheduler.SelectWithOptions(context.Background(), "session", "owner", candidates, now, VirtualPoolSchedulerOptions{
		PendingTTL:        time.Minute,
		ConfirmedTTL:      time.Second,
		CapacityLease:     10 * time.Second,
		PendingRenewEvery: time.Second,
	})
	require.NoError(t, err)
	require.NotNil(t, scheduled)
	scheduler.renewScheduled(context.Background(), scheduled, now.Add(5*time.Second))

	_, acquired, err := capacity.Acquire(context.Background(), scheduled.CapacityKey, "other-owner", 1, now.Add(12*time.Second), time.Second)
	require.NoError(t, err)
	assert.False(t, acquired, "renewal must extend the capacity lease past its original expiry")
	require.NoError(t, scheduler.Abort(context.Background(), scheduled, now.Add(12*time.Second)))
}

func TestVirtualPoolSchedulerCancelsAttemptWhenLeaseRenewalFails(t *testing.T) {
	now := time.Date(2026, time.September, 28, 14, 50, 0, 0, time.UTC)
	store := NewVirtualPoolMemoryBindingStore()
	capacity := NewVirtualPoolMemoryCapacityStore()
	scheduler := NewVirtualPoolScheduler(store, capacity)
	candidates := []VirtualPoolCandidate{{
		AccountIdentity:  "account-a",
		FinalMappedModel: "model",
		Capacity:         1,
	}}

	scheduled, err := scheduler.SelectWithOptions(context.Background(), "session", "owner", candidates, now, VirtualPoolSchedulerOptions{
		PendingTTL:        time.Minute,
		ConfirmedTTL:      time.Minute,
		CapacityLease:     time.Second,
		PendingRenewEvery: time.Hour,
	})
	require.NoError(t, err)
	require.NotNil(t, scheduled)

	lost := make(chan error, 1)
	attemptCtx, cancelAttempt := context.WithCancelCause(context.Background())
	scheduled.setCancelAttempt(cancelAttempt)
	go func() {
		<-attemptCtx.Done()
		lost <- context.Cause(attemptCtx)
	}()

	require.True(t, capacity.Release(
		context.Background(),
		scheduled.CapacityKey,
		scheduled.CapacityLease.Owner,
		scheduled.CapacityLease.Generation,
		now.Add(500*time.Millisecond),
	), "the test fakes another instance reclaiming the expired slot")
	scheduler.renewScheduled(context.Background(), scheduled, now.Add(2*time.Second))

	require.ErrorIs(t, <-lost, ErrVirtualPoolLeaseLost)
	assert.True(t, scheduled.LeaseLost())
	require.NoError(t, scheduler.Abort(context.Background(), scheduled, now.Add(3*time.Second)))
}

func TestVirtualPoolSchedulerCancelsAttemptWhenLeaseLostBeforeContextRegistration(t *testing.T) {
	now := time.Date(2026, time.September, 28, 14, 55, 0, 0, time.UTC)
	store := NewVirtualPoolMemoryBindingStore()
	capacity := NewVirtualPoolMemoryCapacityStore()
	scheduler := NewVirtualPoolScheduler(store, capacity)
	candidates := []VirtualPoolCandidate{{
		AccountIdentity:  "account-a",
		FinalMappedModel: "model",
		Capacity:         1,
	}}

	scheduled, err := scheduler.SelectWithOptions(context.Background(), "session", "owner", candidates, now, VirtualPoolSchedulerOptions{
		PendingTTL:        time.Minute,
		ConfirmedTTL:      time.Minute,
		CapacityLease:     time.Second,
		PendingRenewEvery: time.Hour,
	})
	require.NoError(t, err)
	require.NotNil(t, scheduled)

	require.True(t, capacity.Release(
		context.Background(),
		scheduled.CapacityKey,
		scheduled.CapacityLease.Owner,
		scheduled.CapacityLease.Generation,
		now.Add(500*time.Millisecond),
	))
	scheduler.renewScheduled(context.Background(), scheduled, now.Add(2*time.Second))
	require.True(t, scheduled.LeaseLost())

	attemptCtx, cancelAttempt := context.WithCancelCause(context.Background())
	scheduled.setCancelAttempt(cancelAttempt)
	require.ErrorIs(t, context.Cause(attemptCtx), ErrVirtualPoolLeaseLost,
		"a lease lost before controller registration must still cancel the attempt")
	require.NoError(t, scheduler.Abort(context.Background(), scheduled, now.Add(3*time.Second)))
}

func TestVirtualPoolMemoryBindingRenewRequiresLiveLease(t *testing.T) {
	store := NewVirtualPoolMemoryBindingStore()
	ctx := context.Background()
	now := time.Date(2026, time.September, 28, 15, 0, 0, 0, time.UTC)

	binding, claimed, err := store.Claim(ctx, "session", "candidate", "owner", now, time.Minute, time.Hour)
	require.NoError(t, err)
	require.True(t, claimed)
	require.True(t, store.RenewPending(ctx, "session", "owner", binding.Generation, now.Add(30*time.Second), time.Minute))
	assert.False(t, store.RenewPending(ctx, "session", "owner", binding.Generation, now.Add(2*time.Minute), time.Minute),
		"a lease that already expired cannot be resurrected")

	require.True(t, store.Confirm(ctx, "session", "owner", binding.Generation, "candidate", now.Add(30*time.Second), time.Hour))
	assert.True(t, store.Renew(ctx, "session", "owner", binding.Generation, now.Add(time.Minute), time.Hour))
	assert.False(t, store.Renew(ctx, "session", "owner", binding.Generation, now.Add(2*time.Hour), time.Hour),
		"a confirmed lease that already expired cannot be resurrected")
}

func TestVirtualPoolSchedulerDoesNotResurrectExpiredBinding(t *testing.T) {
	now := time.Date(2026, time.September, 28, 15, 30, 0, 0, time.UTC)
	store := NewVirtualPoolMemoryBindingStore()
	capacity := NewVirtualPoolMemoryCapacityStore()
	scheduler := NewVirtualPoolScheduler(store, capacity)
	candidates := []VirtualPoolCandidate{{
		AccountIdentity:  "account-a",
		FinalMappedModel: "model",
		Capacity:         2,
	}}

	scheduled, err := scheduler.SelectWithOptions(context.Background(), "session", "owner-a", candidates, now, VirtualPoolSchedulerOptions{
		PendingTTL:        time.Second,
		ConfirmedTTL:      time.Minute,
		CapacityLease:     time.Minute,
		PendingRenewEvery: time.Hour,
	})
	require.NoError(t, err)
	require.NotNil(t, scheduled)

	second, err := scheduler.Select(context.Background(), "session", "owner-b", candidates, now.Add(3*time.Second))
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Greater(t, second.Binding.Generation, scheduled.Binding.Generation)
	assert.Equal(t, "owner-b", second.Binding.Owner, "an expired pending binding must be replaced")
	require.NoError(t, scheduler.Abort(context.Background(), second, now.Add(4*time.Second)))
	require.NoError(t, scheduler.Abort(context.Background(), scheduled, now.Add(4*time.Second)))
}

func TestRetryParamPreparesEachAttemptWithAFreshCapacityLease(t *testing.T) {
	installVirtualPoolStickyForTest(t, operation_setting.VirtualPoolStickySetting{
		Enabled:              true,
		BindingMode:          operation_setting.VirtualPoolBindingModeMemory,
		CapacityWaitMillis:   20,
		CapacityLeaseSeconds: 60,
	})
	session := &VirtualPoolSession{
		SessionDigest: common.Sha1([]byte("fresh-attempt-session")),
		CacheKey:      "scope:fresh-attempt-session",
	}
	param := &RetryParam{
		virtualSession: session,
		preparedRoute: &VirtualPoolPreparedRoute{
			Session: session,
			Candidates: []VirtualPoolCandidate{{
				Channel:          &model.Channel{Id: 2801},
				VirtualModel:     "virtual",
				UpstreamModel:    "upstream",
				FinalMappedModel: "upstream",
				AccountIdentity:  "2801",
				Capacity:         1,
			}},
		},
	}

	require.NoError(t, param.PrepareVirtualPoolAttempt(nil))
	require.NotNil(t, param.scheduled)
	firstGeneration := param.scheduled.CapacityLease.Generation
	param.ReleaseVirtualPoolAttempt(nil, true)
	require.Nil(t, param.scheduled)

	require.NoError(t, param.PrepareVirtualPoolAttempt(nil))
	require.NotNil(t, param.scheduled)
	assert.NotZero(t, param.scheduled.CapacityLease.Generation,
		"a retry must acquire a usable lease rather than reuse the failed attempt")
	assert.NotEqual(t, firstGeneration, param.scheduled.CapacityLease.Generation,
		"the retry lease must carry a new fencing generation")
}

func TestVirtualPoolCapacityWithoutSessionUsesSharedAccountLimit(t *testing.T) {
	now := time.Date(2026, time.September, 28, 16, 0, 0, 0, time.UTC)
	bindings := NewVirtualPoolMemoryBindingStore()
	capacity := NewVirtualPoolMemoryCapacityStore()
	scheduler := NewVirtualPoolScheduler(bindings, capacity)
	candidates := []VirtualPoolCandidate{
		{
			AccountIdentity:  "input-subscription-a",
			FinalMappedModel: "deepseek-v4.1-flash",
			Capacity:         15,
			CapacityGroup:    "input-15",
		},
		{
			AccountIdentity:  "input-subscription-b",
			FinalMappedModel: "deepseek-v4.1-flash",
			Capacity:         15,
			CapacityGroup:    "input-15",
		},
		{
			AccountIdentity:  "input-subscription-c",
			FinalMappedModel: "deepseek-v4.1-flash",
			Capacity:         15,
			CapacityGroup:    "input-15",
		},
	}

	leases := make([]*VirtualPoolScheduledCandidate, 0, 15)
	for index := 0; index < 15; index++ {
		scheduled, err := scheduler.SelectCapacityWithOptions(
			context.Background(),
			fmt.Sprintf("owner-%d", index),
			candidates,
			now,
			VirtualPoolSchedulerOptions{CapacityLease: time.Minute},
		)
		require.NoError(t, err)
		require.NotNil(t, scheduled, "slot %d of the shared limit must be available", index+1)
		leases = append(leases, scheduled)
	}
	exhausted, err := scheduler.SelectCapacityWithOptions(
		context.Background(),
		"owner-16",
		candidates,
		now,
		VirtualPoolSchedulerOptions{CapacityLease: time.Minute},
	)
	require.NoError(t, err)
	assert.Nil(t, exhausted, "the three subscriptions must share one 15-slot account limit")
	for _, scheduled := range leases {
		require.NoError(t, scheduler.Release(context.Background(), scheduled))
	}
}
