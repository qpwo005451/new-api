package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
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
}

func ollamaModelUsageLevel(modelName string) int {
	if level, ok := ollamaModelUsageLevels[modelName]; ok {
		return level
	}
	return 0
}

func estimateOllamaChannelUsage(stats []model.ModelTokenStat, windowSeconds int64, now time.Time) ollamaLocalUsageWindow {
	window := ollamaLocalUsageWindow{
		WindowSeconds: windowSeconds,
		Since:         now.Add(-time.Duration(windowSeconds) * time.Second).Unix(),
		Models:        []ollamaLocalModelUsage{},
	}
	for _, stat := range stats {
		tokens := stat.PromptTokens + stat.CompletionTokens
		entry := ollamaLocalModelUsage{
			ModelName:        stat.ModelName,
			Requests:         stat.Requests,
			PromptTokens:     stat.PromptTokens,
			CompletionTokens: stat.CompletionTokens,
			Level:            ollamaModelUsageLevel(stat.ModelName),
		}
		window.TotalTokens += tokens
		if entry.Level > 0 {
			window.WeightedUsage += float64(entry.Level) * float64(tokens)
		}
		window.Models = append(window.Models, entry)
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

	sessionStats, err := model.SumModelTokensByChannel(id, now.Add(-time.Duration(ollamaSessionWindowSeconds)*time.Second).Unix())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	weeklyStats, err := model.SumModelTokensByChannel(id, now.Add(-time.Duration(ollamaWeeklyWindowSeconds)*time.Second).Unix())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"channel_id": id,
			"fetched_at": now.Unix(),
			"upstream": gin.H{
				"session":  upstream.Limits.Session,
				"weekly":   upstream.Limits.Weekly,
				"activity": upstream.Activity,
			},
			"local": gin.H{
				"session": estimateOllamaChannelUsage(sessionStats, ollamaSessionWindowSeconds, now),
				"weekly":  estimateOllamaChannelUsage(weeklyStats, ollamaWeeklyWindowSeconds, now),
			},
		},
	})
}
