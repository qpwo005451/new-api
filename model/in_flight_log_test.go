package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupInFlightLogTestDB(t *testing.T) {
	t.Helper()
	originalDB := DB
	originalLogDB := LOG_DB
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalInFlightUsageLogEnabled := common.InFlightUsageLogEnabled
	originalDataExportEnabled := common.DataExportEnabled
	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		common.RedisEnabled = originalRedisEnabled
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.InFlightUsageLogEnabled = originalInFlightUsageLogEnabled
		common.DataExportEnabled = originalDataExportEnabled
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}, &User{}))
	LOG_DB = db
	DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.LogConsumeEnabled = true
	common.InFlightUsageLogEnabled = true
	common.DataExportEnabled = false
}

func newTestGinContext(requestID string, userID int) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set(common.RequestIdKey, requestID)
	c.Set("username", "tester")
	c.Set("id", userID)
	return c
}

func TestRecordPendingAndFinalizeToConsume(t *testing.T) {
	setupInFlightLogTestDB(t)
	c := newTestGinContext("req-pending-1", 7)

	RecordPendingLog(c, 7, RecordPendingLogParams{
		ChannelId: 3,
		ModelName: "gpt-test",
		TokenName: "tok",
		TokenId:   11,
		Group:     "default",
	})
	pendingID := common.GetContextKeyInt(c, constant.ContextKeyPendingLogId)
	require.Greater(t, pendingID, 0)

	RecordConsumeLog(c, 7, RecordConsumeLogParams{
		ChannelId:        3,
		PromptTokens:     10,
		CompletionTokens: 20,
		ModelName:        "gpt-test",
		TokenName:        "tok",
		Quota:            123,
		Content:          "done",
		TokenId:          11,
		UseTimeSeconds:   4,
		Group:            "default",
	})

	var row Log
	require.NoError(t, LOG_DB.First(&row, pendingID).Error)
	assert.Equal(t, LogTypeConsume, row.Type)
	assert.Equal(t, 123, row.Quota)
	assert.Equal(t, 10, row.PromptTokens)
	assert.Equal(t, 20, row.CompletionTokens)
	assert.Equal(t, "done", row.Content)
	assert.Equal(t, "default", row.Group)
	assert.Equal(t, "req-pending-1", row.RequestId)

	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestRecordPendingAndFinalizeToError(t *testing.T) {
	setupInFlightLogTestDB(t)
	c := newTestGinContext("req-pending-err", 8)
	RecordPendingLog(c, 8, RecordPendingLogParams{ModelName: "m", TokenName: "t", TokenId: 1})
	pendingID := common.GetContextKeyInt(c, constant.ContextKeyPendingLogId)
	require.Greater(t, pendingID, 0)

	RecordErrorLog(c, 8, 2, "m", "t", "upstream failed", 1, 2, false, "default", map[string]interface{}{"error_code": "x"})

	var row Log
	require.NoError(t, LOG_DB.First(&row, pendingID).Error)
	assert.Equal(t, LogTypeError, row.Type)
	assert.Equal(t, 0, row.Quota)
	assert.Contains(t, row.Content, "upstream failed")
}

func TestPendingLogSupportsDeferredChannelSelection(t *testing.T) {
	setupInFlightLogTestDB(t)
	c := newTestGinContext("req-deferred-channel", 12)
	RecordPendingLog(c, 12, RecordPendingLogParams{
		ModelName: "m",
		TokenName: "t",
		TokenId:   1,
	})
	pendingID := common.GetContextKeyInt(c, constant.ContextKeyPendingLogId)
	require.Greater(t, pendingID, 0)

	var row Log
	require.NoError(t, LOG_DB.First(&row, pendingID).Error)
	assert.Equal(t, 0, row.ChannelId)

	TouchPendingLogChannel(c, 4)
	require.NoError(t, LOG_DB.First(&row, pendingID).Error)
	assert.Equal(t, 4, row.ChannelId)
}

func TestViolationFeeDoesNotFinalizePendingRequest(t *testing.T) {
	setupInFlightLogTestDB(t)
	c := newTestGinContext("req-violation-fee", 9)
	RecordPendingLog(c, 9, RecordPendingLogParams{
		ModelName: "m",
		TokenName: "t",
		TokenId:   1,
	})
	pendingID := common.GetContextKeyInt(c, constant.ContextKeyPendingLogId)
	require.Greater(t, pendingID, 0)

	RecordConsumeLog(c, 9, RecordConsumeLogParams{
		ModelName: "m",
		TokenName: "t",
		TokenId:   1,
		Quota:     25,
		Content:   "Violation fee charged",
		Other: map[string]interface{}{
			"violation_fee": true,
		},
	})

	var pending Log
	require.NoError(t, LOG_DB.First(&pending, pendingID).Error)
	assert.Equal(t, LogTypePending, pending.Type)

	var feeLog Log
	require.NoError(t, LOG_DB.Where("type = ? AND content = ?", LogTypeConsume, "Violation fee charged").First(&feeLog).Error)
	assert.Equal(t, 25, feeLog.Quota)
	assert.NotEqual(t, pending.Id, feeLog.Id)
}

func TestIntermediateRetryErrorDoesNotFinalizePendingRequest(t *testing.T) {
	setupInFlightLogTestDB(t)
	c := newTestGinContext("req-intermediate-retry", 10)
	RecordPendingLog(c, 10, RecordPendingLogParams{
		ModelName: "m",
		TokenName: "t",
		TokenId:   1,
	})
	pendingID := common.GetContextKeyInt(c, constant.ContextKeyPendingLogId)
	require.Greater(t, pendingID, 0)

	RecordErrorLog(c, 10, 2, "m", "t", "first channel failed", 1, 2, false, "default", map[string]interface{}{
		"intermediate_retry": true,
	})

	var pending Log
	require.NoError(t, LOG_DB.First(&pending, pendingID).Error)
	assert.Equal(t, LogTypePending, pending.Type)

	var retryError Log
	require.NoError(t, LOG_DB.Where("type = ? AND content = ?", LogTypeError, "first channel failed").First(&retryError).Error)
	assert.NotEqual(t, pending.Id, retryError.Id)

	RecordConsumeLog(c, 10, RecordConsumeLogParams{
		ModelName: "m",
		TokenName: "t",
		TokenId:   1,
		Quota:     12,
		Content:   "retry succeeded",
	})

	require.NoError(t, LOG_DB.First(&pending, pendingID).Error)
	assert.Equal(t, LogTypeConsume, pending.Type)
	assert.Equal(t, "retry succeeded", pending.Content)
}

func TestPendingFinalizesWhenSwitchDisabledAfterCreation(t *testing.T) {
	setupInFlightLogTestDB(t)
	c := newTestGinContext("req-disabled-after-create", 11)
	RecordPendingLog(c, 11, RecordPendingLogParams{
		ModelName: "m",
		TokenName: "t",
		TokenId:   1,
	})
	pendingID := common.GetContextKeyInt(c, constant.ContextKeyPendingLogId)
	require.Greater(t, pendingID, 0)

	common.InFlightUsageLogEnabled = false
	common.LogConsumeEnabled = false
	RecordConsumeLog(c, 11, RecordConsumeLogParams{
		ModelName: "m",
		TokenName: "t",
		TokenId:   1,
		Quota:     7,
		Content:   "completed after switch off",
	})

	var row Log
	require.NoError(t, LOG_DB.First(&row, pendingID).Error)
	assert.Equal(t, LogTypeConsume, row.Type)
	assert.Equal(t, 7, row.Quota)
}

func TestFinalizeStaleInFlightLogs(t *testing.T) {
	setupInFlightLogTestDB(t)
	old := &Log{
		UserId:    1,
		Username:  "u",
		CreatedAt: common.GetTimestamp() - 3600,
		Type:      LogTypePending,
		Content:   "request in progress",
		RequestId: "stale-1",
	}
	require.NoError(t, LOG_DB.Create(old).Error)

	fresh := &Log{
		UserId:    1,
		Username:  "u",
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypePending,
		Content:   "request in progress",
		RequestId: "fresh-1",
	}
	require.NoError(t, LOG_DB.Create(fresh).Error)

	n, err := FinalizeStaleInFlightLogs(context.Background(), 30*60, 100)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n)

	var stale Log
	require.NoError(t, LOG_DB.First(&stale, old.Id).Error)
	assert.Equal(t, LogTypeError, stale.Type)
	assert.Equal(t, 0, stale.Quota)

	var stillPending Log
	require.NoError(t, LOG_DB.First(&stillPending, fresh.Id).Error)
	assert.Equal(t, LogTypePending, stillPending.Type)
}

func TestInFlightDisabledSkipsPending(t *testing.T) {
	setupInFlightLogTestDB(t)
	common.InFlightUsageLogEnabled = false
	c := newTestGinContext("req-disabled", 1)
	RecordPendingLog(c, 1, RecordPendingLogParams{ModelName: "m"})
	assert.Equal(t, 0, common.GetContextKeyInt(c, constant.ContextKeyPendingLogId))
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}
