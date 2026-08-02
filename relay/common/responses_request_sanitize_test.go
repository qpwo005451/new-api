package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRemoveResponsesInputEncryptedContent(t *testing.T) {
	body := []byte(`{"model":"gpt-test","input":[{"type":"reasoning","summary":[{"type":"summary_text","text":"plan"}],"encrypted_content":"ciphertext"},{"type":"agent_message","content":[{"type":"input_text","text":"head"},{"type":"encrypted_content","encrypted_content":"ciphertext"}]},{"type":"function_call_output","call_id":"call_1","output":"ok"}],"metadata":{"encrypted_content":"keep-outside-input","large":9007199254740993}}`)

	got, changed, err := RemoveResponsesInputEncryptedContent(body)
	require.NoError(t, err)
	require.True(t, changed)

	assert.False(t, gjson.GetBytes(got, "input.0.encrypted_content").Exists())
	assert.Equal(t, "plan", gjson.GetBytes(got, "input.0.summary.0.text").String())
	assert.Equal(t, "head", gjson.GetBytes(got, "input.1.content.0.text").String())
	assert.False(t, gjson.GetBytes(got, "input.1.content.1").Exists())
	assert.Equal(t, "ok", gjson.GetBytes(got, "input.2.output").String())
	assert.Equal(t, "keep-outside-input", gjson.GetBytes(got, "metadata.encrypted_content").String())
	assert.Equal(t, "9007199254740993", gjson.GetBytes(got, "metadata.large").Raw)
}

func TestRemoveResponsesInputEncryptedContentLeavesCleanBodyUnchanged(t *testing.T) {
	body := []byte(`{"model":"gpt-test","input":[{"type":"message","role":"user","content":"hello"}]}`)

	got, changed, err := RemoveResponsesInputEncryptedContent(body)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, body, got)
}

func TestRemoveResponsesInputEncryptedContentIgnoresOtherTopLevelFields(t *testing.T) {
	body := []byte(`{"model":"gpt-test","input":[{"type":"message","content":"hello"}],"metadata":{"encrypted_content":"keep"}}`)

	got, changed, err := RemoveResponsesInputEncryptedContent(body)
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, body, got)
}

func TestRemoveResponsesInputEncryptedContentRejectsInvalidInputJSON(t *testing.T) {
	body := []byte(`{"model":"gpt-test","input":[{"encrypted_content":]}`)

	got, changed, err := RemoveResponsesInputEncryptedContent(body)
	require.Error(t, err)
	assert.False(t, changed)
	assert.Equal(t, body, got)
}
