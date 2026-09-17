package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func installVirtualRouteForTest(t *testing.T, modelName string, route operation_setting.VirtualModelRoute) {
	t.Helper()

	retrySetting := operation_setting.GetModelRetryPolicySetting()
	originalRoutes := retrySetting.VirtualModelRoutes
	routes := make(map[string]operation_setting.VirtualModelRoute, len(originalRoutes)+1)
	for name, existing := range originalRoutes {
		routes[name] = existing
	}
	routes[modelName] = route
	retrySetting.VirtualModelRoutes = routes
	t.Cleanup(func() {
		retrySetting.VirtualModelRoutes = originalRoutes
	})
}

func createMultiModelSelectChannel(t *testing.T, db *gorm.DB, id int, group string, modelNames ...string) {
	t.Helper()

	priority := int64(0)
	weight := uint(100)
	require.NoError(t, db.Create(&model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      fmt.Sprintf("key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", id),
		Weight:   &weight,
		Models:   strings.Join(modelNames, ","),
		Group:    group,
		Priority: &priority,
	}).Error)
	createSelectChannelAbilities(t, db, id, group, modelNames...)
}

// rewriteSelectChannelModels mimics an operator saving a channel with a new
// model list, which is the only maintenance the aggregate model needs.
func rewriteSelectChannelModels(t *testing.T, db *gorm.DB, id int, group string, modelNames ...string) {
	t.Helper()

	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", id).Update("models", strings.Join(modelNames, ",")).Error)
	require.NoError(t, db.Where("channel_id = ?", id).Delete(&model.Ability{}).Error)
	createSelectChannelAbilities(t, db, id, group, modelNames...)
	model.InitChannelCache()
}

func createSelectChannelAbilities(t *testing.T, db *gorm.DB, id int, group string, modelNames ...string) {
	t.Helper()

	priority := int64(0)
	weight := uint(100)
	for _, modelName := range modelNames {
		require.NoError(t, db.Create(&model.Ability{
			Group:     group,
			Model:     modelName,
			ChannelId: id,
			Enabled:   true,
			Priority:  &priority,
			Weight:    weight,
		}).Error)
	}
}

// collectVirtualRoutePool walks every attempt of one request and returns the
// pool entries it reached, keyed by "channelId|upstreamModel".
func collectVirtualRoutePool(t *testing.T, param *RetryParam) map[string]struct{} {
	t.Helper()

	attempts := param.RetryLimit(9) + 1
	pool := make(map[string]struct{}, attempts)
	for attempt := 0; attempt < attempts; attempt++ {
		channel, _, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, channel)
		upstreamModel := common.GetContextKeyString(param.Ctx, constant.ContextKeyVirtualUpstreamModel)
		pool[fmt.Sprintf("%d|%s", channel.Id, upstreamModel)] = struct{}{}
		param.IncreaseRetry()
	}
	_, _, err := CacheGetRandomSatisfiedChannel(param)
	require.ErrorIs(t, err, model.ErrPriorityFallbackExhausted)
	return pool
}

func newVirtualRouteSourceGroup(ctx *gin.Context, modelName string) *RetryParam {
	retry := 0
	return &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}
}

func TestVirtualRoutePoolFollowsSourceChannelModelList(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-source"
	createMultiModelSelectChannel(t, db, 2501, "default", "free-a", "free-b")
	createMultiModelSelectChannel(t, db, 2502, "default", "free-c")
	// A non-source channel offers one of the same models and must stay out of the pool.
	createMultiModelSelectChannel(t, db, 2599, "default", "free-a")
	model.InitChannelCache()
	installVirtualRouteForTest(t, modelName, operation_setting.VirtualModelRoute{
		Rotation:    operation_setting.VirtualModelRouteRotationRoundRobin,
		MaxAttempts: 3,
		Sources:     []operation_setting.VirtualModelRouteSource{{ChannelId: 2501}, {ChannelId: 2502}},
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	pool := collectVirtualRoutePool(t, newVirtualRouteSourceGroup(ctx, modelName))
	assert.Equal(t, map[string]struct{}{
		"2501|free-a": {},
		"2501|free-b": {},
		"2502|free-c": {},
	}, pool, "the pool must be the source channels' models only")

	// The operator edits channel 2501 and the aggregate model follows it.
	rewriteSelectChannelModels(t, db, 2501, "default", "free-d", "free-c")
	pool = collectVirtualRoutePool(t, newVirtualRouteSourceGroup(ctx, modelName))
	assert.Equal(t, map[string]struct{}{
		"2501|free-d": {},
		"2501|free-c": {},
		"2502|free-c": {},
	}, pool)
}

func TestVirtualRoutePoolKeepsStaticTargetsAheadOfSourceChannels(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-pinned"
	createMultiModelSelectChannel(t, db, 2511, "default", "pinned-model")
	createMultiModelSelectChannel(t, db, 2512, "default", "source-model")
	model.InitChannelCache()
	installVirtualRouteForTest(t, modelName, operation_setting.VirtualModelRoute{
		Targets: []operation_setting.VirtualModelRouteTarget{{Model: "pinned-model"}},
		Sources: []operation_setting.VirtualModelRouteSource{{ChannelId: 2512}},
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	param := newVirtualRouteSourceGroup(ctx, modelName)
	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2511, channel.Id, "a pinned target stays first")

	pool := collectVirtualRoutePool(t, newVirtualRouteSourceGroup(ctx, modelName))
	assert.Equal(t, map[string]struct{}{
		"2511|pinned-model": {},
		"2512|source-model": {},
	}, pool)
}

func TestVirtualRoutePoolStopsWhenTheSourceChannelHasNoModels(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-empty-source"
	createMultiModelSelectChannel(t, db, 2521, "default", "unused-model")
	model.InitChannelCache()
	installVirtualRouteForTest(t, modelName, operation_setting.VirtualModelRoute{
		Sources: []operation_setting.VirtualModelRouteSource{{ChannelId: 2522}},
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	param := newVirtualRouteSourceGroup(ctx, modelName)
	assert.False(t, param.UsesVirtualRoute(), "a route without resolvable members falls back to the normal path")

	_, _, err := CacheGetRandomSatisfiedChannel(param)
	require.ErrorIs(t, err, model.ErrPriorityFallbackExhausted)
}

// The abilities read needs the dialect column names that model.InitDB prepares,
// so this test only covers the union added for virtual models.
func TestGetGroupsEnabledModelsIncludesVirtualModelNames(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-free-visible"
	installVirtualRouteForTest(t, modelName, operation_setting.VirtualModelRoute{
		Sources: []operation_setting.VirtualModelRouteSource{{ChannelId: 2531}},
	})
	installVirtualRouteForTest(t, "auto-free-hidden", operation_setting.VirtualModelRoute{
		Targets: []operation_setting.VirtualModelRouteTarget{{Model: "plain-model"}},
	})

	models := GetGroupsEnabledModels([]string{"default"})
	assert.Contains(t, models, modelName, "a configured virtual model needs no channel model list entry to be visible")
	assert.Contains(t, models, "auto-free-hidden")
}

func TestGetVirtualModelRouteNamesSkipsRoutesWithoutAPool(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	installVirtualRouteForTest(t, "auto-free-visible-names", operation_setting.VirtualModelRoute{
		Sources: []operation_setting.VirtualModelRouteSource{{ChannelId: 2541}},
	})
	installVirtualRouteForTest(t, "auto-free-empty-names", operation_setting.VirtualModelRoute{})

	assert.Contains(t, operation_setting.GetVirtualModelRouteNames(), "auto-free-visible-names")
	assert.NotContains(t, operation_setting.GetVirtualModelRouteNames(), "auto-free-empty-names")
}
