package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetVirtualModelRouteUsesCaseInsensitiveExactMatch(t *testing.T) {
	originalRoutes := modelRetryPolicySetting.VirtualModelRoutes
	modelRetryPolicySetting.VirtualModelRoutes = map[string][]VirtualModelRouteTarget{
		"auto-subagent-codex": {
			{Model: "gpt-5.6-luna"},
			{Model: "gpt-5.6-terra"},
		},
	}
	t.Cleanup(func() {
		modelRetryPolicySetting.VirtualModelRoutes = originalRoutes
	})

	route := GetVirtualModelRoute(" AUTO-SUBAGENT-CODEX ")
	assert.Equal(t, []VirtualModelRouteTarget{
		{Model: "gpt-5.6-luna", ReasoningEffortMap: map[string]string{}},
		{Model: "gpt-5.6-terra", ReasoningEffortMap: map[string]string{}},
	}, route)

	route[0].Model = "changed"
	assert.Equal(t, "gpt-5.6-luna", GetVirtualModelRoute("auto-subagent-codex")[0].Model)
	assert.Nil(t, GetVirtualModelRoute("auto-subagent"))
}

func TestMapVirtualModelReasoningEffort(t *testing.T) {
	target := VirtualModelRouteTarget{
		Model: "grok-4.5",
		ReasoningEffortMap: map[string]string{
			"minimal": "low",
			"xhigh":   "high",
			"max":     "high",
		},
	}
	assert.Equal(t, "low", MapVirtualModelReasoningEffort(target, "minimal"))
	assert.Equal(t, "medium", MapVirtualModelReasoningEffort(target, "medium"))
	assert.Equal(t, "high", MapVirtualModelReasoningEffort(target, "MAX"))
}

func TestValidateVirtualModelRoutes(t *testing.T) {
	assert.NoError(t, ValidateVirtualModelRoutes(`{"auto-subagent":[{"model":"gpt-5.6-luna"},{"model":"grok-4.5","reasoning_effort_map":{"minimal":"low","max":"high"}}]}`))
	assert.Error(t, ValidateVirtualModelRoutes(`{"auto-subagent":[]}`))
	assert.Error(t, ValidateVirtualModelRoutes(`{"auto-subagent":[{"model":""}]}`))
}
