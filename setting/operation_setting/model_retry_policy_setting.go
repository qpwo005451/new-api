package operation_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

// ModelRetryPolicySetting controls models that should visit each channel
// priority exactly once. Other models keep the gateway-wide retry behavior.
type ModelRetryPolicySetting struct {
	SinglePassPriorityModels []string `json:"single_pass_priority_models"`
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

func GetModelRetryPolicySetting() *ModelRetryPolicySetting {
	return &modelRetryPolicySetting
}
