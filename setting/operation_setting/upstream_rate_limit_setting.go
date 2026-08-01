package operation_setting

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// UpstreamRateLimitRule spaces relay starts for one upstream host/model pair.
// Limits are scoped by upstream host, selected key, and model.
type UpstreamRateLimitRule struct {
	Name            string   `json:"name"`
	BaseURLHost     string   `json:"base_url_host"`
	Models          []string `json:"models"`
	RPM             int      `json:"rpm"`
	CooldownSeconds int      `json:"cooldown_seconds"`
}

type UpstreamRateLimitSetting struct {
	Enabled bool                    `json:"enabled"`
	Rules   []UpstreamRateLimitRule `json:"rules"`
}

var upstreamRateLimitSetting = UpstreamRateLimitSetting{
	Enabled: true,
	Rules: []UpstreamRateLimitRule{
		{
			Name:            "Input Kimi K2.7 Code",
			BaseURLHost:     "ai.input.im",
			Models:          []string{"kimi-k2.7-code"},
			RPM:             10,
			CooldownSeconds: 60,
		},
	},
}

func init() {
	config.GlobalConfig.Register("upstream_rate_limit_setting", &upstreamRateLimitSetting)
}

func GetUpstreamRateLimitSetting() *UpstreamRateLimitSetting {
	return &upstreamRateLimitSetting
}

func ValidateUpstreamRateLimitRules(value string) error {
	var rules []UpstreamRateLimitRule
	if err := common.UnmarshalJsonStr(value, &rules); err != nil {
		return fmt.Errorf("upstream RPM rules must be a JSON array: %w", err)
	}
	for index, rule := range rules {
		if strings.TrimSpace(rule.Name) == "" {
			return fmt.Errorf("upstream RPM rule %d requires a name", index+1)
		}
		if strings.TrimSpace(rule.BaseURLHost) == "" {
			return fmt.Errorf("upstream RPM rule %d requires a base URL host", index+1)
		}
		if len(rule.Models) == 0 {
			return fmt.Errorf("upstream RPM rule %d requires at least one model", index+1)
		}
		for _, modelName := range rule.Models {
			if strings.TrimSpace(modelName) == "" {
				return fmt.Errorf("upstream RPM rule %d contains an empty model", index+1)
			}
		}
		if rule.RPM < 1 || rule.RPM > 600 {
			return fmt.Errorf("upstream RPM rule %d must set rpm between 1 and 600", index+1)
		}
		if rule.CooldownSeconds < 0 || rule.CooldownSeconds > 3600 {
			return fmt.Errorf("upstream RPM rule %d must set cooldown_seconds between 0 and 3600", index+1)
		}
	}
	return nil
}
