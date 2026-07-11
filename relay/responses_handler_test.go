package relay

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterImageGenerationTool(t *testing.T) {
	input := []byte(`{"tools":[{"type":"function","name":"keep"},{"type":"image_generation"}],"model":"test"}`)
	filtered := filterImageGenerationTool(input)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(filtered, &payload))
	tools, ok := payload["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)
	require.Equal(t, "function", tools[0].(map[string]any)["type"])
}

func TestFilterImageGenerationToolLeavesUnchangedPayload(t *testing.T) {
	input := []byte(`{"tools":[{"type":"function","name":"keep"}],"model":"test"}`)
	require.Equal(t, input, filterImageGenerationTool(input))
}
