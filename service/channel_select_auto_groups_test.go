package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSelectAutoGroupsTest(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRetryTimes := common.RetryTimes
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalMaxTokenAutoGroups := setting.GetMaxTokenAutoGroups()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelBalanceProtection{}))
	model.DB = db
	common.MemoryCacheEnabled = true
	common.RetryTimes = 0

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))

	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RetryTimes = originalRetryTimes
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(fmt.Sprintf("%d", originalMaxTokenAutoGroups)))

		if originalMemoryCacheEnabled && originalDB != nil &&
			originalDB.Migrator().HasTable(&model.Channel{}) && originalDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	return db
}

func createChannelSelectAutoGroupsChannel(t *testing.T, db *gorm.DB, id int, group, modelName string) {
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
		Models:   modelName,
		Group:    group,
		Priority: &priority,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

func TestCacheGetRandomSatisfiedChannelUsesTokenAutoGroupsWhenGlobalAutoIsEmpty(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-groups-runtime-model"
	createChannelSelectAutoGroupsChannel(t, db, 2101, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2102, "default", modelName)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}

	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2101, first.Id)
	assert.Equal(t, "vip", selectedGroup)
	assert.Equal(t, "vip", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
	assert.Empty(t, setting.GetAutoGroups(), "the selection must not depend on the global Auto list")

	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2102, second.Id)
	assert.Equal(t, "default", selectedGroup)
	assert.Equal(t, "default", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

func TestCacheGetRandomSatisfiedChannelExhaustsEachVirtualRouteModelInOrder(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)

	retrySetting := operation_setting.GetModelRetryPolicySetting()
	originalRoutes := retrySetting.VirtualModelRoutes
	retrySetting.VirtualModelRoutes = map[string][]operation_setting.VirtualModelRouteTarget{
		"auto-subagent-codex": {
			{Model: "gpt-5.6-luna"},
			{Model: "gpt-5.6-terra"},
		},
	}
	t.Cleanup(func() {
		retrySetting.VirtualModelRoutes = originalRoutes
	})

	createChannelSelectAutoGroupsChannel(t, db, 2201, "default", "gpt-5.6-luna")
	createChannelSelectAutoGroupsChannel(t, db, 2202, "default", "gpt-5.6-luna")
	createChannelSelectAutoGroupsChannel(t, db, 2203, "default", "gpt-5.6-terra")
	createChannelSelectAutoGroupsChannel(t, db, 2204, "default", "gpt-5.6-terra")

	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2201).Updates(map[string]any{"priority": 500, "weight": 1}).Error)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2202).Updates(map[string]any{"priority": 498, "weight": 5}).Error)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2203).Updates(map[string]any{"priority": 496, "weight": 1}).Error)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2204).Updates(map[string]any{"priority": 100, "weight": 1}).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 2201).Updates(map[string]any{"priority": 500, "weight": 1}).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 2202).Updates(map[string]any{"priority": 498, "weight": 5}).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 2203).Updates(map[string]any{"priority": 496, "weight": 1}).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 2204).Updates(map[string]any{"priority": 100, "weight": 1}).Error)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   "auto-subagent-codex",
		RequestPath: "/v1/responses",
		Retry:       &retry,
	}

	assert.Equal(t, 3, param.RetryLimit(common.RetryTimes))

	expected := []struct {
		channelID     int
		upstreamModel string
	}{
		{2201, "gpt-5.6-luna"},
		{2202, "gpt-5.6-luna"},
		{2203, "gpt-5.6-terra"},
		{2204, "gpt-5.6-terra"},
	}
	for index, want := range expected {
		channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, channel)
		assert.Equal(t, want.channelID, channel.Id)
		assert.Equal(t, "default", selectedGroup)
		assert.Equal(t, want.upstreamModel, common.GetContextKeyString(ctx, constant.ContextKeyVirtualUpstreamModel))
		if index < len(expected)-1 {
			param.IncreaseRetry()
		}
	}

	param.IncreaseRetry()
	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.ErrorIs(t, err, model.ErrPriorityFallbackExhausted)
	assert.Nil(t, channel)
}

func TestCacheGetRandomSatisfiedChannelVirtualRouteIgnoresDisabledChannels(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)

	retrySetting := operation_setting.GetModelRetryPolicySetting()
	originalRoutes := retrySetting.VirtualModelRoutes
	retrySetting.VirtualModelRoutes = map[string][]operation_setting.VirtualModelRouteTarget{
		"auto-subagent": {
			{Model: "gpt-5.6-luna"},
			{Model: "grok-4.5"},
			{Model: "deepseek-v4-flash"},
		},
	}
	t.Cleanup(func() {
		retrySetting.VirtualModelRoutes = originalRoutes
	})

	createChannelSelectAutoGroupsChannel(t, db, 2301, "default", "gpt-5.6-luna")
	createChannelSelectAutoGroupsChannel(t, db, 2302, "default", "gpt-5.6-luna")
	createChannelSelectAutoGroupsChannel(t, db, 2303, "default", "grok-4.5")
	createChannelSelectAutoGroupsChannel(t, db, 2304, "default", "deepseek-v4-flash")
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2302).Update("status", common.ChannelStatusManuallyDisabled).Error)
	require.NoError(t, db.Model(&model.Ability{}).Where("channel_id = ?", 2302).Update("enabled", false).Error)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   "auto-subagent",
		RequestPath: "/v1/responses",
		Retry:       &retry,
	}

	var channelIDs []int
	for index := 0; index <= param.RetryLimit(common.RetryTimes); index++ {
		channel, _, err := CacheGetRandomSatisfiedChannel(param)
		require.NoError(t, err)
		require.NotNil(t, channel)
		channelIDs = append(channelIDs, channel.Id)
		param.IncreaseRetry()
	}
	assert.Equal(t, []int{2301, 2303, 2304}, channelIDs)
}

func TestRetryParamVirtualRouteWithNoAvailableChannelsExhaustsImmediately(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)

	retrySetting := operation_setting.GetModelRetryPolicySetting()
	originalRoutes := retrySetting.VirtualModelRoutes
	retrySetting.VirtualModelRoutes = map[string][]operation_setting.VirtualModelRouteTarget{
		"auto-subagent": {
			{Model: "gpt-5.6-luna"},
			{Model: "grok-4.5"},
			{Model: "deepseek-v4-flash"},
		},
	}
	t.Cleanup(func() {
		retrySetting.VirtualModelRoutes = originalRoutes
	})
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   "auto-subagent",
		RequestPath: "/v1/responses",
		Retry:       &retry,
	}

	assert.True(t, param.UsesVirtualRoute())
	assert.Equal(t, 0, param.RetryLimit(common.RetryTimes))
	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.ErrorIs(t, err, model.ErrPriorityFallbackExhausted)
	assert.Nil(t, channel)
}

func TestCacheGetRandomSatisfiedChannelVirtualRouteDeduplicatesChannelsAcrossAutoGroups(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)

	retrySetting := operation_setting.GetModelRetryPolicySetting()
	originalRoutes := retrySetting.VirtualModelRoutes
	retrySetting.VirtualModelRoutes = map[string][]operation_setting.VirtualModelRouteTarget{
		"auto-subagent-codex": {
			{Model: "gpt-5.6-luna"},
			{Model: "gpt-5.6-terra"},
		},
	}
	t.Cleanup(func() {
		retrySetting.VirtualModelRoutes = originalRoutes
	})

	createChannelSelectAutoGroupsChannel(t, db, 2501, "vip", "gpt-5.6-luna")
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.6-luna",
		ChannelId: 2501,
		Enabled:   true,
		Priority:  common.GetPointer[int64](0),
		Weight:    100,
	}).Error)
	createChannelSelectAutoGroupsChannel(t, db, 2502, "default", "gpt-5.6-terra")
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   "auto-subagent-codex",
		RequestPath: "/v1/responses",
		Retry:       &retry,
	}

	assert.Equal(t, 1, param.RetryLimit(common.RetryTimes))
	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2501, first.Id)
	assert.Equal(t, "vip", selectedGroup)

	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2502, second.Id)
	assert.Equal(t, "default", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelVirtualRouteMapsReasoningEffortPerTarget(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)

	retrySetting := operation_setting.GetModelRetryPolicySetting()
	originalRoutes := retrySetting.VirtualModelRoutes
	retrySetting.VirtualModelRoutes = map[string][]operation_setting.VirtualModelRouteTarget{
		"auto-subagent": {
			{Model: "gpt-5.6-luna"},
			{
				Model: "grok-4.5",
				ReasoningEffortMap: map[string]string{
					"max": "high",
				},
			},
		},
	}
	t.Cleanup(func() {
		retrySetting.VirtualModelRoutes = originalRoutes
	})

	createChannelSelectAutoGroupsChannel(t, db, 2601, "default", "gpt-5.6-luna")
	createChannelSelectAutoGroupsChannel(t, db, 2602, "default", "grok-4.5")
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(
		"POST",
		"/v1/responses",
		strings.NewReader(`{"model":"auto-subagent","reasoning":{"effort":"max"}}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   "auto-subagent",
		RequestPath: "/v1/responses",
		Retry:       &retry,
	}

	first, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2601, first.Id)
	assert.Equal(t, "max", common.GetContextKeyString(ctx, constant.ContextKeyVirtualReasoningEffort))

	param.IncreaseRetry()
	second, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2602, second.Id)
	assert.Equal(t, "high", common.GetContextKeyString(ctx, constant.ContextKeyVirtualReasoningEffort))
}
