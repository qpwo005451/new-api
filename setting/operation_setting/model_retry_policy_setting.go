package operation_setting

import (
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// Virtual model route rotations. "ordered" keeps the configured target order,
// so every request starts at the first target. "random" and "round_robin" pick
// the starting target per request and spread the pool across requests.
const (
	VirtualModelRouteRotationOrdered    = "ordered"
	VirtualModelRouteRotationRandom     = "random"
	VirtualModelRouteRotationRoundRobin = "round_robin"
)

type VirtualModelRouteTarget struct {
	Model              string            `json:"model"`
	ReasoningEffortMap map[string]string `json:"reasoning_effort_map,omitempty"`
}

// Health policy defaults for routes that enable availability based selection.
const (
	DefaultVirtualModelRouteFailureThreshold = 2
	DefaultVirtualModelRouteCooldownSeconds  = 60
	DefaultVirtualModelRouteMaxCooldownSecs  = 600
)

// VirtualModelRouteHealth turns observed failures into a cooldown, so a pool
// entry that stopped serving is used only after the healthy entries were tried.
// The signal comes from relayed traffic; no probe or extra request is issued.
type VirtualModelRouteHealth struct {
	Enabled            bool `json:"enabled"`
	FailureThreshold   int  `json:"failure_threshold,omitempty"`
	CooldownSeconds    int  `json:"cooldown_seconds,omitempty"`
	MaxCooldownSeconds int  `json:"max_cooldown_seconds,omitempty"`
}

// Normalize fills the documented defaults so callers read concrete values.
func (health VirtualModelRouteHealth) Normalize() VirtualModelRouteHealth {
	if !health.Enabled {
		return health
	}
	if health.FailureThreshold <= 0 {
		health.FailureThreshold = DefaultVirtualModelRouteFailureThreshold
	}
	if health.CooldownSeconds <= 0 {
		health.CooldownSeconds = DefaultVirtualModelRouteCooldownSeconds
	}
	if health.MaxCooldownSeconds <= 0 {
		health.MaxCooldownSeconds = DefaultVirtualModelRouteMaxCooldownSecs
	}
	if health.MaxCooldownSeconds < health.CooldownSeconds {
		health.MaxCooldownSeconds = health.CooldownSeconds
	}
	return health
}

// VirtualModelRouteSource derives pool entries from a channel model list, so an
// aggregate model follows the channel instead of a hand-kept target list. The
// channel stays the single place to add or remove models.
type VirtualModelRouteSource struct {
	ChannelId int `json:"channel_id"`
}

// VirtualModelRoute describes one virtual model: the upstream models a request
// may be sent to, how the first attempt picks a target, how many pool entries a
// single request may try, and whether observed failures move an entry to the
// back of the pool. A bare target array is still accepted, which is the ordered
// form without an attempt limit.
type VirtualModelRoute struct {
	Rotation    string                    `json:"rotation,omitempty"`
	MaxAttempts int                       `json:"max_attempts,omitempty"`
	Health      VirtualModelRouteHealth   `json:"health,omitempty"`
	Sources     []VirtualModelRouteSource `json:"sources,omitempty"`
	Targets     []VirtualModelRouteTarget `json:"targets,omitempty"`
}

// HasPool reports whether the route has anything to route to.
func (route VirtualModelRoute) HasPool() bool {
	return len(route.Targets) > 0 || len(route.Sources) > 0
}

// UnmarshalJSON accepts both the route object and the legacy target array.
func (route *VirtualModelRoute) UnmarshalJSON(data []byte) error {
	if strings.HasPrefix(strings.TrimSpace(string(data)), "[") {
		var targets []VirtualModelRouteTarget
		if err := common.Unmarshal(data, &targets); err != nil {
			return err
		}
		route.Targets = targets
		return nil
	}
	// The alias type keeps this method from recursing into itself.
	type routeAlias VirtualModelRoute
	var parsed routeAlias
	if err := common.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*route = VirtualModelRoute(parsed)
	return nil
}

// RotationMode returns the configured rotation, defaulting to the ordered walk
// so routes configured before rotations existed keep their behavior.
func (route VirtualModelRoute) RotationMode() string {
	switch strings.ToLower(strings.TrimSpace(route.Rotation)) {
	case VirtualModelRouteRotationRandom:
		return VirtualModelRouteRotationRandom
	case VirtualModelRouteRotationRoundRobin:
		return VirtualModelRouteRotationRoundRobin
	default:
		return VirtualModelRouteRotationOrdered
	}
}

// ModelRetryPolicySetting controls models that should visit each channel
// priority exactly once. Other models keep the gateway-wide retry behavior.
type ModelRetryPolicySetting struct {
	SinglePassPriorityModels []string                     `json:"single_pass_priority_models"`
	VirtualModelRoutes       map[string]VirtualModelRoute `json:"virtual_model_routes"`
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

func GetVirtualModelRoute(modelName string) VirtualModelRoute {
	for configuredModel, route := range modelRetryPolicySetting.VirtualModelRoutes {
		if !strings.EqualFold(strings.TrimSpace(configuredModel), strings.TrimSpace(modelName)) {
			continue
		}
		// Return a copy so routing never mutates the registered configuration.
		routed := VirtualModelRoute{
			Rotation:    route.Rotation,
			MaxAttempts: route.MaxAttempts,
			Health:      route.Health.Normalize(),
		}
		routed.Sources = make([]VirtualModelRouteSource, len(route.Sources))
		copy(routed.Sources, route.Sources)
		routed.Targets = make([]VirtualModelRouteTarget, 0, len(route.Targets))
		for _, target := range route.Targets {
			effortMap := make(map[string]string, len(target.ReasoningEffortMap))
			for effort, mappedEffort := range target.ReasoningEffortMap {
				effortMap[effort] = mappedEffort
			}
			routed.Targets = append(routed.Targets, VirtualModelRouteTarget{
				Model:              target.Model,
				ReasoningEffortMap: effortMap,
			})
		}
		return routed
	}
	return VirtualModelRoute{}
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
	var routes map[string]VirtualModelRoute
	if err := common.UnmarshalJsonStr(value, &routes); err != nil {
		return fmt.Errorf("invalid virtual model routes JSON: %w", err)
	}
	for virtualModel, route := range routes {
		if strings.TrimSpace(virtualModel) == "" {
			return fmt.Errorf("virtual model name cannot be empty")
		}
		if !route.HasPool() {
			return fmt.Errorf("virtual model %q must contain at least one route target or source", virtualModel)
		}
		switch strings.ToLower(strings.TrimSpace(route.Rotation)) {
		case "", VirtualModelRouteRotationOrdered, VirtualModelRouteRotationRandom, VirtualModelRouteRotationRoundRobin:
		default:
			return fmt.Errorf("virtual model %q has unsupported rotation %q", virtualModel, route.Rotation)
		}
		if route.MaxAttempts < 0 {
			return fmt.Errorf("virtual model %q cannot have a negative max_attempts", virtualModel)
		}
		if route.Health.FailureThreshold < 0 || route.Health.CooldownSeconds < 0 || route.Health.MaxCooldownSeconds < 0 {
			return fmt.Errorf("virtual model %q has a negative health value", virtualModel)
		}
		for index, source := range route.Sources {
			if source.ChannelId <= 0 {
				return fmt.Errorf("virtual model %q source %d needs a positive channel_id", virtualModel, index)
			}
		}
		for index, target := range route.Targets {
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

// GetVirtualModelRouteNames lists the configured virtual model names that have
// a pool, so callers can expose them without a channel model list entry.
func GetVirtualModelRouteNames() []string {
	names := make([]string, 0, len(modelRetryPolicySetting.VirtualModelRoutes))
	for name, route := range modelRetryPolicySetting.VirtualModelRoutes {
		if strings.TrimSpace(name) == "" || !route.HasPool() {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func GetModelRetryPolicySetting() *ModelRetryPolicySetting {
	return &modelRetryPolicySetting
}
