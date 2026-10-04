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
	MaxModelWeightEntries          = 10000
	MaxModelWeightValue            = 1000000
	MaxModelPriorityValue          = 1000000000
	MaxModelWeightPresets          = 50
	MaxModelWeightPresetNameLength = 64
)

// ModelWeightOverride overrides the channel-level selection attributes for one
// model on one channel. It is scoped to (channel, model); every other model on
// the same channel keeps using the channel-level weight and priority.
//
// Both fields are optional so an entry can override only the priority, only the
// weight, or both. An absent field falls back to the channel value, while an
// explicit 0 is a real value and not a fallback.
type ModelWeightOverride struct {
	ChannelID int    `json:"channel_id"`
	Model     string `json:"model"`
	Weight    *uint  `json:"weight,omitempty"`
	Priority  *int64 `json:"priority,omitempty"`
}

// ModelWeightPreset is a named, ready-to-apply weight configuration. Applying a
// preset copies its weights into ModelWeightSetting.Weights; the preset itself
// stays untouched so it can be re-applied later.
type ModelWeightPreset struct {
	Name    string                `json:"name"`
	Weights []ModelWeightOverride `json:"weights"`
}

// ModelWeightSetting keeps the overrides as a raw JSON array string so the
// config registry persists it like the other JSON-array options.
type ModelWeightSetting struct {
	Weights string `json:"weights"` // JSON array of ModelWeightOverride
	Presets string `json:"presets"` // JSON array of ModelWeightPreset
}

var modelWeightSetting = ModelWeightSetting{Weights: "[]", Presets: "[]"}

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
	index map[string]ModelWeightOverride
}{}

func modelWeightIndex() map[string]ModelWeightOverride {
	raw := modelWeightSetting.Weights
	modelWeightIndexCache.RLock()
	if modelWeightIndexCache.index != nil && modelWeightIndexCache.raw == raw {
		index := modelWeightIndexCache.index
		modelWeightIndexCache.RUnlock()
		return index
	}
	modelWeightIndexCache.RUnlock()

	index := make(map[string]ModelWeightOverride)
	if strings.TrimSpace(raw) != "" {
		var entries []ModelWeightOverride
		if err := common.UnmarshalJsonStr(raw, &entries); err == nil {
			for _, entry := range entries {
				if entry.ChannelID <= 0 || strings.TrimSpace(entry.Model) == "" {
					continue
				}
				index[modelWeightKey(entry.ChannelID, entry.Model)] = entry
			}
		}
	}

	modelWeightIndexCache.Lock()
	modelWeightIndexCache.raw = raw
	modelWeightIndexCache.index = index
	modelWeightIndexCache.Unlock()
	return index
}

// EffectiveModelWeight returns the configured per-model weight override for
// (channel, model), or channelWeight when no override exists. A configured
// weight of 0 is an explicit zero, not a fallback.
func EffectiveModelWeight(channelID int, modelName string, channelWeight int) int {
	index := modelWeightIndex()
	if len(index) == 0 {
		return channelWeight
	}
	if entry, ok := index[modelWeightKey(channelID, modelName)]; ok && entry.Weight != nil {
		return int(*entry.Weight)
	}
	return channelWeight
}

// EffectiveModelPriority returns the configured per-model priority override for
// (channel, model), or channelPriority when no override exists. Selection groups
// candidates into priority tiers before applying weights, so overriding the
// priority for a single model is what lets that model spread across channels of
// different channel-level priorities without changing the channel-level
// priority that every other model on those channels still uses.
//
// A configured priority of 0 is an explicit zero, not a fallback.
func EffectiveModelPriority(channelID int, modelName string, channelPriority int64) int64 {
	index := modelWeightIndex()
	if len(index) == 0 {
		return channelPriority
	}
	if entry, ok := index[modelWeightKey(channelID, modelName)]; ok && entry.Priority != nil {
		return *entry.Priority
	}
	return channelPriority
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
	return validateModelWeightEntries(entries, "model weights")
}

// ValidateModelWeightPresets enforces the structural invariants of the preset
// array. Each preset reuses the same per-entry rules as the live weights, and
// names are unique case-insensitively so the UI can address a preset by name.
func ValidateModelWeightPresets(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var presets []ModelWeightPreset
	if err := common.UnmarshalJsonStr(value, &presets); err != nil {
		return fmt.Errorf("model weight presets must be a JSON array: %w", err)
	}
	if presets == nil {
		return fmt.Errorf("model weight presets must be a JSON array")
	}
	if len(presets) > MaxModelWeightPresets {
		return fmt.Errorf("too many model weight presets (max %d)", MaxModelWeightPresets)
	}
	seen := make(map[string]struct{}, len(presets))
	for index, preset := range presets {
		name := strings.TrimSpace(preset.Name)
		if name == "" {
			return fmt.Errorf("model weight preset %d requires a name", index+1)
		}
		if len(name) > MaxModelWeightPresetNameLength {
			return fmt.Errorf("model weight preset %d name is longer than %d bytes", index+1, MaxModelWeightPresetNameLength)
		}
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate model weight preset name %q", name)
		}
		seen[key] = struct{}{}
		if err := validateModelWeightEntries(preset.Weights, fmt.Sprintf("model weight preset %q", name)); err != nil {
			return err
		}
	}
	return nil
}

func validateModelWeightEntries(entries []ModelWeightOverride, label string) error {
	if entries == nil {
		return fmt.Errorf("%s must be a JSON array", label)
	}
	if len(entries) > MaxModelWeightEntries {
		return fmt.Errorf("%s has too many entries (max %d)", label, MaxModelWeightEntries)
	}
	seen := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		if entry.ChannelID <= 0 {
			return fmt.Errorf("%s entry %d has an invalid channel_id %d", label, index+1, entry.ChannelID)
		}
		model := strings.TrimSpace(entry.Model)
		if model == "" {
			return fmt.Errorf("%s entry %d requires a model", label, index+1)
		}
		if len(model) > 255 {
			return fmt.Errorf("%s entry %d has a model name longer than 255 bytes", label, index+1)
		}
		if entry.Weight != nil && *entry.Weight > MaxModelWeightValue {
			return fmt.Errorf("%s entry %d exceeds the maximum weight %d", label, index+1, MaxModelWeightValue)
		}
		if entry.Priority != nil {
			if *entry.Priority < 0 {
				return fmt.Errorf("%s entry %d has a negative priority %d", label, index+1, *entry.Priority)
			}
			if *entry.Priority > MaxModelPriorityValue {
				return fmt.Errorf("%s entry %d exceeds the maximum priority %d", label, index+1, MaxModelPriorityValue)
			}
		}
		if entry.Weight == nil && entry.Priority == nil {
			return fmt.Errorf("%s entry %d for channel %d model %q must set a weight or a priority", label, index+1, entry.ChannelID, model)
		}
		key := modelWeightKey(entry.ChannelID, model)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%s has duplicate entry for channel %d model %q", label, entry.ChannelID, model)
		}
		seen[key] = struct{}{}
	}
	return nil
}
