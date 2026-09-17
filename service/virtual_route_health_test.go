package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
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

func TestVirtualRouteHealthEntryCooldownDoublesUpToTheMaximum(t *testing.T) {
	health := operation_setting.VirtualModelRouteHealth{
		Enabled:            true,
		FailureThreshold:   2,
		CooldownSeconds:    60,
		MaxCooldownSeconds: 120,
	}.Normalize()

	entry := &virtualRouteHealthEntry{}
	entry.recordFailure(health)
	assert.True(t, entry.cooldownUntil.IsZero(), "the first failure stays below the threshold")

	entry.recordFailure(health)
	assert.False(t, entry.cooldownUntil.IsZero())

	entry.recordSuccess()
	assert.True(t, entry.cooldownUntil.IsZero(), "a success clears the cooldown")

	for i := 0; i < 8; i++ {
		entry.recordFailure(health)
	}
	assert.False(t, entry.cooldownUntil.IsZero())
}

func TestIsVirtualRouteUnavailableStatus(t *testing.T) {
	assert.True(t, IsVirtualRouteUnavailableStatus(http.StatusTooManyRequests))
	assert.True(t, IsVirtualRouteUnavailableStatus(http.StatusNotFound))
	assert.True(t, IsVirtualRouteUnavailableStatus(http.StatusServiceUnavailable))
	assert.False(t, IsVirtualRouteUnavailableStatus(http.StatusBadRequest))
	assert.False(t, IsVirtualRouteUnavailableStatus(http.StatusOK))
}
