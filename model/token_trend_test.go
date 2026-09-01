package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTokenTrendTestDB(t *testing.T) {
	t.Helper()
	originalLogDB := LOG_DB
	originalLogDatabaseType := common.LogDatabaseType()
	t.Cleanup(func() {
		LOG_DB = originalLogDB
		common.SetLogDatabaseType(originalLogDatabaseType)
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
}

func seedTokenTrendLog(t *testing.T, userId int, createdAt int64, modelName string, prompt, completion int, other string) {
	t.Helper()
	log := &Log{
		UserId:           userId,
		CreatedAt:        createdAt,
		Type:             LogTypeConsume,
		Username:         "tester",
		ModelName:        modelName,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		Other:            other,
	}
	require.NoError(t, LOG_DB.Create(log).Error)
}

func setupTokenTrendChannelDB(t *testing.T) {
	t.Helper()
	setupTokenTrendTestDB(t)

	originalDB := DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	t.Cleanup(func() {
		DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}))
	DB = db
	common.MemoryCacheEnabled = false
}

func seedTokenTrendLogOnChannel(t *testing.T, userId int, channelID int, createdAt int64, modelName string, prompt, completion int, other string) {
	t.Helper()
	log := &Log{
		UserId:           userId,
		ChannelId:        channelID,
		CreatedAt:        createdAt,
		Type:             LogTypeConsume,
		Username:         "tester",
		ModelName:        modelName,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		Other:            other,
	}
	require.NoError(t, LOG_DB.Create(log).Error)
}

// seedTokenTrendOllamaAndNormalChannels creates one Ollama channel (id 1) and
// one cache-reporting channel (id 2) in the main DB, then logs 900 prompt
// tokens without cache fields on the Ollama channel and 1000 prompt tokens
// with cache fields on the normal channel, both inside the same hourly bucket.
func seedTokenTrendOllamaAndNormalChannels(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.Create(&Channel{Id: 1, Type: constant.ChannelTypeOllama, Name: "ollama", Key: "ollama-key", Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, DB.Create(&Channel{Id: 2, Type: 1, Name: "openai", Key: "openai-key", Status: common.ChannelStatusEnabled}).Error)
	seedTokenTrendLogOnChannel(t, 7, 1, 100, "m1", 900, 90, `{}`)
	seedTokenTrendLogOnChannel(t, 7, 2, 150, "m1", 1000, 100, `{"cache_tokens":800,"cache_write_tokens":50}`)
}

func TestGetTokenTrendAggregatesHourlyBucketsWithCacheTokens(t *testing.T) {
	setupTokenTrendTestDB(t)

	// Bucket 100: two logs, one with cache read + write in other.
	seedTokenTrendLog(t, 7, 100, "m1", 1000, 100, `{"cache_tokens":400,"cache_write_tokens":150,"frt":12}`)
	seedTokenTrendLog(t, 7, 200, "m2", 500, 50, `{"cache_tokens":0}`)
	// Bucket 3700: next hour.
	seedTokenTrendLog(t, 7, 3700, "m1", 800, 20, `{"cache_creation_tokens":60}`)
	// Out of range: must be excluded.
	seedTokenTrendLog(t, 7, 10000, "m1", 999, 999, `{}`)
	// Other user: must be excluded by the user scope.
	seedTokenTrendLog(t, 9, 150, "m1", 777, 777, `{"cache_tokens":333}`)
	// Non-consume log type: must be excluded.
	nonConsume := &Log{UserId: 7, CreatedAt: 160, Type: LogTypeManage, ModelName: "m1", PromptTokens: 55, CompletionTokens: 55}
	require.NoError(t, LOG_DB.Create(nonConsume).Error)

	points, err := GetTokenTrend(7, 0, 7200, "")
	require.NoError(t, err)
	require.Len(t, points, 2)

	first := points[0]
	assert.Equal(t, int64(0), first.CreatedAt)
	assert.Equal(t, int64(2), first.Requests)
	assert.Equal(t, int64(1500), first.PromptTokens)
	assert.Equal(t, int64(150), first.CompletionTokens)
	assert.Equal(t, int64(400), first.CacheRead)
	assert.Equal(t, int64(150), first.CacheWrite)

	second := points[1]
	assert.Equal(t, int64(3600), second.CreatedAt)
	assert.Equal(t, int64(1), second.Requests)
	assert.Equal(t, int64(800), second.PromptTokens)
	assert.Equal(t, int64(20), second.CompletionTokens)
	assert.Equal(t, int64(0), second.CacheRead)
	// cache_creation_tokens is the legacy cache-write field and must be counted.
	assert.Equal(t, int64(60), second.CacheWrite)
}

func TestGetTokenTrendAdminScopeReturnsAllUsers(t *testing.T) {
	setupTokenTrendTestDB(t)

	seedTokenTrendLog(t, 7, 100, "m1", 1000, 100, `{"cache_tokens":400}`)
	seedTokenTrendLog(t, 9, 150, "m1", 777, 0, `{"cache_tokens":111}`)

	points, err := GetTokenTrend(0, 0, 7200, "")
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.Equal(t, int64(2), points[0].Requests)
	assert.Equal(t, int64(1777), points[0].PromptTokens)
	assert.Equal(t, int64(511), points[0].CacheRead)
}

func TestGetTokenTrendClampsMalformedCacheFields(t *testing.T) {
	setupTokenTrendTestDB(t)

	// Negative and oversized cache values must not produce negative or
	// prompt-exceeding series.
	seedTokenTrendLog(t, 7, 100, "m1", 1000, 50, `{"cache_tokens":5000,"cache_write_tokens":-20}`)

	points, err := GetTokenTrend(7, 0, 7200, "")
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.Equal(t, int64(1000), points[0].CacheRead, "cache read clamped to prompt_tokens")
	assert.Equal(t, int64(0), points[0].CacheWrite, "negative cache write clamped to 0")
}

func TestGetTokenTrendModelFilter(t *testing.T) {
	setupTokenTrendTestDB(t)

	seedTokenTrendLog(t, 7, 100, "m1", 1000, 100, `{"cache_tokens":400}`)
	seedTokenTrendLog(t, 7, 150, "m2", 2000, 50, `{"cache_tokens":10}`)

	all, err := GetTokenTrend(7, 0, 7200, "")
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, int64(2), all[0].Requests)
	assert.Equal(t, int64(3000), all[0].PromptTokens)

	onlyM1, err := GetTokenTrend(7, 0, 7200, "m1")
	require.NoError(t, err)
	require.Len(t, onlyM1, 1)
	assert.Equal(t, int64(1), onlyM1[0].Requests)
	assert.Equal(t, int64(1000), onlyM1[0].PromptTokens)
	assert.Equal(t, int64(400), onlyM1[0].CacheRead)

	// Unknown model yields empty result, not an error.
	none, err := GetTokenTrend(7, 0, 7200, "no-such-model")
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestGetTokenTrendModels(t *testing.T) {
	setupTokenTrendTestDB(t)

	seedTokenTrendLog(t, 7, 100, "m2", 1000, 100, `{}`)
	seedTokenTrendLog(t, 7, 150, "m1", 500, 50, `{}`)
	seedTokenTrendLog(t, 7, 200, "m1", 500, 50, `{}`)
	// Other user's model must not leak into user-scoped list.
	seedTokenTrendLog(t, 9, 120, "m3", 500, 50, `{}`)
	// Out-of-range model excluded.
	seedTokenTrendLog(t, 7, 10000, "m4", 500, 50, `{}`)

	names, err := GetTokenTrendModels(7, 0, 7200)
	require.NoError(t, err)
	assert.Equal(t, []string{"m1", "m2"}, names)

	allNames, err := GetTokenTrendModels(0, 0, 7200)
	require.NoError(t, err)
	assert.Equal(t, []string{"m1", "m2", "m3"}, allNames)
}

func TestGetTokenTrendFallbackParsesOtherInGo(t *testing.T) {
	setupTokenTrendTestDB(t)

	// Force the fallback path by hiding the JSON expressions behind an
	// unsupported dialect.
	originalLogDB := LOG_DB
	originalLogDatabaseType := common.LogDatabaseType()
	common.SetLogDatabaseType(common.DatabaseType("unsupported"))
	t.Cleanup(func() {
		LOG_DB = originalLogDB
		common.SetLogDatabaseType(originalLogDatabaseType)
	})

	seedTokenTrendLog(t, 7, 100, "m1", 1000, 100, `{"cache_tokens":400,"cache_creation_tokens":60}`)
	// Malformed other must be skipped, not fail the query.
	seedTokenTrendLog(t, 7, 120, "m1", 300, 30, `not-json`)

	points, err := GetTokenTrend(7, 0, 7200, "")
	require.NoError(t, err)
	require.Len(t, points, 1)
	assert.Equal(t, int64(2), points[0].Requests)
	assert.Equal(t, int64(1300), points[0].PromptTokens)
	assert.Equal(t, int64(400), points[0].CacheRead)
	assert.Equal(t, int64(60), points[0].CacheWrite)
}

func TestGetTokenTrendCachePromptExcludesOllamaChannel(t *testing.T) {
	setupTokenTrendChannelDB(t)
	seedTokenTrendOllamaAndNormalChannels(t)

	points, err := GetTokenTrend(7, 0, 7200, "")
	require.NoError(t, err)
	require.Len(t, points, 1)

	// Requests/prompt/completion stay full-scope; only the cache hit-rate
	// denominator drops the Ollama channel's prompt tokens.
	assert.Equal(t, int64(2), points[0].Requests)
	assert.Equal(t, int64(1900), points[0].PromptTokens)
	assert.Equal(t, int64(1000), points[0].CachePrompt)
	assert.Equal(t, int64(800), points[0].CacheRead)
	assert.Equal(t, int64(50), points[0].CacheWrite)
}

func TestGetTokenTrendFallbackCachePromptExcludesOllamaChannel(t *testing.T) {
	setupTokenTrendChannelDB(t)
	seedTokenTrendOllamaAndNormalChannels(t)

	// Force the Go-parsing fallback by hiding the JSON expressions behind an
	// unsupported dialect; cache_prompt still comes from the SQL expression
	// shared by both paths.
	common.SetLogDatabaseType(common.DatabaseType("unsupported"))

	points, err := GetTokenTrend(7, 0, 7200, "")
	require.NoError(t, err)
	require.Len(t, points, 1)

	assert.Equal(t, int64(2), points[0].Requests)
	assert.Equal(t, int64(1900), points[0].PromptTokens)
	assert.Equal(t, int64(1000), points[0].CachePrompt)
	assert.Equal(t, int64(800), points[0].CacheRead)
	assert.Equal(t, int64(50), points[0].CacheWrite)
}

func TestGetTokenTrendCachePromptMatchesPromptTokensWithoutOllama(t *testing.T) {
	setupTokenTrendChannelDB(t)

	// No Ollama channel exists in the main DB: the cache hit-rate denominator
	// must stay identical to the full prompt total.
	require.NoError(t, DB.Create(&Channel{Id: 2, Type: 1, Name: "openai", Key: "openai-key", Status: common.ChannelStatusEnabled}).Error)
	seedTokenTrendLogOnChannel(t, 7, 2, 100, "m1", 900, 90, `{}`)
	seedTokenTrendLogOnChannel(t, 7, 2, 150, "m1", 1000, 100, `{"cache_tokens":800,"cache_write_tokens":50}`)

	points, err := GetTokenTrend(7, 0, 7200, "")
	require.NoError(t, err)
	require.Len(t, points, 1)

	assert.Equal(t, int64(2), points[0].Requests)
	assert.Equal(t, int64(1900), points[0].PromptTokens)
	assert.Equal(t, int64(1900), points[0].CachePrompt)
	assert.Equal(t, int64(800), points[0].CacheRead)
	assert.Equal(t, int64(50), points[0].CacheWrite)
}
