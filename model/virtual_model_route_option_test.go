package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateOptionValueRejectsInvalidVirtualModelRoutes(t *testing.T) {
	const key = "model_retry_policy_setting.virtual_model_routes"

	assert.NoError(t, validateOptionValue(
		key,
		`{"auto-subagent":[{"model":"gpt-5.6-luna"},{"model":"grok-4.5","reasoning_effort_map":{"max":"high"}}]}`,
	))
	assert.NoError(t, validateOptionValue(
		key,
		`{"auto-free":{"rotation":"round_robin","max_attempts":3,"targets":[{"model":"gpt-5.6-luna"},{"model":"nvidia/nemotron-3-super-120b-a12b"}]}}`,
	))
	assert.Error(t, validateOptionValue(key, `{"auto-subagent":[]}`))
	assert.Error(t, validateOptionValue(key, `{"auto-free":{"rotation":"shuffle","targets":[{"model":"gpt-5.6-luna"}]}}`))
	assert.Error(t, validateOptionValue(key, `{"auto-free":{"max_attempts":-1,"targets":[{"model":"gpt-5.6-luna"}]}}`))
	assert.Error(t, validateOptionValue(key, `not-json`))
}
