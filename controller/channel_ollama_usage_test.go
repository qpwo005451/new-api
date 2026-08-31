package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOllamaUsageResponse(t *testing.T) {
	// Captured verbatim from GET https://ollama.com/api/usage on 2026-08-30.
	payload := `{
		"activity": {
			"cost": "0.00000",
			"period": {"type": "last_4_weeks", "starting_at": "2026-08-03T00:00:00Z", "ending_at": "2026-08-30T01:18:22Z"},
			"models": []
		},
		"limits": {
			"session": {
				"usage": 12.5,
				"models": [
					{"name": "deepseek-v4-flash:0731", "request_count": 3},
					{"name": "gpt-oss:20b", "request_count": 7}
				]
			},
			"weekly": {
				"usage": 40,
				"models": [
					{"name": "deepseek-v4-flash:0731", "request_count": 9},
					{"name": "gpt-oss:20b", "request_count": 11}
				]
			}
		}
	}`

	var parsed ollamaUsageResponse
	// Same unmarshal path as the handler, exercising the DTO mapping.
	require.NoError(t, common.Unmarshal([]byte(payload), &parsed))

	assert.Equal(t, "0.00000", parsed.Activity.Cost)
	assert.JSONEq(t, `{"type": "last_4_weeks", "starting_at": "2026-08-03T00:00:00Z", "ending_at": "2026-08-30T01:18:22Z"}`, string(parsed.Activity.Period))
	assert.Equal(t, 12.5, parsed.Limits.Session.Usage)
	assert.Equal(t, 40.0, parsed.Limits.Weekly.Usage)
	require.Len(t, parsed.Limits.Session.Models, 2)
	assert.Equal(t, "deepseek-v4-flash:0731", parsed.Limits.Session.Models[0].Name)
	assert.EqualValues(t, 3, parsed.Limits.Session.Models[0].RequestCount)
	assert.EqualValues(t, 7, parsed.Limits.Session.Models[1].RequestCount)
	require.Len(t, parsed.Limits.Weekly.Models, 2)
	assert.EqualValues(t, 11, parsed.Limits.Weekly.Models[1].RequestCount)
}

func TestOllamaModelUsageLevel(t *testing.T) {
	// Levels observed on ollama.com model pages on 2026-08-30.
	assert.Equal(t, 1, ollamaModelUsageLevel("gpt-oss:20b"))
	assert.Equal(t, 1, ollamaModelUsageLevel("gemma4:31b"))
	assert.Equal(t, 1, ollamaModelUsageLevel("nemotron-3-nano:30b"))
	assert.Equal(t, 2, ollamaModelUsageLevel("glm-5.3-flash"))
	assert.Equal(t, 2, ollamaModelUsageLevel("deepseek-v4-flash:0731"))
	assert.Equal(t, 4, ollamaModelUsageLevel("deepseek-v4-pro:0813"))
	// Unknown or local-variant models carry no level.
	assert.Equal(t, 0, ollamaModelUsageLevel("kimi-k3"))
	assert.Equal(t, 0, ollamaModelUsageLevel("gpt-oss"))
}

func TestFoldChannelModelStats(t *testing.T) {
	// Mirrors the production ollama_pro channel: "deepseek-v4-flash" is
	// mapped to the upstream "deepseek-v4-flash:0731", so both log names
	// meter the same upstream model and must merge. Logs of a deleted
	// channel reusing the same id (gpt-5.6-luna) must be dropped because
	// the current channel does not serve that model.
	mapping := map[string]string{"deepseek-v4-flash": "deepseek-v4-flash:0731"}
	served := map[string]bool{
		"glm-5.3-flash":          true,
		"deepseek-v4-flash:0731": true,
		"deepseek-v4-flash":      true,
		"gpt-oss:20b":            true,
	}
	stats := []model.ModelTokenStat{
		{ModelName: " deepseek-v4-flash ", Requests: 420, PromptTokens: 100, CompletionTokens: 200},
		{ModelName: "deepseek-v4-flash:0731", Requests: 2, PromptTokens: 5, CompletionTokens: 16},
		{ModelName: "glm-5.3-flash", Requests: 131, PromptTokens: 300, CompletionTokens: 400},
		{ModelName: "gpt-5.6-luna", Requests: 2, PromptTokens: 2, CompletionTokens: 2},
	}

	folded := foldChannelModelStats(stats, mapping, served)

	require.Len(t, folded, 2)
	// Insertion order: first folded occurrence of each upstream name.
	deepseek := folded[0]
	assert.Equal(t, "deepseek-v4-flash:0731", deepseek.ModelName)
	assert.EqualValues(t, 422, deepseek.Requests)
	assert.EqualValues(t, 105, deepseek.PromptTokens)
	assert.EqualValues(t, 216, deepseek.CompletionTokens)
	glm := folded[1]
	assert.Equal(t, "glm-5.3-flash", glm.ModelName)
	assert.EqualValues(t, 131, glm.Requests)
}

func TestFoldChannelModelStatsNilServedKeepsAll(t *testing.T) {
	// A channel with an empty model list must not hide everything.
	stats := []model.ModelTokenStat{
		{ModelName: "anything", Requests: 1, PromptTokens: 1, CompletionTokens: 1},
	}
	folded := foldChannelModelStats(stats, nil, nil)
	require.Len(t, folded, 1)
	assert.Equal(t, "anything", folded[0].ModelName)
}

func TestEstimateOllamaChannelUsage(t *testing.T) {
	stats := []model.ModelTokenStat{
		{ModelName: "gpt-oss:20b", Requests: 2, PromptTokens: 1000, CompletionTokens: 500},
		{ModelName: "glm-5.3-flash", Requests: 3, PromptTokens: 2000, CompletionTokens: 3000},
		{ModelName: "kimi-k3", Requests: 1, PromptTokens: 100, CompletionTokens: 50},
	}

	now := time.Unix(1788050524, 0)
	window := estimateOllamaChannelUsage(stats, ollamaSessionWindowSeconds, now)

	assert.Equal(t, ollamaSessionWindowSeconds, window.WindowSeconds)
	assert.Equal(t, now.Unix()-ollamaSessionWindowSeconds, window.Since)
	require.Len(t, window.Models, 3)
	assert.Equal(t, 1, window.Models[0].Level)
	assert.Equal(t, 2, window.Models[1].Level)
	assert.Equal(t, 0, window.Models[2].Level)
	// Total tokens count every model.
	assert.EqualValues(t, 1000+500+2000+3000+100+50, window.TotalTokens)
	// Weighted usage only counts models with a known level: 1*1500 + 2*5000.
	assert.Equal(t, float64(11500), window.WeightedUsage)
}

func TestEstimateOllamaChannelUsageEmpty(t *testing.T) {
	window := estimateOllamaChannelUsage(nil, ollamaWeeklyWindowSeconds, time.Unix(1788050524, 0))
	assert.Empty(t, window.Models)
	assert.NotNil(t, window.Models)
	assert.EqualValues(t, 0, window.TotalTokens)
	assert.Equal(t, float64(0), window.WeightedUsage)
}
