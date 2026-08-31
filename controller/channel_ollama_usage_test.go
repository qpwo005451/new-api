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

func TestFoldChannelModelUsage(t *testing.T) {
	// Mirrors the production ollama_pro channel: "deepseek-v4-flash" is
	// mapped to the upstream "deepseek-v4-flash:0731", so both log names
	// meter the same upstream model. Logs of a deleted channel reusing the
	// same id (gpt-5.6-luna) must be dropped because the current channel
	// does not serve that model.
	mapping := map[string]string{"deepseek-v4-flash": "deepseek-v4-flash:0731"}
	served := map[string]bool{
		"glm-5.3-flash":          true,
		"deepseek-v4-flash:0731": true,
		"deepseek-v4-flash":      true,
		"gpt-oss:20b":            true,
	}
	rows := []model.ModelUsageSecond{
		{CreatedAt: 1788050000, ModelName: " deepseek-v4-flash ", Requests: 420, PromptTokens: 100, CompletionTokens: 200},
		{CreatedAt: 1788050100, ModelName: "deepseek-v4-flash:0731", Requests: 2, PromptTokens: 5, CompletionTokens: 16},
		{CreatedAt: 1788050200, ModelName: "glm-5.3-flash", Requests: 131, PromptTokens: 300, CompletionTokens: 400},
		{CreatedAt: 1788050300, ModelName: "gpt-5.6-luna", Requests: 2, PromptTokens: 2, CompletionTokens: 2},
	}

	folded := foldChannelModelUsage(rows, mapping, served)

	require.Len(t, folded, 3)
	// Insertion order preserved; names rewritten through the mapping and
	// trimmed, timestamps kept for the slide-out projection.
	assert.Equal(t, "deepseek-v4-flash:0731", folded[0].ModelName)
	assert.EqualValues(t, 1788050000, folded[0].CreatedAt)
	assert.EqualValues(t, 420, folded[0].Requests)
	assert.Equal(t, "deepseek-v4-flash:0731", folded[1].ModelName)
	assert.Equal(t, "glm-5.3-flash", folded[2].ModelName)
}

func TestFoldChannelModelUsageNilServedKeepsAll(t *testing.T) {
	// A channel with an empty model list must not hide everything.
	rows := []model.ModelUsageSecond{
		{ModelName: "anything", Requests: 1, PromptTokens: 1, CompletionTokens: 1},
	}
	folded := foldChannelModelUsage(rows, nil, nil)
	require.Len(t, folded, 1)
	assert.Equal(t, "anything", folded[0].ModelName)
}

func TestEstimateOllamaChannelUsage(t *testing.T) {
	// Two deepseek-v4-flash rows at different seconds under the mapped
	// upstream name must aggregate into one model entry (the duplicate
	// counting regression), plus one unknown-level model.
	rows := []model.ModelUsageSecond{
		{CreatedAt: 1788047000, ModelName: "deepseek-v4-flash:0731", Requests: 2, PromptTokens: 1000, CompletionTokens: 500},
		{CreatedAt: 1788049000, ModelName: "glm-5.3-flash", Requests: 3, PromptTokens: 2000, CompletionTokens: 3000},
		{CreatedAt: 1788050400, ModelName: "kimi-k3", Requests: 1, PromptTokens: 100, CompletionTokens: 50},
	}

	now := time.Unix(1788050524, 0)
	window := estimateOllamaChannelUsage(rows, ollamaSessionWindowSeconds, now)

	assert.Equal(t, ollamaSessionWindowSeconds, window.WindowSeconds)
	assert.Equal(t, now.Unix()-ollamaSessionWindowSeconds, window.Since)
	// Most-requested model first, ties broken by name.
	require.Len(t, window.Models, 3)
	assert.Equal(t, "glm-5.3-flash", window.Models[0].ModelName)
	assert.Equal(t, 2, window.Models[0].Level)
	assert.Equal(t, "deepseek-v4-flash:0731", window.Models[1].ModelName)
	assert.Equal(t, "kimi-k3", window.Models[2].ModelName)
	// Total tokens count every model.
	assert.EqualValues(t, 1500+5000+150, window.TotalTokens)
	// Weighted usage only counts models with a known level: 2*1500 + 2*5000.
	assert.Equal(t, float64(13000), window.WeightedUsage)
	// The oldest request leaves the 5-hour window at its creation + 5h.
	assert.Equal(t, int64(1788047000+ollamaSessionWindowSeconds), window.EarliestReleaseAt)
}

func TestEstimateOllamaChannelUsageProjection(t *testing.T) {
	// now = 1788050524, 5-hour window => since = 1788032524, hourly buckets.
	// Row A: weighted 2*1500=3000, created since+1800 (slides out within the
	// first hour). Row B: weighted 2*5000=10000, created since+10800 (slides
	// out during the third hour, gone at the checkpoint T=14400).
	rows := []model.ModelUsageSecond{
		{CreatedAt: 1788034324, ModelName: "glm-5.3-flash", Requests: 2, PromptTokens: 1000, CompletionTokens: 500},
		{CreatedAt: 1788043324, ModelName: "glm-5.3-flash", Requests: 3, PromptTokens: 2000, CompletionTokens: 3000},
	}

	now := time.Unix(1788050524, 0)
	window := estimateOllamaChannelUsage(rows, ollamaSessionWindowSeconds, now)

	since := now.Unix() - ollamaSessionWindowSeconds
	assert.Equal(t, since, window.Since)
	assert.Equal(t, int64(3600), window.Projection.BucketSeconds)
	require.Len(t, window.Projection.Points, 5)
	// Checkpoint at one hour after now: row A has already slid out.
	assert.Equal(t, int64(3600), window.Projection.Points[0].AfterSeconds)
	assert.Equal(t, float64(10000), window.Projection.Points[0].WeightedUsage)
	assert.EqualValues(t, 3, window.Projection.Points[0].Requests)
	// Buckets 2 and 3: row B (created since+10800) is still inside.
	assert.Equal(t, float64(10000), window.Projection.Points[1].WeightedUsage)
	assert.Equal(t, float64(10000), window.Projection.Points[2].WeightedUsage)
	assert.EqualValues(t, 3, window.Projection.Points[2].Requests)
	// Checkpoint at four hours: row B (out at T>10800) slid out too.
	assert.Equal(t, float64(0), window.Projection.Points[3].WeightedUsage)
	assert.EqualValues(t, 0, window.Projection.Points[3].Requests)
	assert.Equal(t, float64(0), window.Projection.Points[4].WeightedUsage)
	// Requests are also projected to zero at the end of the window.
	assert.EqualValues(t, 0, window.Projection.Points[4].Requests)
}

func TestEstimateOllamaChannelUsageEmpty(t *testing.T) {
	window := estimateOllamaChannelUsage(nil, ollamaWeeklyWindowSeconds, time.Unix(1788050524, 0))
	assert.Empty(t, window.Models)
	assert.NotNil(t, window.Models)
	assert.EqualValues(t, 0, window.TotalTokens)
	assert.Equal(t, float64(0), window.WeightedUsage)
	// No traffic means no meaningful recovery timestamps.
	assert.Equal(t, int64(0), window.EarliestReleaseAt)
	assert.Empty(t, window.Projection.Points)
	assert.Equal(t, int64(86400), window.Projection.BucketSeconds)
}
