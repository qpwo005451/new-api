package middleware

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeVirtualModelMappingPreservesExistingMappings(t *testing.T) {
	mapping := mergeVirtualModelMapping(
		`{"other-alias":"other-upstream","auto-subagent-codex":"stale-model"}`,
		"auto-subagent-codex",
		"gpt-5.6-terra",
	)

	var decoded map[string]string
	require.NoError(t, common.UnmarshalJsonStr(mapping, &decoded))
	assert.Equal(t, "other-upstream", decoded["other-alias"])
	assert.Equal(t, "gpt-5.6-terra", decoded["auto-subagent-codex"])
}
