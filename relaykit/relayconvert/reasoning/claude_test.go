package reasoning

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Opus 5.5 rejects thinking.type="disabled" upstream, so the fork must never
// derive a disabled thinking block for the claude-opus-5 family. The capability
// table is what drives that decision, and the manual families must keep the
// upstream behaviour.
func TestClaudeCapabilityTableDisableSupport(t *testing.T) {
	tests := []struct {
		model           string
		wantAdaptive    bool
		wantManual      bool
		wantDisable     bool
		wantDefault     bool
		wantXHighAndMax bool
	}{
		{model: "claude-opus-5-5", wantAdaptive: true, wantManual: false, wantDisable: false, wantDefault: true, wantXHighAndMax: true},
		{model: "claude-opus-5", wantAdaptive: true, wantManual: false, wantDisable: false, wantDefault: true, wantXHighAndMax: true},
		{model: "claude-fable-5-1", wantAdaptive: true, wantManual: false, wantDisable: false, wantDefault: true, wantXHighAndMax: true},
		{model: "claude-sonnet-5", wantAdaptive: true, wantManual: false, wantDisable: true, wantDefault: true, wantXHighAndMax: true},
		{model: "claude-opus-4-8", wantAdaptive: true, wantManual: false, wantDisable: true, wantDefault: false, wantXHighAndMax: true},
		{model: "claude-opus-4-5", wantAdaptive: false, wantManual: true, wantDisable: true, wantDefault: false, wantXHighAndMax: false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := claudeCapabilitiesFor(tt.model)
			assert.Equal(t, tt.wantAdaptive, got.adaptive)
			assert.Equal(t, tt.wantManual, got.supportsManual)
			assert.Equal(t, tt.wantDisable, got.supportsDisable)
			assert.Equal(t, tt.wantDefault, got.defaultThinking)
			assert.Equal(t, tt.wantXHighAndMax, got.supportsXHigh && got.supportsMax)
		})
	}
}

// The product requirement fixes the adaptive Claude 5 default tiers: Opus 5.5
// defaults to medium while Fable 5.1 stays at high. Requests without an effort
// signal must be accounted at the family default while leaving
// output_config.effort absent, so the gateway never invents a level the caller
// did not ask for (mirrors geminiDefaultEffort).
func TestClaudeDefaultEffort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		model string
		// wantDefault is the family tier reported by claudeDefaultEffort.
		wantDefault Effort
		// wantAccounting is the tier a request without any effort signal is
		// rendered with. Families that apply no default keep an empty effort.
		wantAccounting Effort
	}{
		{model: "claude-opus-5-5", wantDefault: EffortMedium, wantAccounting: EffortMedium},
		{model: "claude-opus-5", wantDefault: EffortMedium, wantAccounting: EffortMedium},
		{model: "claude-fable-5-1", wantDefault: EffortHigh, wantAccounting: EffortHigh},
		{model: "claude-sonnet-5", wantDefault: EffortHigh, wantAccounting: EffortHigh},
		{model: "claude-opus-4-5", wantDefault: EffortHigh, wantAccounting: Effort("")},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.wantDefault, claudeDefaultEffort(tt.model))

			got, err := RenderClaude(tt.model, Intent{}, nil, 0)
			require.NoError(t, err)
			assert.Nil(t, got.Thinking)
			assert.Equal(t, Effort(""), got.OutputEffort)
			assert.Equal(t, tt.wantAccounting, got.EffectiveEffort)
		})
	}
}
