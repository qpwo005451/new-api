package operation_setting

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetVirtualModelRouteUsesCaseInsensitiveExactMatch(t *testing.T) {
	originalRoutes := modelRetryPolicySetting.VirtualModelRoutes
	modelRetryPolicySetting.VirtualModelRoutes = map[string]VirtualModelRoute{
		"auto-subagent-codex": {
			Targets: []VirtualModelRouteTarget{
				{Model: "gpt-5.6-luna"},
				{Model: "gpt-5.6-terra"},
			},
		},
	}
	t.Cleanup(func() {
		modelRetryPolicySetting.VirtualModelRoutes = originalRoutes
	})

	route := GetVirtualModelRoute(" AUTO-SUBAGENT-CODEX ")
	assert.Equal(t, []VirtualModelRouteTarget{
		{Model: "gpt-5.6-luna", ReasoningEffortMap: map[string]string{}},
		{Model: "gpt-5.6-terra", ReasoningEffortMap: map[string]string{}},
	}, route.Targets)

	route.Targets[0].Model = "changed"
	assert.Equal(t, "gpt-5.6-luna", GetVirtualModelRoute("auto-subagent-codex").Targets[0].Model)
	assert.Empty(t, GetVirtualModelRoute("auto-subagent").Targets)
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
	assert.NoError(t, ValidateVirtualModelRoutes(`{"auto-free":{"rotation":"round_robin","max_attempts":3,"targets":[{"model":"gpt-5.6-luna"}]}}`))
	assert.Error(t, ValidateVirtualModelRoutes(`{"auto-subagent":[]}`))
	assert.Error(t, ValidateVirtualModelRoutes(`{"auto-subagent":[{"model":""}]}`))
	assert.NoError(t, ValidateVirtualModelRoutes(`{"auto-free":{"rotation":"random","health":{"enabled":true,"failure_threshold":1,"cooldown_seconds":30,"max_cooldown_seconds":300},"targets":[{"model":"gpt-5.6-luna"}]}}`))
	assert.Error(t, ValidateVirtualModelRoutes(`{"auto-free":{"rotation":"shuffle","targets":[{"model":"gpt-5.6-luna"}]}}`))
	assert.Error(t, ValidateVirtualModelRoutes(`{"auto-free":{"max_attempts":-1,"targets":[{"model":"gpt-5.6-luna"}]}}`))
	assert.Error(t, ValidateVirtualModelRoutes(`{"auto-free":{"health":{"enabled":true,"cooldown_seconds":-1},"targets":[{"model":"gpt-5.6-luna"}]}}`))
	assert.NoError(t, ValidateVirtualModelRoutes(`{"auto-free":{"rotation":"round_robin","max_attempts":3,"sources":[{"channel_id":5},{"channel_id":6}]}}`))
	assert.Error(t, ValidateVirtualModelRoutes(`{"auto-free":{"sources":[{"channel_id":0}]}}`))
	assert.Error(t, ValidateVirtualModelRoutes(`{"auto-free":{"sources":[{"channel_id":5}],"rotation":"shuffle"}}`))
}

func TestVirtualModelRouteHasPoolCoversSourcesAndTargets(t *testing.T) {
	assert.False(t, VirtualModelRoute{}.HasPool())
	assert.True(t, VirtualModelRoute{Sources: []VirtualModelRouteSource{{ChannelId: 5}}}.HasPool())
	assert.True(t, VirtualModelRoute{Targets: []VirtualModelRouteTarget{{Model: "gpt-5.6-luna"}}}.HasPool())
}

func TestVirtualModelRouteHealthNormalizeFillsDefaults(t *testing.T) {
	health := VirtualModelRouteHealth{Enabled: true, CooldownSeconds: 90, MaxCooldownSeconds: 10}.Normalize()
	assert.Equal(t, DefaultVirtualModelRouteFailureThreshold, health.FailureThreshold)
	assert.Equal(t, 90, health.CooldownSeconds)
	assert.Equal(t, 90, health.MaxCooldownSeconds, "a maximum below the base cooldown is raised to it")

	assert.Equal(t, VirtualModelRouteHealth{}, VirtualModelRouteHealth{}.Normalize(), "a disabled policy stays untouched")
}

func TestVirtualModelRouteAcceptsLegacyArrayAndObjectForms(t *testing.T) {
	var legacy map[string]VirtualModelRoute
	require.NoError(t, common.UnmarshalJsonStr(`{"auto-free":[{"model":"gpt-5.6-luna"},{"model":"gpt-5.6-terra"}]}`, &legacy))
	assert.Equal(t, VirtualModelRouteRotationOrdered, legacy["auto-free"].RotationMode())
	assert.Equal(t, []VirtualModelRouteTarget{
		{Model: "gpt-5.6-luna"},
		{Model: "gpt-5.6-terra"},
	}, legacy["auto-free"].Targets)
	assert.Equal(t, 2, len(legacy["auto-free"].Targets))

	var configured map[string]VirtualModelRoute
	require.NoError(t, common.UnmarshalJsonStr(`{"auto-free":{"rotation":"random","max_attempts":1,"targets":[{"model":"gpt-5.6-luna"},{"model":"gpt-5.6-terra"}]}}`, &configured))
	assert.Equal(t, VirtualModelRouteRotationRandom, configured["auto-free"].RotationMode())
	assert.Equal(t, 1, configured["auto-free"].MaxAttempts)
}

func TestVirtualModelRouteRotationModeNormalizesConfiguredValue(t *testing.T) {
	route := VirtualModelRoute{Rotation: "ROUND_ROBIN", Targets: []VirtualModelRouteTarget{{Model: "a"}}}
	assert.Equal(t, VirtualModelRouteRotationRoundRobin, route.RotationMode())

	route = VirtualModelRoute{Rotation: "unknown", Targets: []VirtualModelRouteTarget{{Model: "a"}}}
	assert.Equal(t, VirtualModelRouteRotationOrdered, route.RotationMode())
}

func TestVirtualPoolStickySettingNormalizeDefaultsAndBounds(t *testing.T) {
	setting := VirtualPoolStickySetting{Enabled: true}.Normalize()
	assert.Equal(t, VirtualPoolSessionModeThread, setting.SessionMode)
	assert.Equal(t, VirtualPoolBindingModeMemory, setting.BindingMode)
	assert.Equal(t, VirtualPoolMultiKeyPolicyBindIndex, setting.MultiKeyPolicy)
	assert.Equal(t, DefaultVirtualPoolPendingLeaseSeconds, setting.PendingLeaseSeconds)
	assert.Equal(t, DefaultVirtualPoolPendingRenewSeconds, setting.PendingRenewSeconds)
	assert.Equal(t, DefaultVirtualPoolConfirmedTTLSeconds, setting.ConfirmedTTLSeconds)
	assert.Equal(t, DefaultVirtualPoolClaimWaitMillis, setting.ClaimWaitMillis)
	assert.Equal(t, DefaultVirtualPoolCapacityWaitMillis, setting.CapacityWaitMillis)
	assert.Equal(t, DefaultVirtualPoolCapacityLeaseSeconds, setting.CapacityLeaseSeconds)

	bounded := VirtualPoolStickySetting{
		Enabled:              true,
		PendingLeaseSeconds:  99999,
		PendingRenewSeconds:  99999,
		ConfirmedTTLSeconds:  999999999,
		ClaimWaitMillis:      999999,
		CapacityWaitMillis:   999999,
		CapacityLeaseSeconds: 999999,
	}.Normalize()
	assert.Equal(t, 3600, bounded.PendingLeaseSeconds)
	assert.Equal(t, 3600, bounded.PendingRenewSeconds)
	assert.Equal(t, 7*24*3600, bounded.ConfirmedTTLSeconds)
	assert.Equal(t, 30000, bounded.ClaimWaitMillis)
	assert.Equal(t, 30000, bounded.CapacityWaitMillis)
	assert.Equal(t, 24*3600, bounded.CapacityLeaseSeconds)
}

func TestValidateVirtualModelRoutesCapacityBounds(t *testing.T) {
	assert.NoError(t, ValidateVirtualModelRoutes(`{"auto-free":{"targets":[{"model":"m","capacity":2,"weight":0.5,"shared_capacity_group":"ollama"}]}}`))
	assert.Error(t, ValidateVirtualModelRoutes(`{"auto-free":{"targets":[{"model":"m","capacity":-1}]}}`))
	assert.Error(t, ValidateVirtualModelRoutes(`{"auto-free":{"sources":[{"channel_id":1,"weight":-1}]}}`))
	assert.Error(t, ValidateVirtualModelRoutes(`{"auto-free":{"targets":[{"model":"m","weight":1e999}]}}`))
	assert.Error(t, ValidateVirtualModelRoutes(`{"auto-free":{"targets":[{"model":"m","shared_capacity_group":"`+strings.Repeat("x", 129)+`"}]}}`))
}

func TestVirtualPoolStickySettingAllowsUsesAllowlists(t *testing.T) {
	setting := VirtualPoolStickySetting{
		Enabled: true,
		Users:   []int{7},
		Tokens:  []int{9},
		Groups:  []string{"default"},
		Models:  []string{"deepseek v4.1 flash"},
	}
	assert.True(t, setting.Allows(7, 9, "default", "deepseek v4.1 flash"))
	assert.False(t, setting.Allows(8, 9, "default", "deepseek v4.1 flash"))
	assert.False(t, setting.Allows(7, 10, "default", "deepseek v4.1 flash"))
	assert.False(t, setting.Allows(7, 9, "vip", "deepseek v4.1 flash"))
	assert.False(t, setting.Allows(7, 9, "default", "other-model"))
}
