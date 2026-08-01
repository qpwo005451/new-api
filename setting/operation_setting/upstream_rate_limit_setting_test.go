package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateUpstreamRateLimitRules(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{
			name:  "valid rule",
			value: `[{"name":"Input Kimi","base_url_host":"ai.input.im","models":["kimi-k2.7-code"],"rpm":10,"cooldown_seconds":60}]`,
			valid: true,
		},
		{
			name:  "invalid JSON",
			value: `{}`,
			valid: false,
		},
		{
			name:  "missing model",
			value: `[{"name":"Input Kimi","base_url_host":"ai.input.im","models":[],"rpm":10,"cooldown_seconds":60}]`,
			valid: false,
		},
		{
			name:  "invalid RPM",
			value: `[{"name":"Input Kimi","base_url_host":"ai.input.im","models":["kimi-k2.7-code"],"rpm":0,"cooldown_seconds":60}]`,
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpstreamRateLimitRules(tt.value)
			if tt.valid {
				assert.NoError(t, err)
				return
			}
			assert.Error(t, err)
		})
	}
}
