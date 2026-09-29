package oaichat

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIChatRequestToClaudeMessagesNormalizesToolInputSchema(t *testing.T) {
	tests := []struct {
		name       string
		parameters any
		wantSchema map[string]any
	}{
		{
			name:       "omitted parameters",
			parameters: nil,
			wantSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			name: "missing type and properties",
			parameters: map[string]any{
				"additionalProperties": false,
			},
			wantSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
			},
		},
		{
			name: "non-string type",
			parameters: map[string]any{
				"type":       123,
				"properties": map[string]any{},
			},
			wantSchema: map[string]any{
				"type":       123,
				"properties": map[string]any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxTokens := uint(1024)
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, dto.GeneralOpenAIRequest{
				Model:     "claude-test",
				MaxTokens: &maxTokens,
				Messages: []dto.Message{
					{Role: "user", Content: "Call the tool."},
				},
				Tools: []dto.ToolCallRequest{
					{
						Type: "function",
						Function: dto.FunctionRequest{
							Name:        "get_current_time",
							Description: "Get the current time",
							Parameters:  tt.parameters,
						},
					},
				},
			})

			require.NoError(t, err)
			tools, ok := got.Tools.([]any)
			require.True(t, ok)
			require.Len(t, tools, 1)
			tool, ok := tools[0].(*dto.Tool)
			require.True(t, ok)
			assert.Equal(t, "get_current_time", tool.Name)
			assert.Equal(t, tt.wantSchema, tool.InputSchema)
		})
	}
}

// TestOpenAIChatRequestToClaudeMessages_ReasoningEffortMatrix pins the five-level
// Claude effort contract (low/medium/high/xhigh/max) that the OpenAI-compatible
// chat entry must expose for the adaptive Claude 5 families:
//
//   - adaptive thinking is always on, so thinking.type is "adaptive", never
//     "enabled" with a budget, and budget_tokens is never emitted;
//   - output_config.effort carries the level verbatim when the model supports
//     it, and is absent when no effort was requested;
//   - an unknown level is rejected instead of being silently downgraded.
func TestOpenAIChatRequestToClaudeMessages_ReasoningEffortMatrix(t *testing.T) {
	maxTokens := uint(4096)

	tests := []struct {
		name             string
		model            string
		effort           string
		wantThinkingType string
		wantEffort       string
		wantAccounting   string
		wantErr          bool
	}{
		{name: "opus-5-5 minimal", model: "claude-opus-5-5", effort: "minimal", wantThinkingType: "adaptive", wantEffort: "low", wantAccounting: "low"},
		{name: "opus-5-5 low", model: "claude-opus-5-5", effort: "low", wantThinkingType: "adaptive", wantEffort: "low", wantAccounting: "low"},
		{name: "opus-5-5 medium", model: "claude-opus-5-5", effort: "medium", wantThinkingType: "adaptive", wantEffort: "medium", wantAccounting: "medium"},
		{name: "opus-5-5 high", model: "claude-opus-5-5", effort: "high", wantThinkingType: "adaptive", wantEffort: "high", wantAccounting: "high"},
		{name: "opus-5-5 xhigh", model: "claude-opus-5-5", effort: "xhigh", wantThinkingType: "adaptive", wantEffort: "xhigh", wantAccounting: "xhigh"},
		{name: "opus-5-5 max", model: "claude-opus-5-5", effort: "max", wantThinkingType: "adaptive", wantEffort: "max", wantAccounting: "max"},
		{name: "opus-5-5 none", model: "claude-opus-5-5", effort: "none", wantThinkingType: "disabled", wantAccounting: "none"},
		{name: "opus-5-5 omitted", model: "claude-opus-5-5", effort: "", wantAccounting: "high"},
		{name: "fable-5-1 minimal", model: "claude-fable-5-1", effort: "minimal", wantThinkingType: "adaptive", wantEffort: "low", wantAccounting: "low"},
		{name: "fable-5-1 low", model: "claude-fable-5-1", effort: "low", wantThinkingType: "adaptive", wantEffort: "low", wantAccounting: "low"},
		{name: "fable-5-1 medium", model: "claude-fable-5-1", effort: "medium", wantThinkingType: "adaptive", wantEffort: "medium", wantAccounting: "medium"},
		{name: "fable-5-1 high", model: "claude-fable-5-1", effort: "high", wantThinkingType: "adaptive", wantEffort: "high", wantAccounting: "high"},
		{name: "fable-5-1 xhigh", model: "claude-fable-5-1", effort: "xhigh", wantThinkingType: "adaptive", wantEffort: "xhigh", wantAccounting: "xhigh"},
		{name: "fable-5-1 max", model: "claude-fable-5-1", effort: "max", wantThinkingType: "adaptive", wantEffort: "max", wantAccounting: "max"},
		{name: "fable-5-1 none", model: "claude-fable-5-1", effort: "none", wantThinkingType: "adaptive", wantAccounting: "high"},
		{name: "fable-5-1 omitted", model: "claude-fable-5-1", effort: "", wantAccounting: "high"},
		{name: "unsupported level", model: "claude-opus-5-5", effort: "bogus", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := &convmeta.Values{}
			request := dto.GeneralOpenAIRequest{
				Model:           tt.model,
				MaxTokens:       &maxTokens,
				Messages:        []dto.Message{{Role: "user", Content: "hi"}},
				ReasoningEffort: tt.effort,
			}

			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), meta, request)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)

			if tt.wantThinkingType == "" {
				assert.Nil(t, got.Thinking)
			} else {
				require.NotNil(t, got.Thinking)
				assert.Equal(t, tt.wantThinkingType, got.Thinking.Type)
			}
			if got.Thinking != nil {
				assert.Nil(t, got.Thinking.BudgetTokens, "adaptive Claude thinking never sends budget_tokens")
			}

			if tt.wantEffort == "" {
				assert.Empty(t, got.OutputConfig)
			} else {
				assert.JSONEq(t, `{"effort":"`+tt.wantEffort+`"}`, string(got.OutputConfig))
			}
			assert.Equal(t, tt.wantAccounting, meta.GetReasoningEffort())
		})
	}
}

// TestOpenAIChatRequestToClaudeMessages_EffortNormalizationPerModelFamily guards
// the capability table itself: a level the model cannot express must be
// normalized to the closest supported one instead of being passed through.
func TestOpenAIChatRequestToClaudeMessages_EffortNormalizationPerModelFamily(t *testing.T) {
	maxTokens := uint(4096)

	tests := []struct {
		name             string
		model            string
		effort           string
		wantThinkingType string
		wantEffort       string
	}{
		{name: "opus-5-5 keeps xhigh", model: "claude-opus-5-5", effort: "xhigh", wantThinkingType: "adaptive", wantEffort: "xhigh"},
		{name: "opus-4-6 lifts xhigh to max", model: "claude-opus-4-6", effort: "xhigh", wantThinkingType: "adaptive", wantEffort: "max"},
		{name: "opus-4-5 caps xhigh at high", model: "claude-opus-4-5", effort: "xhigh", wantThinkingType: "enabled", wantEffort: "high"},
		{name: "opus-4-5 keeps disabled when thinking is off", model: "claude-opus-4-5", effort: "none", wantThinkingType: "disabled"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := dto.GeneralOpenAIRequest{
				Model:           tt.model,
				MaxTokens:       &maxTokens,
				Messages:        []dto.Message{{Role: "user", Content: "hi"}},
				ReasoningEffort: tt.effort,
			}

			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), &convmeta.Values{}, request)
			require.NoError(t, err)
			require.NotNil(t, got)

			if tt.wantThinkingType == "" {
				assert.Nil(t, got.Thinking)
			} else {
				require.NotNil(t, got.Thinking)
				assert.Equal(t, tt.wantThinkingType, got.Thinking.Type)
			}
			if tt.wantEffort == "" {
				assert.Empty(t, got.OutputConfig)
			} else {
				assert.JSONEq(t, `{"effort":"`+tt.wantEffort+`"}`, string(got.OutputConfig))
			}
		})
	}
}
