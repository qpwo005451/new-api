package operation_setting

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	MaxModelWeightEntries = 10000
	MaxModelWeightValue   = 1000000
)

// ModelWeightOverride replaces the channel-level weight for one model on one
// channel. It is scoped to (channel, model); every other model keeps using the
// channel weight.
type ModelWeightOverride struct {
	ChannelID int    `json:"channel_id"`
	Model     string `json:"model"`
	Weight    uint   `json:"weight"`
}

// ModelWeightSetting keeps the overrides as a raw JSON array string so the
// config registry persists it like the other JSON-array options.
type ModelWeightSetting struct {
	Weights string `json:"weights"` // JSON array of ModelWeightOverride
}

var modelWeightSetting = ModelWeightSetting{Weights: "[]"}

func init() {
	config.GlobalConfig.Register("model_weight_setting", &modelWeightSetting)
}

func GetModelWeightSetting() *ModelWeightSetting {
	return &modelWeightSetting
}

func modelWeightKey(channelID int, modelName string) string {
	return strconv.Itoa(channelID) + "|" + strings.ToLower(strings.TrimSpace(modelName))
}

// modelWeightIndexCache caches the parsed overrides keyed by the raw option
// string, so selection never parses the JSON twice for the same value.
var modelWeightIndexCache = struct {
	sync.RWMutex
	raw   string
	index map[string]uint
}{}

func modelWeightIndex() map[string]uint {
	raw := modelWeightSetting.Weights
	modelWeightIndexCache.RLock()
	if modelWeightIndexCache.index != nil && modelWeightIndexCache.raw == raw {
		index := modelWeightIndexCache.index
		modelWeightIndexCache.RUnlock()
		return index
	}
	modelWeightIndexCache.RUnlock()

	index := make(map[string]uint)
	if strings.TrimSpace(raw) != "" {
		var entries []ModelWeightOverride
		if err := common.UnmarshalJsonStr(raw, &entries); err == nil {
			for _, entry := range entries {
				if entry.ChannelID <= 0 || strings.TrimSpace(entry.Model) == "" {
					continue
				}
				index[modelWeightKey(entry.ChannelID, entry.Model)] = entry.Weight
			}
		}
	}

	modelWeightIndexCache.Lock()
	modelWeightIndexCache.raw = raw
	modelWeightIndexCache.index = index
	modelWeightIndexCache.Unlock()
	return index
}

// EffectiveModelWeight returns the configured per-model override for
// (channel, model), or channelWeight when no override exists. A configured
// weight of 0 is an explicit zero, not a fallback.
func EffectiveModelWeight(channelID int, modelName string, channelWeight int) int {
	index := modelWeightIndex()
	if len(index) == 0 {
		return channelWeight
	}
	if weight, ok := index[modelWeightKey(channelID, modelName)]; ok {
		return int(weight)
	}
	return channelWeight
}

// ListModelWeightOverrides returns the parsed overrides for one channel.
func ListModelWeightOverrides(channelID int) []ModelWeightOverride {
	overrides := []ModelWeightOverride{}
	var entries []ModelWeightOverride
	if err := common.UnmarshalJsonStr(modelWeightSetting.Weights, &entries); err != nil {
		return overrides
	}
	for _, entry := range entries {
		if entry.ChannelID == channelID {
			overrides = append(overrides, entry)
		}
	}
	return overrides
}

// ValidateModelWeights enforces the structural invariants of the override
// array. It does not check that the channel or model exists: a stale entry is
// ignored by EffectiveModelWeight and never blocks routing.
func ValidateModelWeights(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var entries []ModelWeightOverride
	if err := common.UnmarshalJsonStr(value, &entries); err != nil {
		return fmt.Errorf("model weights must be a JSON array: %w", err)
	}
	if entries == nil {
		return fmt.Errorf("model weights must be a JSON array")
	}
	if len(entries) > MaxModelWeightEntries {
		return fmt.Errorf("too many model weight entries (max %d)", MaxModelWeightEntries)
	}
	seen := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		if entry.ChannelID <= 0 {
			return fmt.Errorf("model weight entry %d has an invalid channel_id %d", index+1, entry.ChannelID)
		}
		model := strings.TrimSpace(entry.Model)
		if model == "" {
			return fmt.Errorf("model weight entry %d requires a model", index+1)
		}
		if len(model) > 255 {
			return fmt.Errorf("model weight entry %d has a model name longer than 255 bytes", index+1)
		}
		if entry.Weight > MaxModelWeightValue {
			return fmt.Errorf("model weight entry %d exceeds the maximum weight %d", index+1, MaxModelWeightValue)
		}
		key := modelWeightKey(entry.ChannelID, model)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate model weight for channel %d model %q", entry.ChannelID, model)
		}
		seen[key] = struct{}{}
	}
	return nil
}
