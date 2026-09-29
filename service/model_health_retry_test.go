package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A policy-model retry must fail over to the healthy sibling in the same
// priority tier instead of jumping to the next priority level, mirroring the
// virtual-pool candidate walk. Weighted random selection alone re-hit the
// channel that just failed and never reached the healthy sibling.
func TestModelHealthRetryPrefersFreshSameTierChannel(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "deepseek-v4.1-flash"
	createChannelSelectAutoGroupsChannel(t, db, 2841, "default", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2842, "default", modelName)
	for _, id := range []int{2841, 2842} {
		require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", id).Updates(map[string]any{"priority": 700, "weight": 1}).Error)
		require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", id).Updates(map[string]any{"priority": 700, "weight": 1}).Error)
	}
	createChannelSelectAutoGroupsChannel(t, db, 2843, "default", modelName)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2843).Updates(map[string]any{"priority": 500, "weight": 1}).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 2843).Updates(map[string]any{"priority": 500, "weight": 1}).Error)
	model.InitChannelCache()
	installModelHealthPolicyForTest(t, operation_setting.ModelHealthPolicyRule{
		Name:             "同层 failover",
		Enabled:          true,
		Models:           []string{modelName},
		FailureThreshold: 2,
		CooldownSeconds:  300,
		StatusCodes:      []int{429, 500, 502, 503, 504},
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	// Simulate the controller retry loop: one attempt per selection, with the
	// attempted channel recorded between attempts, threshold not yet reached.
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       new(int),
	}

	first, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	firstID := first.Id
	require.Contains(t, []int{2841, 2842}, firstID, "first attempt must stay in the top tier")

	param.MarkAttemptedChannel(firstID)

	second, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.NotEqual(t, firstID, second.Id, "same-tier retry must not re-hit the channel that just failed")
	if firstID == 2841 {
		assert.Equal(t, 2842, second.Id, "retry must reach the healthy sibling before leaving the tier")
	} else {
		assert.Equal(t, 2841, second.Id, "retry must reach the healthy sibling before leaving the tier")
	}
	assert.NotEqual(t, 2843, second.Id, "retry must not jump to the next priority tier while a sibling is healthy")
}

// Once the sibling also failed and the threshold is crossed, the cooling
// channels are filtered and the retry drops to the next priority tier.
func TestModelHealthRetryFallsThroughWhenTierExhausted(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "glm-5.3-flash"
	createChannelSelectAutoGroupsChannel(t, db, 2851, "default", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2852, "default", modelName)
	for _, id := range []int{2851, 2852} {
		require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", id).Updates(map[string]any{"priority": 700, "weight": 1}).Error)
		require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", id).Updates(map[string]any{"priority": 700, "weight": 1}).Error)
	}
	createChannelSelectAutoGroupsChannel(t, db, 2853, "default", modelName)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2853).Updates(map[string]any{"priority": 500, "weight": 1}).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 2853).Updates(map[string]any{"priority": 500, "weight": 1}).Error)
	model.InitChannelCache()
	installModelHealthPolicyForTest(t, operation_setting.ModelHealthPolicyRule{
		Name:             "层级下探",
		Enabled:          true,
		Models:           []string{modelName},
		FailureThreshold: 2,
		CooldownSeconds:  300,
		StatusCodes:      []int{429, 500, 502, 503, 504},
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       new(int),
	}

	for _, id := range []int{2851, 2852} {
		// FailureThreshold is 2: cross the threshold on both tier channels.
		RecordModelHealthFailure(nil, modelName, "default", id, http.StatusServiceUnavailable)
		RecordModelHealthFailure(nil, modelName, "default", id, http.StatusServiceUnavailable)
	}
	// Threshold reached on both tier channels: both are cooling now.

	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2853, channel.Id, "when the whole tier is cooling, selection falls through to the next tier")
}
