package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func installVirtualRouteHealthRoute(t *testing.T, modelName string, health operation_setting.VirtualModelRouteHealth, targets ...string) {
	t.Helper()

	retrySetting := operation_setting.GetModelRetryPolicySetting()
	originalRoutes := retrySetting.VirtualModelRoutes
	routeTargets := make([]operation_setting.VirtualModelRouteTarget, 0, len(targets))
	for _, target := range targets {
		routeTargets = append(routeTargets, operation_setting.VirtualModelRouteTarget{Model: target})
	}
	retrySetting.VirtualModelRoutes = map[string]operation_setting.VirtualModelRoute{
		modelName: {Health: health, Targets: routeTargets},
	}
	t.Cleanup(func() {
		retrySetting.VirtualModelRoutes = originalRoutes
	})
}

func newVirtualRouteHealthAttempt(ctx *gin.Context, modelName string) *RetryParam {
	retry := 0
	return &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}
}

func TestVirtualRouteHealthMovesCoolingEntryBehindHealthyOnes(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-health-cooling"
	createChannelSelectAutoGroupsChannel(t, db, 2401, "default", "health-alpha")
	createChannelSelectAutoGroupsChannel(t, db, 2402, "default", "health-beta")
	model.InitChannelCache()
	installVirtualRouteHealthRoute(t, modelName, operation_setting.VirtualModelRouteHealth{
		Enabled:            true,
		FailureThreshold:   1,
		CooldownSeconds:    60,
		MaxCooldownSeconds: 600,
	}, "health-alpha", "health-beta")

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyVirtualUpstreamModel, "health-alpha")

	RecordVirtualRouteFailure(ctx, 2401, modelName, http.StatusTooManyRequests)

	param := newVirtualRouteHealthAttempt(ctx, modelName)
	assert.Equal(t, 1, param.RetryLimit(9), "the cooling entry stays in the pool as a last resort")

	first, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2402, first.Id, "the healthy entry is preferred")

	param.IncreaseRetry()
	second, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2401, second.Id, "the cooling entry is still tried after the healthy ones")

	param.IncreaseRetry()
	_, _, err = CacheGetRandomSatisfiedChannel(param)
	require.ErrorIs(t, err, model.ErrPriorityFallbackExhausted)
}

func TestVirtualRouteHealthSuccessClearsCooldown(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-health-recovery"
	createChannelSelectAutoGroupsChannel(t, db, 2411, "default", "recovery-alpha")
	createChannelSelectAutoGroupsChannel(t, db, 2412, "default", "recovery-beta")
	model.InitChannelCache()
	installVirtualRouteHealthRoute(t, modelName, operation_setting.VirtualModelRouteHealth{
		Enabled:          true,
		FailureThreshold: 1,
		CooldownSeconds:  60,
	}, "recovery-alpha", "recovery-beta")

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyVirtualUpstreamModel, "recovery-alpha")

	RecordVirtualRouteFailure(ctx, 2411, modelName, http.StatusServiceUnavailable)
	RecordVirtualRouteSuccess(ctx, 2411, modelName)

	param := newVirtualRouteHealthAttempt(ctx, modelName)
	first, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2411, first.Id, "a recovered entry returns to the front of the pool")
}

func TestVirtualRouteHealthRecordsFailureFromSelectedVirtualCandidate(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-health-selected-candidate"
	createChannelSelectAutoGroupsChannel(t, db, 2415, "default", "selected-health-alpha")
	createChannelSelectAutoGroupsChannel(t, db, 2416, "default", "selected-health-beta")
	model.InitChannelCache()
	installVirtualRouteHealthRoute(t, modelName, operation_setting.VirtualModelRouteHealth{
		Enabled:          true,
		FailureThreshold: 1,
		CooldownSeconds:  60,
	}, "selected-health-alpha", "selected-health-beta")

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}
	first, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, 2415, first.Id)

	RecordVirtualRouteFailure(ctx, first.Id, modelName, http.StatusInternalServerError)

	secondParam := newVirtualRouteHealthAttempt(ctx, modelName)
	second, _, err := CacheGetRandomSatisfiedChannel(secondParam)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2416, second.Id, "a failed selected candidate must cool down for the next request")
}

func TestVirtualRouteHealthCoolingCandidateIsUsedByScheduler(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-health-scheduled-candidate"
	createChannelSelectAutoGroupsChannel(t, db, 2417, "default", "scheduled-health-alpha")
	createChannelSelectAutoGroupsChannel(t, db, 2418, "default", "scheduled-health-beta")
	model.InitChannelCache()
	installVirtualRouteHealthRoute(t, modelName, operation_setting.VirtualModelRouteHealth{
		Enabled:          true,
		FailureThreshold: 1,
		CooldownSeconds:  60,
	}, "scheduled-health-alpha", "scheduled-health-beta")

	retrySetting := operation_setting.GetModelRetryPolicySetting()
	originalSticky := retrySetting.VirtualPoolSticky
	retrySetting.VirtualPoolSticky = operation_setting.VirtualPoolStickySetting{
		Enabled:        true,
		BindingMode:    operation_setting.VirtualPoolBindingModeMemory,
		SessionMode:    operation_setting.VirtualPoolSessionModeThread,
		MultiKeyPolicy: operation_setting.VirtualPoolMultiKeyPolicyBindIndex,
	}
	t.Cleanup(func() {
		retrySetting.VirtualPoolSticky = originalSticky
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	retry := 0
	firstParam := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}
	first, _, err := CacheGetRandomSatisfiedChannel(firstParam)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.Equal(t, 2417, first.Id)

	RecordVirtualRouteFailure(ctx, first.Id, modelName, http.StatusInternalServerError)
	firstParam.ReleaseVirtualPoolAttempt(ctx, true)

	secondParam := newVirtualRouteHealthAttempt(ctx, modelName)
	second, _, err := CacheGetRandomSatisfiedChannel(secondParam)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2418, second.Id, "sticky scheduling must follow the health-adjusted candidate order")
}

func TestVirtualRouteHealthIgnoresClientErrorsAndDisabledRoutes(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-health-client-error"
	createChannelSelectAutoGroupsChannel(t, db, 2421, "default", "client-error-alpha")
	createChannelSelectAutoGroupsChannel(t, db, 2422, "default", "client-error-beta")
	model.InitChannelCache()
	installVirtualRouteHealthRoute(t, modelName, operation_setting.VirtualModelRouteHealth{
		Enabled:          true,
		FailureThreshold: 1,
		CooldownSeconds:  60,
	}, "client-error-alpha", "client-error-beta")

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyVirtualUpstreamModel, "client-error-alpha")

	RecordVirtualRouteFailure(ctx, 2421, modelName, http.StatusBadRequest)
	RecordVirtualRouteFailure(ctx, 2421, modelName, http.StatusUnauthorized)

	param := newVirtualRouteHealthAttempt(ctx, modelName)
	first, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2421, first.Id, "a client-side error must not cool the pool entry down")

	// A route without the health policy keeps the configured order as well.
	installVirtualRouteHealthRoute(t, modelName, operation_setting.VirtualModelRouteHealth{}, "client-error-alpha", "client-error-beta")
	RecordVirtualRouteFailure(ctx, 2421, modelName, http.StatusServiceUnavailable)
	param = newVirtualRouteHealthAttempt(ctx, modelName)
	first, _, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2421, first.Id)
}

func TestVirtualRouteHealthRedisFailureFailsClosedWhenRequired(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-health-redis-required"
	createChannelSelectAutoGroupsChannel(t, db, 2431, "default", "redis-health-alpha")
	createChannelSelectAutoGroupsChannel(t, db, 2432, "default", "redis-health-beta")
	model.InitChannelCache()
	installVirtualRouteHealthRoute(t, modelName, operation_setting.VirtualModelRouteHealth{
		Enabled:          true,
		FailureThreshold: 1,
		CooldownSeconds:  60,
	}, "redis-health-alpha", "redis-health-beta")

	previousRedisEnabled := common.RedisEnabled
	previousRDB := common.RDB
	previousSticky := operation_setting.GetModelRetryPolicySetting().VirtualPoolSticky
	common.RedisEnabled = false
	common.RDB = nil
	operation_setting.GetModelRetryPolicySetting().VirtualPoolSticky = operation_setting.VirtualPoolStickySetting{
		Enabled:               true,
		BindingMode:           operation_setting.VirtualPoolBindingModeRedis,
		RedisRequiredForReady: true,
	}
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		common.RDB = previousRDB
		operation_setting.GetModelRetryPolicySetting().VirtualPoolSticky = previousSticky
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyVirtualUpstreamModel, "redis-health-alpha")

	param := newVirtualRouteHealthAttempt(ctx, modelName)
	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.ErrorIs(t, err, ErrVirtualPoolStoreUnavailable)
	assert.Nil(t, channel, "a required Redis health store must not silently fall back to process-local state")
}

func TestVirtualRouteHealthEntryCooldownDoublesUpToTheMaximum(t *testing.T) {
	health := operation_setting.VirtualModelRouteHealth{
		Enabled:            true,
		FailureThreshold:   2,
		CooldownSeconds:    60,
		MaxCooldownSeconds: 120,
	}.Normalize()

	entry := &virtualRouteHealthEntry{}
	now := time.Date(2026, time.September, 28, 18, 0, 0, 0, time.UTC)
	entry.recordFailure(health, now)
	assert.True(t, entry.cooldownUntil.IsZero(), "the first failure stays below the threshold")

	entry.recordFailure(health, now)
	assert.False(t, entry.cooldownUntil.IsZero())

	entry.recordSuccess()
	assert.True(t, entry.cooldownUntil.IsZero(), "a success clears the cooldown")

	for i := 0; i < 8; i++ {
		entry.recordFailure(health, now)
	}
	assert.False(t, entry.cooldownUntil.IsZero())
}

func TestVirtualPoolRedisHealthSharesCooldownAcrossClients(t *testing.T) {
	server := miniredis.RunT(t)
	server.SetTime(time.Date(2026, time.September, 28, 18, 30, 0, 0, time.UTC))
	clientA := redis.NewClient(&redis.Options{Addr: server.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = clientA.Close()
		_ = clientB.Close()
	})
	storeA := NewVirtualPoolRedisHealthStore(clientA)
	storeB := NewVirtualPoolRedisHealthStore(clientB)
	ctx := context.Background()
	now := time.Date(2026, time.September, 28, 18, 30, 0, 0, time.UTC)
	health := operation_setting.VirtualModelRouteHealth{
		Enabled:            true,
		FailureThreshold:   1,
		CooldownSeconds:    60,
		MaxCooldownSeconds: 120,
	}

	require.NoError(t, storeA.RecordFailure(ctx, "entry", health, now))
	cooling, err := storeB.IsCoolingDown(ctx, "entry", now.Add(time.Second))
	require.NoError(t, err)
	assert.True(t, cooling, "a second instance must observe the shared cooldown")

	server.SetTime(now.Add(2 * time.Minute))
	cooling, err = storeB.IsCoolingDown(ctx, "entry", now.Add(2*time.Minute))
	require.NoError(t, err)
	assert.False(t, cooling, "the cooldown expires without another process-local reset")

	require.NoError(t, storeB.RecordSuccess(ctx, "entry"))
	cooling, err = storeA.IsCoolingDown(ctx, "entry", now.Add(time.Second))
	require.NoError(t, err)
	assert.False(t, cooling)
}

func TestVirtualRouteHealthCooldownDoublesAcrossStoreImplementations(t *testing.T) {
	health := operation_setting.VirtualModelRouteHealth{
		Enabled:            true,
		FailureThreshold:   2,
		CooldownSeconds:    60,
		MaxCooldownSeconds: 120,
	}.Normalize()
	now := time.Date(2026, time.September, 28, 19, 0, 0, 0, time.UTC)

	memory := NewVirtualRouteMemoryHealthStore()
	require.NoError(t, memory.RecordFailure(context.Background(), "entry", health, now))
	cooling, err := memory.IsCoolingDown(context.Background(), "entry", now)
	require.NoError(t, err)
	assert.False(t, cooling)
	require.NoError(t, memory.RecordFailure(context.Background(), "entry", health, now))
	cooling, err = memory.IsCoolingDown(context.Background(), "entry", now.Add(time.Second))
	require.NoError(t, err)
	assert.True(t, cooling)

	server := miniredis.RunT(t)
	server.SetTime(now)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisStore := NewVirtualPoolRedisHealthStore(client)
	require.NoError(t, redisStore.RecordFailure(context.Background(), "entry", health, now))
	cooling, err = redisStore.IsCoolingDown(context.Background(), "entry", now)
	require.NoError(t, err)
	assert.False(t, cooling)
	require.NoError(t, redisStore.RecordFailure(context.Background(), "entry", health, now))
	cooling, err = redisStore.IsCoolingDown(context.Background(), "entry", now.Add(time.Second))
	require.NoError(t, err)
	assert.True(t, cooling)
}

func TestVirtualPoolRedisHealthUsesServerTimeForCooldown(t *testing.T) {
	serverTime := time.Date(2026, time.September, 28, 19, 30, 0, 0, time.UTC)
	server := miniredis.RunT(t)
	server.SetTime(serverTime)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store := NewVirtualPoolRedisHealthStore(client)
	health := operation_setting.VirtualModelRouteHealth{
		Enabled:            true,
		FailureThreshold:   1,
		CooldownSeconds:    60,
		MaxCooldownSeconds: 60,
	}
	// The caller clock is intentionally far ahead. Redis must still own the
	// cooldown clock so instances with skewed clocks agree on expiry.
	callerTime := serverTime.Add(24 * time.Hour)

	require.NoError(t, store.RecordFailure(context.Background(), "entry", health, callerTime))
	cooling, err := store.IsCoolingDown(context.Background(), "entry", callerTime)
	require.NoError(t, err)
	assert.True(t, cooling)

	server.SetTime(serverTime.Add(time.Minute))
	cooling, err = store.IsCoolingDown(context.Background(), "entry", callerTime)
	require.NoError(t, err)
	assert.False(t, cooling)
}

func TestIsVirtualRouteUnavailableStatus(t *testing.T) {
	assert.True(t, IsVirtualRouteUnavailableStatus(http.StatusTooManyRequests))
	assert.True(t, IsVirtualRouteUnavailableStatus(http.StatusNotFound))
	assert.True(t, IsVirtualRouteUnavailableStatus(http.StatusServiceUnavailable))
	assert.False(t, IsVirtualRouteUnavailableStatus(http.StatusBadRequest))
	assert.False(t, IsVirtualRouteUnavailableStatus(http.StatusOK))
}
