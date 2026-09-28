package operation_setting

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	DefaultModelHealthFailureThreshold = 2
	DefaultModelHealthCooldownSeconds  = 300
)

var modelHealthRegexCache = struct {
	sync.RWMutex
	values map[string]*regexp.Regexp
}{values: make(map[string]*regexp.Regexp)}

func matchModelHealthRegex(pattern string, value string) (bool, error) {
	modelHealthRegexCache.RLock()
	compiled := modelHealthRegexCache.values[pattern]
	modelHealthRegexCache.RUnlock()
	if compiled == nil {
		var err error
		compiled, err = regexp.Compile(pattern)
		if err != nil {
			return false, err
		}
		modelHealthRegexCache.Lock()
		if modelHealthRegexCache.values == nil {
			modelHealthRegexCache.values = make(map[string]*regexp.Regexp)
		}
		modelHealthRegexCache.values[pattern] = compiled
		modelHealthRegexCache.Unlock()
	}
	return compiled.MatchString(value), nil
}

// ModelHealthPolicyRule applies model-level channel cooldown to ordinary model
// names without requiring a virtual model route.
type ModelHealthPolicyRule struct {
	Name             string   `json:"name"`
	Enabled          bool     `json:"enabled"`
	Models           []string `json:"models,omitempty"`
	ModelRegex       []string `json:"model_regex,omitempty"`
	FailureThreshold int      `json:"failure_threshold,omitempty"`
	CooldownSeconds  int      `json:"cooldown_seconds,omitempty"`
	StatusCodes      []int    `json:"status_codes,omitempty"`
	Groups           []string `json:"groups,omitempty"`
}

type ModelHealthPolicySetting struct {
	Enabled bool                    `json:"enabled"`
	Rules   []ModelHealthPolicyRule `json:"rules"`
}

var modelHealthPolicySetting = ModelHealthPolicySetting{
	Rules: []ModelHealthPolicyRule{},
}

func init() {
	config.GlobalConfig.Register("model_health_policy_setting", &modelHealthPolicySetting)
}

func GetModelHealthPolicySetting() *ModelHealthPolicySetting {
	return &modelHealthPolicySetting
}

func (rule ModelHealthPolicyRule) Normalize() ModelHealthPolicyRule {
	if rule.FailureThreshold <= 0 {
		rule.FailureThreshold = DefaultModelHealthFailureThreshold
	}
	if rule.CooldownSeconds <= 0 {
		rule.CooldownSeconds = DefaultModelHealthCooldownSeconds
	}
	if len(rule.StatusCodes) == 0 {
		rule.StatusCodes = []int{404, 429, 500, 502, 503, 504}
	}
	return rule
}

func (rule ModelHealthPolicyRule) MatchesModel(modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return false
	}
	for _, candidate := range rule.Models {
		if strings.EqualFold(strings.TrimSpace(candidate), modelName) {
			return true
		}
	}
	for _, pattern := range rule.ModelRegex {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if matched, err := matchModelHealthRegex(pattern, modelName); err == nil && matched {
			return true
		}
	}
	return false
}

func (rule ModelHealthPolicyRule) MatchesGroup(group string) bool {
	if len(rule.Groups) == 0 {
		return true
	}
	for _, candidate := range rule.Groups {
		if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(group)) {
			return true
		}
	}
	return false
}

func (rule ModelHealthPolicyRule) AllowsStatus(statusCode int) bool {
	for _, configured := range rule.Normalize().StatusCodes {
		if configured == statusCode {
			return true
		}
	}
	return false
}

// MatchModelHealthPolicy returns the first enabled rule that matches the model
// and group. The returned policy is normalized so callers can use it directly.
func MatchModelHealthPolicy(modelName string, group string) (ModelHealthPolicyRule, bool) {
	setting := modelHealthPolicySetting
	if !setting.Enabled {
		return ModelHealthPolicyRule{}, false
	}
	for _, rule := range setting.Rules {
		if !rule.Enabled || !rule.MatchesGroup(group) || !rule.MatchesModel(modelName) {
			continue
		}
		return rule.Normalize(), true
	}
	return ModelHealthPolicyRule{}, false
}

func ValidateModelHealthPolicy(value string) error {
	var rules []ModelHealthPolicyRule
	if strings.HasPrefix(strings.TrimSpace(value), "[") {
		if err := common.UnmarshalJsonStr(value, &rules); err != nil {
			return fmt.Errorf("invalid model health policy rules JSON: %w", err)
		}
	} else {
		var setting ModelHealthPolicySetting
		if err := common.UnmarshalJsonStr(value, &setting); err != nil {
			return fmt.Errorf("invalid model health policy JSON: %w", err)
		}
		rules = setting.Rules
	}
	for index, rule := range rules {
		if strings.TrimSpace(rule.Name) == "" {
			return fmt.Errorf("model health policy rule %d needs a name", index)
		}
		if len(rule.Models) == 0 && len(rule.ModelRegex) == 0 {
			return fmt.Errorf("model health policy rule %q needs models or model_regex", rule.Name)
		}
		if rule.FailureThreshold < 0 {
			return fmt.Errorf("model health policy rule %q has a negative failure_threshold", rule.Name)
		}
		if rule.CooldownSeconds < 0 {
			return fmt.Errorf("model health policy rule %q has a negative cooldown_seconds", rule.Name)
		}
		for _, statusCode := range rule.StatusCodes {
			if statusCode < 100 || statusCode > 599 {
				return fmt.Errorf("model health policy rule %q has an invalid status code %d", rule.Name, statusCode)
			}
		}
		for _, pattern := range rule.ModelRegex {
			if strings.TrimSpace(pattern) == "" {
				return fmt.Errorf("model health policy rule %q has an empty model_regex", rule.Name)
			}
			if _, err := matchModelHealthRegex(pattern, "model"); err != nil {
				return fmt.Errorf("model health policy rule %q has an invalid model_regex %q: %w", rule.Name, pattern, err)
			}
		}
	}
	return nil
}
