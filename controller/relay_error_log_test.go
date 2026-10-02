package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Relay records a pending (type=8) usage log before billing preparation runs.
// An early rejection there - sensitive words, token estimation, pricing - must
// still produce a terminal error log, otherwise the console shows the request
// as pending until the stale sweeper rewrites it much later.
func TestRelayFinalizesPendingLogWhenBillingPreparationRejects(t *testing.T) {
	fixture := setupVirtualPoolE2EFixture(t, false, "round_robin", false)

	previousConsumeEnabled := common.LogConsumeEnabled
	previousInFlightEnabled := common.InFlightUsageLogEnabled
	common.LogConsumeEnabled = true
	common.InFlightUsageLogEnabled = true
	t.Cleanup(func() {
		common.LogConsumeEnabled = previousConsumeEnabled
		common.InFlightUsageLogEnabled = previousInFlightEnabled
	})
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))

	previousSensitiveWords := setting.SensitiveWords
	previousCheckEnabled := setting.CheckSensitiveEnabled
	previousPromptCheckEnabled := setting.CheckSensitiveOnPromptEnabled
	setting.SensitiveWords = []string{"relay-pending-sensitive-word"}
	setting.CheckSensitiveEnabled = true
	setting.CheckSensitiveOnPromptEnabled = true
	t.Cleanup(func() {
		setting.SensitiveWords = previousSensitiveWords
		setting.CheckSensitiveEnabled = previousCheckEnabled
		setting.CheckSensitiveOnPromptEnabled = previousPromptCheckEnabled
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/completions",
		strings.NewReader(`{"model":"vpool-upstream-a","messages":[{"role":"user","content":"say relay-pending-sensitive-word"}],"max_tokens":1}`),
	)
	request.Header.Set("Content-Type", "application/json")
	fixture.router.ServeHTTP(recorder, request)

	require.NotEqual(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "sensitive words detected")

	requestIdMatch := regexp.MustCompile(`request id: ([A-Za-z0-9]+)`).FindStringSubmatch(recorder.Body.String())
	require.Len(t, requestIdMatch, 2, "relay error response must carry the request id")

	var stored model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", requestIdMatch[1]).First(&stored).Error)
	assert.Equal(t, model.LogTypeError, stored.Type)
	assert.Contains(t, stored.Content, "sensitive words detected")
	// The pending row already knew the selected channel; the early-return
	// finalization must not clobber it with zero.
	assert.Contains(t, []int{9401, 9402}, stored.ChannelId, "finalized row must keep the selected channel")

	var pendingCount int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypePending).Count(&pendingCount).Error)
	assert.Zero(t, pendingCount, "the pending row must be finalized in place")
}

func TestProcessChannelErrorUsesSnapshotWithoutLeakingChannelMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousErrorLogEnabled := constant.ErrorLogEnabled

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB, model.LOG_DB = database, database
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		constant.ErrorLogEnabled = previousErrorLogEnabled
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, database.Create(&model.User{Id: 7, Username: "log-owner", Group: "default"}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 7)
	ctx.Set("username", "log-owner")
	ctx.Set("token_name", "test-token")
	ctx.Set("token_id", 11)
	ctx.Set("original_model", "gpt-test")
	ctx.Set("group", "default")
	ctx.Set("channel_id", 202)
	ctx.Set("channel_name", "mutable-context-channel")
	ctx.Set("channel_type", 9)
	ctx.Set("use_channel", []string{"101"})
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now().Add(-time.Second))

	channelSnapshot := types.ChannelError{
		ChannelId:   101,
		ChannelType: 1,
		ChannelName: "snapshot-channel",
		AutoBan:     false,
	}
	apiErr := types.NewOpenAIError(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)

	processChannelError(ctx, channelSnapshot, apiErr, nil, false)

	var stored model.Log
	require.NoError(t, database.First(&stored).Error)
	assert.Equal(t, channelSnapshot.ChannelId, stored.ChannelId)
	storedOther, err := common.StrToMap(stored.Other)
	require.NoError(t, err)
	assert.Equal(t, float64(http.StatusBadGateway), storedOther["status_code"])
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, storedOther, key)
	}
	adminInfo, ok := storedOther["admin_info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"101"}, adminInfo["use_channel"])

	logs, total, err := model.GetUserLogs(7, model.LogTypeError, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, channelSnapshot.ChannelId, logs[0].ChannelId)
	assert.Empty(t, logs[0].ChannelName)
	userOther, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	assert.NotContains(t, userOther, "admin_info")
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, userOther, key)
	}
}
