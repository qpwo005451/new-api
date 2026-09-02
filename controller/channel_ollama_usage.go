package controller

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// Usage levels shown on the ollama.com model pages for cloud models,
// observed on 2026-08-30 (Low = 1, Medium = 2, High = 3, Extra High = 4).
// The weight of each level in Ollama's usage accounting is not published,
// so levels are used here only as relative weights for the local estimate.
// The catalog changes over time; unknown models fall back to level 0 and
// are excluded from the weighted usage.
var ollamaModelUsageLevels = map[string]int{
	// Low Usage
	"gpt-oss:20b":         1,
	"gemma4":              1,
	"gemma4:31b":          1,
	"nemotron-3-nano:4b":  1,
	"nemotron-3-nano:30b": 1,
	// Medium Usage
	"gpt-oss:120b":           2,
	"glm-5.3-flash":          2,
	"deepseek-v4-flash":      2,
	"deepseek-v4-flash:0731": 2,
	"mistral-large-3":        2,
	"mistral-large-3:675b":   2,
	"nemotron-3-super":       2,
	"nemotron-3-super:120b":  2,
	// Extra High Usage
	"deepseek-v4-pro":      4,
	"deepseek-v4-pro:0813": 4,
}

const (
	ollamaSessionWindowSeconds = int64(5 * 3600)
	ollamaWeeklyWindowSeconds  = int64(7 * 24 * 3600)
)

type ollamaUsageModelEntry struct {
	Name         string `json:"name"`
	RequestCount int64  `json:"request_count"`
}

type ollamaUsageWindow struct {
	Usage  float64                 `json:"usage"`
	Models []ollamaUsageModelEntry `json:"models"`
}

type ollamaUsageResponse struct {
	Activity struct {
		Cost   string          `json:"cost"`
		Period json.RawMessage `json:"period"`
		Models json.RawMessage `json:"models"`
	} `json:"activity"`
	Limits struct {
		Session ollamaUsageWindow `json:"session"`
		Weekly  ollamaUsageWindow `json:"weekly"`
	} `json:"limits"`
}

type ollamaLocalModelUsage struct {
	ModelName        string `json:"model_name"`
	Requests         int64  `json:"requests"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	Level            int    `json:"level"`
}

type ollamaLocalUsageWindow struct {
	WindowSeconds int64                   `json:"window_seconds"`
	Since         int64                   `json:"since"`
	Models        []ollamaLocalModelUsage `json:"models"`
	TotalTokens   int64                   `json:"total_tokens"`
	WeightedUsage float64                 `json:"weighted_usage"`
	// EarliestReleaseAt is the moment the oldest request still counted in
	// the window slides out and usage starts recovering; 0 for empty windows.
	EarliestReleaseAt int64                      `json:"earliest_release_at"`
	Projection        ollamaLocalUsageProjection `json:"projection"`
}

type ollamaUsageProjectionPoint struct {
	AfterSeconds  int64   `json:"after_seconds"`
	WeightedUsage float64 `json:"weighted_usage"`
	Requests      int64   `json:"requests"`
}

type ollamaLocalUsageProjection struct {
	BucketSeconds int64                        `json:"bucket_seconds"`
	Points        []ollamaUsageProjectionPoint `json:"points"`
}

// ollamaSnapshotWindow mirrors one window of the N8N usage snapshot. The
// workflow reads usedPercent and resetsAt from the server-rendered
// ollama.com/settings page, so resetsAt is the authoritative reset instant
// the /api/usage endpoint does not expose.
type ollamaSnapshotWindow struct {
	UsedPercent    float64 `json:"usedPercent"`
	ResetsAt       *string `json:"resetsAt"`
	ResetInMinutes *int64  `json:"resetInMinutes"`
}

type ollamaSnapshotUsage struct {
	FiveHour  *ollamaSnapshotWindow `json:"fiveHour"`
	Weekly    *ollamaSnapshotWindow `json:"weekly"`
	FetchedAt *string               `json:"fetchedAt"`
	Source    string                `json:"source"`
}

// Raw webhook response. The refresh webhook returns {ok, usage:{...}}; the
// read-only query webhook puts the windows at the top level, so both shapes
// are accepted.
type ollamaSnapshotResponse struct {
	Ok           bool                  `json:"ok"`
	FailureClass string               `json:"failureClass"`
	Usage        *ollamaSnapshotUsage `json:"usage"`
	FiveHour     *ollamaSnapshotWindow `json:"fiveHour"`
	Weekly       *ollamaSnapshotWindow `json:"weekly"`
	FetchedAt    *string              `json:"fetchedAt"`
	Source       string               `json:"source"`
}

// Header carrying the webhook secret; the N8N httpHeaderAuth credential must
// use this exact name with the value configured in OllamaUsageWebhookSecret.
const ollamaUsageWebhookHeader = "X-Ollama-Refresh-Secret"

// Short timeout so a slow refresh falls back to the local estimate instead of
// hanging the admin panel; the workflow keeps running server-side either way.
var ollamaUsageWebhookClient = &http.Client{Timeout: 15 * time.Second}

// fetchOllamaUsageSnapshot triggers the N8N refresh webhook and returns the
// fresh snapshot. One call both refreshes the upstream scrape and returns its
// result; any failure (network, non-200, ok:false, missing windows) is
// returned as an error so the caller can fall back to the local estimate.
func fetchOllamaUsageSnapshot(url, secret string, client *http.Client) (*ollamaSnapshotUsage, error) {
	if client == nil {
		client = ollamaUsageWebhookClient
	}
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("构造 Ollama 用量刷新请求失败: %w", err)
	}
	req.Header.Set(ollamaUsageWebhookHeader, secret)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Ollama 用量刷新 Webhook 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama 用量刷新 Webhook 返回状态码: %d", resp.StatusCode)
	}
	var parsed ollamaSnapshotResponse
	if err := common.DecodeJson(resp.Body, &parsed); err != nil {
		return nil, fmt.Errorf("解析 Ollama 用量刷新响应失败: %w", err)
	}
	if !parsed.Ok {
		return nil, fmt.Errorf("Ollama 用量刷新失败: %s", parsed.FailureClass)
	}
	snapshot := parsed.Usage
	if snapshot == nil {
		// Query shape: windows at the top level.
		snapshot = &ollamaSnapshotUsage{
			FiveHour:  parsed.FiveHour,
			Weekly:    parsed.Weekly,
			FetchedAt: parsed.FetchedAt,
			Source:    parsed.Source,
		}
	}
	if snapshot.FiveHour == nil || snapshot.Weekly == nil {
		return nil, fmt.Errorf("Ollama 用量刷新响应缺少窗口数据")
	}
	// The refresh shape carries no resetInMinutes; compute it locally so the
	// panel can render "resets in N minutes" regardless of response shape.
	now := time.Now()
	for _, w := range []*ollamaSnapshotWindow{snapshot.FiveHour, snapshot.Weekly} {
		if w.ResetInMinutes != nil || w.ResetsAt == nil {
			continue
		}
		resetAt, parseErr := time.Parse(time.RFC3339, *w.ResetsAt)
		if parseErr != nil {
			continue
		}
		minutes := int64(math.Ceil(resetAt.Sub(now).Minutes()))
		if minutes < 0 {
			minutes = 0
		}
		w.ResetInMinutes = &minutes
	}
	return snapshot, nil
}

func ollamaModelUsageLevel(modelName string) int {
	if level, ok := ollamaModelUsageLevels[modelName]; ok {
		return level
	}
	return 0
}

// foldChannelModelUsage renames requested-model usage to the channel's
// upstream model names via the channel's model mapping and drops models the
// channel does not serve (leftover log entries of deleted or re-created
// channels sharing the same id). Ollama meters the upstream model name, so
// the folded usage is what its usage windows actually count.
func foldChannelModelUsage(
	rows []model.ModelUsageSecond,
	mapping map[string]string,
	served map[string]bool,
) []model.ModelUsageSecond {
	folded := make([]model.ModelUsageSecond, 0, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(row.ModelName)
		if mapped, ok := mapping[name]; ok {
			name = mapped
		}
		if served != nil && !served[name] {
			continue
		}
		row.ModelName = name
		folded = append(folded, row)
	}
	return folded
}

func estimateOllamaChannelUsage(rows []model.ModelUsageSecond, windowSeconds int64, now time.Time) ollamaLocalUsageWindow {
	// Hourly checkpoints for the 5-hour window, daily ones for the weekly
	// window — the two window shapes this endpoint estimates.
	bucketSeconds := int64(24 * 3600)
	if windowSeconds <= 24*3600 {
		bucketSeconds = 3600
	}
	since := now.Add(-time.Duration(windowSeconds) * time.Second).Unix()
	window := ollamaLocalUsageWindow{
		WindowSeconds: windowSeconds,
		Since:         since,
		Models:        []ollamaLocalModelUsage{},
		Projection: ollamaLocalUsageProjection{
			BucketSeconds: bucketSeconds,
			Points:        make([]ollamaUsageProjectionPoint, 0, int(windowSeconds/bucketSeconds)),
		},
	}

	modelIndex := make(map[string]int, len(rows))
	type usageSample struct {
		createdAt int64
		weighted  float64
		requests  int64
	}
	samples := make([]usageSample, 0, len(rows))
	for _, row := range rows {
		level := ollamaModelUsageLevel(row.ModelName)
		tokens := row.PromptTokens + row.CompletionTokens
		weighted := float64(level) * float64(tokens)
		if idx, ok := modelIndex[row.ModelName]; ok {
			entry := &window.Models[idx]
			entry.Requests += row.Requests
			entry.PromptTokens += row.PromptTokens
			entry.CompletionTokens += row.CompletionTokens
		} else {
			modelIndex[row.ModelName] = len(window.Models)
			window.Models = append(window.Models, ollamaLocalModelUsage{
				ModelName:        row.ModelName,
				Requests:         row.Requests,
				PromptTokens:     row.PromptTokens,
				CompletionTokens: row.CompletionTokens,
				Level:            level,
			})
		}
		window.TotalTokens += tokens
		window.WeightedUsage += weighted
		samples = append(samples, usageSample{createdAt: row.CreatedAt, weighted: weighted, requests: row.Requests})
	}
	// Most-requested models first so the panel keeps a stable order.
	sort.Slice(window.Models, func(i, j int) bool {
		if window.Models[i].Requests != window.Models[j].Requests {
			return window.Models[i].Requests > window.Models[j].Requests
		}
		return window.Models[i].ModelName < window.Models[j].ModelName
	})

	if len(samples) == 0 {
		return window
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].createdAt < samples[j].createdAt })
	window.EarliestReleaseAt = samples[0].createdAt + windowSeconds

	// Suffix sums let each checkpoint report the weighted usage and request
	// count still inside the window at now + AfterSeconds.
	suffixWeighted := make([]float64, len(samples)+1)
	suffixRequests := make([]int64, len(samples)+1)
	for i := len(samples) - 1; i >= 0; i-- {
		suffixWeighted[i] = suffixWeighted[i+1] + samples[i].weighted
		suffixRequests[i] = suffixRequests[i+1] + samples[i].requests
	}
	for b := int64(0); b < windowSeconds/bucketSeconds; b++ {
		cutoff := since + (b+1)*bucketSeconds
		idx := sort.Search(len(samples), func(i int) bool { return samples[i].createdAt >= cutoff })
		window.Projection.Points = append(window.Projection.Points, ollamaUsageProjectionPoint{
			AfterSeconds:  (b + 1) * bucketSeconds,
			WeightedUsage: suffixWeighted[idx],
			Requests:      suffixRequests[idx],
		})
	}
	return window
}

func GetOllamaChannelUsage(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	channel, err := model.GetChannelById(id, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if channel.Type != constant.ChannelTypeOllama {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "仅支持 Ollama 渠道",
		})
		return
	}

	now := time.Now()
	upstreamURL := strings.TrimSuffix(channel.GetBaseURL(), "/") + "/api/usage"
	body, err := GetResponseBody(http.MethodGet, upstreamURL, channel, GetAuthHeader(channel.Key))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("获取 Ollama 用量失败: %s", err.Error()),
		})
		return
	}
	var upstream ollamaUsageResponse
	if err := common.Unmarshal(body, &upstream); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("解析 Ollama 用量失败: %s", err.Error()),
		})
		return
	}

	weeklyRows, err := model.SumModelUsageByChannelSecond(id, now.Add(-time.Duration(ollamaWeeklyWindowSeconds)*time.Second).Unix())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	modelMapping := normalizeChannelModelMapping(channel)
	var served map[string]bool
	channelModels := channel.GetModels()
	if len(channelModels) > 0 {
		served = make(map[string]bool, len(channelModels))
		for _, m := range channelModels {
			served[strings.TrimSpace(m)] = true
		}
	}
	weeklyRows = foldChannelModelUsage(weeklyRows, modelMapping, served)
	sessionSince := now.Add(-time.Duration(ollamaSessionWindowSeconds) * time.Second).Unix()
	sessionRows := make([]model.ModelUsageSecond, 0, len(weeklyRows))
	for _, row := range weeklyRows {
		if row.CreatedAt >= sessionSince {
			sessionRows = append(sessionRows, row)
		}
	}

	// Authoritative snapshot from the N8N refresh webhook, configured via
	// OllamaUsageWebhookUrl/Secret. One call triggers an immediate upstream
	// scrape and returns its result; on any failure the panel falls back to
	// the local estimates below without affecting the main flow.
	var snapshot *ollamaSnapshotUsage
	if webhookURL := common.OptionMap["OllamaUsageWebhookUrl"]; webhookURL != "" {
		secret := common.OptionMap["OllamaUsageWebhookSecret"]
		fetched, err := fetchOllamaUsageSnapshot(webhookURL, secret, ollamaUsageWebhookClient)
		if err != nil {
			common.SysError("获取 Ollama 用量快照失败: " + err.Error())
		} else {
			snapshot = fetched
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"channel_id": id,
			"fetched_at": now.Unix(),
			"snapshot":   snapshot,
			"upstream": gin.H{
				"session":  upstream.Limits.Session,
				"weekly":   upstream.Limits.Weekly,
				"activity": upstream.Activity,
			},
			"local": gin.H{
				"session": estimateOllamaChannelUsage(sessionRows, ollamaSessionWindowSeconds, now),
				"weekly":  estimateOllamaChannelUsage(weeklyRows, ollamaWeeklyWindowSeconds, now),
			},
		},
	})
}
