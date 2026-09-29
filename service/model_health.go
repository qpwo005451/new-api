package service

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const modelHealthRedisPrefix = "new-api:model-health:v1:"

var modelHealthMemoryStore = NewVirtualRouteMemoryHealthStore()

type ModelHealthView struct {
	ModelName       string `json:"model_name"`
	ChannelID       int    `json:"channel_id"`
	Cooling         bool   `json:"cooling"`
	ConsecutiveErrs int    `json:"consecutive_errors"`
	CooldownUntil   int64  `json:"cooldown_until,omitempty"`
}

func modelHealthKey(modelName string, channelID int) string {
	return strings.ToLower(strings.TrimSpace(modelName)) + "|" + strconv.Itoa(channelID)
}

func modelHealthStore() (VirtualRouteHealthStore, error) {
	setting := operation_setting.GetModelRetryPolicySetting().VirtualPoolSticky.Normalize()
	if setting.BindingMode == operation_setting.VirtualPoolBindingModeRedis {
		if !common.RedisEnabled || common.RDB == nil {
			if setting.RedisRequiredForReady {
				return nil, ErrVirtualPoolStoreUnavailable
			}
			common.SysError("model health Redis is unavailable; falling back to process-local health")
			return modelHealthMemoryStore, nil
		}
		return NewVirtualPoolRedisHealthStoreWithPrefix(common.RDB, modelHealthRedisPrefix), nil
	}
	return modelHealthMemoryStore, nil
}

func modelHealthPolicy(modelName string, group string) (operation_setting.ModelHealthPolicyRule, bool) {
	return operation_setting.MatchModelHealthPolicy(modelName, group)
}

// ModelHealthGroup resolves the group that actually served the attempt. For
// auto-group retries the current attempt group is stored in the context, while
// relayInfo.UsingGroup can still hold the value from the previous attempt.
func ModelHealthGroup(c interface{ Get(string) (any, bool) }, fallback string) string {
	if c != nil {
		if value, ok := c.Get(string(constant.ContextKeyAutoGroup)); ok {
			if group, ok := value.(string); ok && strings.TrimSpace(group) != "" {
				return group
			}
		}
	}
	return fallback
}

// IsModelHealthChannelCooling reports whether the model-health policy matched
// and the (model, channel) entry is currently cooling down. Store errors fail
// open so a health-store outage never blocks routing by itself.
func IsModelHealthChannelCooling(modelName string, group string, channelID int) bool {
	if _, ok := modelHealthPolicy(modelName, group); !ok {
		return false
	}
	store, err := modelHealthStore()
	if err != nil {
		common.SysError("failed to open model health store: " + err.Error())
		return false
	}
	cooling, err := store.IsCoolingDown(context.Background(), modelHealthKey(modelName, channelID), time.Now())
	if err != nil {
		common.SysError("failed to read model health cooldown: " + err.Error())
		return false
	}
	return cooling
}

// FilterModelHealthCandidates removes channels that are cooling down for a
// normal (non-virtual) model. It is intentionally per-model: other models on
// the same channel remain selectable.
func FilterModelHealthCandidates(candidates []*model.Channel, modelName string, group string) ([]*model.Channel, error) {
	_, ok := modelHealthPolicy(modelName, group)
	if !ok || len(candidates) == 0 {
		return candidates, nil
	}
	store, err := modelHealthStore()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	filtered := make([]*model.Channel, 0, len(candidates))
	for _, channel := range candidates {
		if channel == nil {
			continue
		}
		cooling, err := store.IsCoolingDown(context.Background(), modelHealthKey(modelName, channel.Id), now)
		if err != nil {
			return nil, err
		}
		if cooling {
			continue
		}
		filtered = append(filtered, channel)
	}
	return filtered, nil
}

// RecordModelHealthFailure extends the existing per-entry cooldown semantics to
// ordinary model names selected outside a virtual route.
func RecordModelHealthFailure(modelName string, group string, channelID int, statusCode int) {
	policy, ok := modelHealthPolicy(modelName, group)
	if !ok || !policy.AllowsStatus(statusCode) {
		return
	}
	store, err := modelHealthStore()
	if err != nil {
		common.SysError("failed to open model health store: " + err.Error())
		return
	}
	health := operation_setting.VirtualModelRouteHealth{
		Enabled:            true,
		DisableModel:       true,
		FailureThreshold:   policy.FailureThreshold,
		CooldownSeconds:    policy.CooldownSeconds,
		MaxCooldownSeconds: policy.CooldownSeconds,
	}.Normalize()
	if err := store.RecordFailure(context.Background(), modelHealthKey(modelName, channelID), health, time.Now()); err != nil {
		common.SysError("failed to record model health failure: " + err.Error())
	}
}

func RecordModelHealthSuccess(modelName string, group string, channelID int) {
	if _, ok := modelHealthPolicy(modelName, group); !ok {
		return
	}
	store, err := modelHealthStore()
	if err != nil {
		common.SysError("failed to open model health store: " + err.Error())
		return
	}
	if err := store.RecordSuccess(context.Background(), modelHealthKey(modelName, channelID)); err != nil {
		common.SysError("failed to clear model health state: " + err.Error())
	}
}

func (store *VirtualRouteMemoryHealthStore) Snapshot(modelName string, channelID int) (ModelHealthView, bool) {
	if store == nil {
		return ModelHealthView{}, false
	}
	store.mutex.Lock()
	entry := store.entries[modelHealthKey(modelName, channelID)]
	store.mutex.Unlock()
	if entry == nil {
		return ModelHealthView{ModelName: modelName, ChannelID: channelID}, true
	}
	entry.mutex.Lock()
	defer entry.mutex.Unlock()
	return ModelHealthView{
		ModelName:       modelName,
		ChannelID:       channelID,
		Cooling:         time.Now().Before(entry.cooldownUntil),
		ConsecutiveErrs: entry.consecutiveErrors,
		CooldownUntil:   entry.cooldownUntil.Unix(),
	}, true
}

// ModelHealthSnapshot exposes process-local state for the settings UI. It is
// visibility only and never drives routing.
func ModelHealthSnapshot(modelName string, channelID int) (ModelHealthView, bool) {
	return modelHealthMemoryStore.Snapshot(modelName, channelID)
}

func ModelHealthKeys() []string {
	modelHealthMemoryStore.mutex.Lock()
	defer modelHealthMemoryStore.mutex.Unlock()
	keys := make([]string, 0, len(modelHealthMemoryStore.entries))
	for key := range modelHealthMemoryStore.entries {
		keys = append(keys, key)
	}
	return keys
}

func IsModelHealthStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusNotFound, http.StatusTooManyRequests:
		return true
	default:
		return statusCode >= http.StatusInternalServerError
	}
}
