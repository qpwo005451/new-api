package model

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupLogClientInfoTestDB(t *testing.T) {
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

func newClientInfoTestContext(userAgent string, remoteAddr string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	req.Header.Set("User-Agent", userAgent)
	req.RemoteAddr = remoteAddr
	c.Request = req
	c.Set(common.RequestIdKey, "req-client-info")
	c.Set("username", "tester")
	c.Set("id", 7)
	return c
}

func requireClientInfoLogRow(t *testing.T, logType int) Log {
	t.Helper()
	var row Log
	err := LOG_DB.Where("request_id = ? AND type = ?", "req-client-info", logType).
		Order("id desc").First(&row).Error
	require.NoError(t, err)
	return row
}

// TestRecordConsumeLogCapturesAdminClientInfo verifies consume logs record
// the client IP and User-Agent under other.admin_info (admin-only audit
// fields), merged into any existing admin_info, while the user-visible Ip
// column stays gated by the user's own record_ip_log preference and
// formatUserLogs strips the captured info for non-admin views.
func TestRecordConsumeLogCapturesAdminClientInfo(t *testing.T) {
	setupLogClientInfoTestDB(t)
	c := newClientInfoTestContext("codex-agent/1.2", "203.0.113.7:53110")

	RecordConsumeLog(c, 7, RecordConsumeLogParams{
		ChannelId:        3,
		PromptTokens:     10,
		CompletionTokens: 20,
		ModelName:        "deepseek-v4-flash",
		TokenName:        "tok",
		Quota:            123,
		Content:          "done",
		TokenId:          11,
		UseTimeSeconds:   4,
		Group:            "default",
		Other: map[string]interface{}{
			"model_price": 0.004,
			"admin_info": map[string]interface{}{
				"use_channel": []int{3},
			},
		},
	})

	row := requireClientInfoLogRow(t, LogTypeConsume)
	other, err := common.StrToMap(row.Other)
	require.NoError(t, err)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok, "admin_info must exist on consume logs")
	assert.Equal(t, "203.0.113.7", adminInfo["ip"])
	assert.Equal(t, "codex-agent/1.2", adminInfo["user_agent"])
	require.Contains(t, adminInfo, "use_channel", "existing admin_info fields must be preserved")
	assert.Equal(t, 0.004, other["model_price"])
	assert.Empty(t, row.Ip, "Ip column stays empty without the user's record_ip_log opt-in")

	formatUserLogs([]*Log{&row}, 0)
	userOther, err := common.StrToMap(row.Other)
	require.NoError(t, err)
	_, hasAdminInfo := userOther["admin_info"]
	assert.False(t, hasAdminInfo, "captured client info must be stripped for non-admin views")
	assert.Contains(t, userOther, "model_price")
}

func TestRecordConsumeLogCapturesClientInfoWithoutOther(t *testing.T) {
	setupLogClientInfoTestDB(t)
	c := newClientInfoTestContext("curl/8.7.1", "198.51.100.23:44300")

	RecordConsumeLog(c, 7, RecordConsumeLogParams{
		ChannelId:      3,
		ModelName:      "m",
		TokenName:      "t",
		Quota:          1,
		Content:        "done",
		UseTimeSeconds: 1,
		Group:          "default",
		Other:          nil,
	})

	row := requireClientInfoLogRow(t, LogTypeConsume)
	other, err := common.StrToMap(row.Other)
	require.NoError(t, err)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "198.51.100.23", adminInfo["ip"])
	assert.Equal(t, "curl/8.7.1", adminInfo["user_agent"])
}

func TestRecordErrorLogCapturesAdminClientInfo(t *testing.T) {
	setupLogClientInfoTestDB(t)
	c := newClientInfoTestContext("Go-http-client/1.1", "192.0.2.9:22000")

	RecordErrorLog(c, 7, 2, "deepseek-v4-flash", "tok", "upstream failed", 1, 2, false, "default", nil)

	row := requireClientInfoLogRow(t, LogTypeError)
	other, err := common.StrToMap(row.Other)
	require.NoError(t, err)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok, "admin_info must be created for error logs without Other")
	assert.Equal(t, "192.0.2.9", adminInfo["ip"])
	assert.Equal(t, "Go-http-client/1.1", adminInfo["user_agent"])
}

func TestRecordPendingLogCapturesAdminClientInfo(t *testing.T) {
	setupLogClientInfoTestDB(t)
	c := newClientInfoTestContext("codex-agent/2.0", "203.0.113.7:53111")

	RecordPendingLog(c, 7, RecordPendingLogParams{
		ChannelId: 3,
		ModelName: "m",
		TokenName: "t",
		TokenId:   1,
		Group:     "default",
	})

	row := requireClientInfoLogRow(t, LogTypePending)
	other, err := common.StrToMap(row.Other)
	require.NoError(t, err)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "203.0.113.7", adminInfo["ip"])
	assert.Equal(t, "codex-agent/2.0", adminInfo["user_agent"])
}

func TestRecordConsumeLogWithoutRequestOmitsClientInfo(t *testing.T) {
	setupLogClientInfoTestDB(t)
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("username", "tester")
	c.Set("id", 7)

	RecordConsumeLog(c, 7, RecordConsumeLogParams{
		ChannelId:      3,
		ModelName:      "m",
		TokenName:      "t",
		Quota:          1,
		Content:        "done",
		UseTimeSeconds: 1,
		Group:          "default",
		Other:          map[string]interface{}{"model_price": 0.004},
	})

	var row Log
	require.NoError(t, LOG_DB.Where("type = ?", LogTypeConsume).First(&row).Error)
	other, err := common.StrToMap(row.Other)
	require.NoError(t, err)
	_, hasAdminInfo := other["admin_info"]
	assert.False(t, hasAdminInfo, "no client info can be captured without an HTTP request")
}

// TestRecordConsumeLogCapturesClientIdentityHeaders verifies app-declared
// identity headers (OpenRouter-style X-Title/HTTP-Referer and the OpenAI SDK
// x-stainless runtime hints) land in admin_info alongside the IP/User-Agent,
// with over-long values truncated to safe lengths.
func TestRecordConsumeLogCapturesClientIdentityHeaders(t *testing.T) {
	setupLogClientInfoTestDB(t)
	c := newClientInfoTestContext("OpenAI/JS 6.47.0", "10.0.0.180:41000")
	c.Request.Header.Set("X-Title", "Prime Agent")
	c.Request.Header.Set("HTTP-Referer", "https://prime-agent.example.com/console")
	c.Request.Header.Set("X-Stainless-Runtime", "node")
	c.Request.Header.Set("X-Stainless-Runtime-Version", "22.5.0")

	RecordConsumeLog(c, 7, RecordConsumeLogParams{
		ChannelId:      3,
		ModelName:      "m",
		TokenName:      "t",
		Quota:          1,
		Content:        "done",
		UseTimeSeconds: 1,
		Group:          "default",
		Other:          map[string]interface{}{},
	})

	row := requireClientInfoLogRow(t, LogTypeConsume)
	other, err := common.StrToMap(row.Other)
	require.NoError(t, err)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Prime Agent", adminInfo["client_title"])
	assert.Equal(t, "https://prime-agent.example.com/console", adminInfo["client_referer"])
	assert.Equal(t, "node", adminInfo["client_runtime"])
	assert.Equal(t, "22.5.0", adminInfo["client_runtime_version"])
	assert.Equal(t, "OpenAI/JS 6.47.0", adminInfo["user_agent"])
}

func TestRecordConsumeLogTruncatesOversizedClientInfo(t *testing.T) {
	setupLogClientInfoTestDB(t)
	c := newClientInfoTestContext(strings.Repeat("U", 500), "10.0.0.180:41001")
	c.Request.Header.Set("X-Title", strings.Repeat("标", 200))
	c.Request.Header.Set("HTTP-Referer", strings.Repeat("https://r.example/", 40))

	RecordConsumeLog(c, 7, RecordConsumeLogParams{
		ChannelId:      3,
		ModelName:      "m",
		TokenName:      "t",
		Quota:          1,
		Content:        "done",
		UseTimeSeconds: 1,
		Group:          "default",
		Other:          map[string]interface{}{},
	})

	row := requireClientInfoLogRow(t, LogTypeConsume)
	other, err := common.StrToMap(row.Other)
	require.NoError(t, err)
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Len(t, adminInfo["user_agent"], 300)
	title, _ := adminInfo["client_title"].(string)
	assert.Len(t, []rune(title), 120, "client_title must truncate to 120 runes")
	assert.Len(t, adminInfo["client_referer"], 300)
}
