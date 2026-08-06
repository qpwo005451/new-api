package operation_setting

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

type VirtualModelRouteTarget struct {
	Model              string            `json:"model"`
	ReasoningEffortMap map[string]string `json:"reasoning_effort_map,omitempty"`
}

// ModelRetryPolicySetting controls models that should visit each channel
// priority exactly once. Other models keep the gateway-wide retry behavior.
type ModelRetryPolicySetting struct {
	SinglePassPriorityModels []string                             `json:"single_pass_priority_models"`
	VirtualModelRoutes       map[string][]VirtualModelRouteTarget `json:"virtual_model_routes"`
}

var modelRetryPolicySetting = ModelRetryPolicySetting{
	SinglePassPriorityModels: []string{"auto_image_reader"},
}

func init() {
	config.GlobalConfig.Register("model_retry_policy_setting", &modelRetryPolicySetting)
}

func UseSinglePassPriorityFallback(modelName string) bool {
	for _, configuredModel := range modelRetryPolicySetting.SinglePassPriorityModels {
		if strings.EqualFold(strings.TrimSpace(configuredModel), strings.TrimSpace(modelName)) {
			return true
		}
	}
	return false
}

func GetVirtualModelRoute(modelName string) []VirtualModelRouteTarget {
	for configuredModel, route := range modelRetryPolicySetting.VirtualModelRoutes {
		if strings.EqualFold(strings.TrimSpace(configuredModel), strings.TrimSpace(modelName)) {
			copied := make([]VirtualModelRouteTarget, 0, len(route))
			for _, target := range route {
				effortMap := make(map[string]string, len(target.ReasoningEffortMap))
				for effort, mappedEffort := range target.ReasoningEffortMap {
					effortMap[effort] = mappedEffort
				}
				copied = append(copied, VirtualModelRouteTarget{
					Model:              target.Model,
					ReasoningEffortMap: effortMap,
				})
			}
			return copied
		}
	}
	return nil
}

func MapVirtualModelReasoningEffort(target VirtualModelRouteTarget, effort string) string {
	normalizedEffort := strings.ToLower(strings.TrimSpace(effort))
	if normalizedEffort == "" || len(target.ReasoningEffortMap) == 0 {
		return normalizedEffort
	}
	for configuredEffort, mappedEffort := range target.ReasoningEffortMap {
		if strings.EqualFold(strings.TrimSpace(configuredEffort), normalizedEffort) {
			return strings.ToLower(strings.TrimSpace(mappedEffort))
		}
	}
	return normalizedEffort
}

func ValidateVirtualModelRoutes(value string) error {
	var routes map[string][]VirtualModelRouteTarget
	if err := common.UnmarshalJsonStr(value, &routes); err != nil {
		return fmt.Errorf("invalid virtual model routes JSON: %w", err)
	}
	for virtualModel, targets := range routes {
		if strings.TrimSpace(virtualModel) == "" {
			return fmt.Errorf("virtual model name cannot be empty")
		}
		if len(targets) == 0 {
			return fmt.Errorf("virtual model %q must contain at least one route target", virtualModel)
		}
		for index, target := range targets {
			if strings.TrimSpace(target.Model) == "" {
				return fmt.Errorf("virtual model %q route target %d has an empty model", virtualModel, index)
			}
			for effort, mappedEffort := range target.ReasoningEffortMap {
				if strings.TrimSpace(effort) == "" || strings.TrimSpace(mappedEffort) == "" {
					return fmt.Errorf("virtual model %q route target %d has an empty reasoning effort mapping", virtualModel, index)
				}
			}
		}
	}
	return nil
}

func GetModelRetryPolicySetting() *ModelRetryPolicySetting {
	return &modelRetryPolicySetting
}
