package controller

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetChannelUsesVirtualRouteOnFirstRelayAttempt(t *testing.T) {
	previousDB := model.DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelBalanceProtection{}))
	model.DB = db
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		if previousMemoryCacheEnabled && previousDB != nil {
			model.InitChannelCache()
		}
	})

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

	priority := int64(500)
	weight := uint(1)
	channel := &model.Channel{
		Id:       2401,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "key-2401",
		Status:   common.ChannelStatusEnabled,
		Name:     "luna-channel",
		Weight:   &weight,
		Models:   "gpt-5.6-luna",
		Group:    "default",
		Priority: &priority,
	}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     "default",
		Model:     "gpt-5.6-luna",
		ChannelId: channel.Id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	retry := 0
	param := &service.RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   "auto-subagent-codex",
		RequestPath: "/v1/responses",
		Retry:       &retry,
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "auto-subagent-codex",
		TokenGroup:      "default",
	}

	selected, channelErr := getChannel(ctx, info, param)
	require.Nil(t, channelErr)
	require.NotNil(t, selected)
	assert.Equal(t, 2401, selected.Id)
	assert.Equal(t, "gpt-5.6-luna", common.GetContextKeyString(ctx, constant.ContextKeyVirtualUpstreamModel))
}
