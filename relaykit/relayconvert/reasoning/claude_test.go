package reasoning

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Opus 5.5 rejects thinking.type="disabled" upstream, so the fork must never
// derive a disabled thinking block for the claude-opus-5 family. The capability
// table is what drives that decision, and the manual families must keep the
// upstream behaviour (see merge-rc40-tdd-plan.md, B类).
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
