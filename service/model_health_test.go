package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func installModelHealthPolicyForTest(t *testing.T, rule operation_setting.ModelHealthPolicyRule) {
	t.Helper()
	setting := operation_setting.GetModelHealthPolicySetting()
	original := *setting
	setting.Enabled = true
	setting.Rules = []operation_setting.ModelHealthPolicyRule{rule}
	t.Cleanup(func() { *setting = original })
}

func TestModelHealthCoolsOnlyTheMatchingModelAndChannel(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "deepseek-v4.1-flash"
	createChannelSelectAutoGroupsChannel(t, db, 2801, "default", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2802, "default", modelName)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2801).Updates(map[string]any{"priority": 500, "weight": 1}).Error)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2802).Updates(map[string]any{"priority": 499, "weight": 1}).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 2801).Updates(map[string]any{"priority": 500, "weight": 1}).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 2802).Updates(map[string]any{"priority": 499, "weight": 1}).Error)
	createChannelSelectAutoGroupsChannel(t, db, 2803, "default", "glm-5.3-flash")
	model.InitChannelCache()
	installModelHealthPolicyForTest(t, operation_setting.ModelHealthPolicyRule{
		Name:             "国模熔断",
		Enabled:          true,
		Models:           []string{modelName, "glm-5.3-flash"},
		FailureThreshold: 2,
		CooldownSeconds:  300,
		StatusCodes:      []int{429, 502, 503},
	})
	RecordModelHealthFailure(modelName, "default", 2801, http.StatusServiceUnavailable)

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/responses",
		Retry:       &retry,
	}
	first, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2801, first.Id, "one failure must stay below the configured threshold")

	RecordModelHealthFailure(modelName, "default", 2801, http.StatusServiceUnavailable)
	next, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/responses",
		Retry:       new(int),
	})
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.Equal(t, 2802, next.Id, "the cooling channel must be skipped for only this model")

	glm, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   "glm-5.3-flash",
		RequestPath: "/v1/responses",
		Retry:       new(int),
	})
	require.NoError(t, err)
	require.NotNil(t, glm)
	assert.Equal(t, 2803, glm.Id, "another model on the same health policy is selected independently")
}

func TestModelHealthRecoversAfterCooldown(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "glm-5.3-flash"
	createChannelSelectAutoGroupsChannel(t, db, 2811, "default", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2812, "default", modelName)
	model.InitChannelCache()
	installModelHealthPolicyForTest(t, operation_setting.ModelHealthPolicyRule{
		Name:             "glm health",
		Enabled:          true,
		Models:           []string{modelName},
		FailureThreshold: 1,
		CooldownSeconds:  1,
		StatusCodes:      []int{429},
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	RecordModelHealthFailure(modelName, "default", 2811, http.StatusTooManyRequests)
	first, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/responses",
		Retry:       new(int),
	})
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.NotEqual(t, 2811, first.Id)

	time.Sleep(1100 * time.Millisecond)
	require.Eventually(t, func() bool {
		view, ok := ModelHealthSnapshot(modelName, 2811)
		return ok && !view.Cooling
	}, 2*time.Second, 50*time.Millisecond)
}

func TestModelHealthSuccessClearsCounter(t *testing.T) {
	const modelName = "deepseek-v4.1-flash"
	installModelHealthPolicyForTest(t, operation_setting.ModelHealthPolicyRule{
		Name:             "deepseek health",
		Enabled:          true,
		Models:           []string{modelName},
		FailureThreshold: 1,
		CooldownSeconds:  300,
		StatusCodes:      []int{503},
	})
	RecordModelHealthFailure(modelName, "default", 2821, http.StatusServiceUnavailable)
	RecordModelHealthSuccess(modelName, "default", 2821)
	view, ok := ModelHealthSnapshot(modelName, 2821)
	require.True(t, ok)
	assert.False(t, view.Cooling)
	assert.Zero(t, view.ConsecutiveErrs)
}

func TestModelHealthIgnoresUnconfiguredStatus(t *testing.T) {
	const modelName = "glm-5.3-flash"
	installModelHealthPolicyForTest(t, operation_setting.ModelHealthPolicyRule{
		Name:             "glm health",
		Enabled:          true,
		Models:           []string{modelName},
		FailureThreshold: 1,
		CooldownSeconds:  300,
		StatusCodes:      []int{503},
	})
	RecordModelHealthFailure(modelName, "default", 2831, http.StatusBadRequest)
	view, ok := ModelHealthSnapshot(modelName, 2831)
	require.True(t, ok)
	assert.False(t, view.Cooling)
	assert.Zero(t, view.ConsecutiveErrs)
}

func TestModelHealthMatchesRegexAndGroup(t *testing.T) {
	installModelHealthPolicyForTest(t, operation_setting.ModelHealthPolicyRule{
		Name:       "国模规则",
		Enabled:    true,
		ModelRegex: []string{`^(deepseek|glm)-`},
		Groups:     []string{"svip"},
	})
	_, ok := operation_setting.MatchModelHealthPolicy("deepseek-v4.1-flash", "svip")
	assert.True(t, ok)
	_, ok = operation_setting.MatchModelHealthPolicy("deepseek-v4.1-flash", "default")
	assert.False(t, ok)
	_, ok = operation_setting.MatchModelHealthPolicy("gpt-5.6-luna", "svip")
	assert.False(t, ok)
}
