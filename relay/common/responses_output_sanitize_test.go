package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestStripResponsesEncryptedContentReasoning(t *testing.T) {
	body := []byte(`{"type":"response.output_item.done","item":{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"plan"}],"encrypted_content":"gAAAAABciphertext"}}`)

	got := StripResponsesEncryptedContent(body)

	assert.False(t, gjson.GetBytes(got, "item.encrypted_content").Exists())
	assert.Equal(t, "plan", gjson.GetBytes(got, "item.summary.0.text").String())
	assert.Equal(t, "reasoning", gjson.GetBytes(got, "item.type").String())
}

func TestStripResponsesEncryptedContentAgentMessage(t *testing.T) {
	body := []byte(`{"type":"response.output_item.done","item":{"type":"agent_message","id":"amsg_1","content":[{"type":"input_text","text":"head"},{"type":"encrypted_content","encrypted_content":"gAAAAABciphertext"}]}}`)

	got := StripResponsesEncryptedContent(body)

	assert.False(t, gjson.GetBytes(got, "item.content.1").Exists())
	assert.False(t, gjson.GetBytes(got, "item.content.0.encrypted_content").Exists())
	assert.Equal(t, "head", gjson.GetBytes(got, "item.content.0.text").String())
}

func TestStripResponsesEncryptedContentNested(t *testing.T) {
	body := []byte(`{"a":{"b":[{"encrypted_content":"x"},{"c":{"encrypted_content":"y","keep":1}}]}}`)

	got := StripResponsesEncryptedContent(body)

	// The first array entry was removed entirely, so the surviving object
	// shifts to index 0.
	assert.False(t, gjson.GetBytes(got, "a.b.0.encrypted_content").Exists())
	assert.False(t, gjson.GetBytes(got, "a.b.0.c.encrypted_content").Exists())
	assert.Equal(t, int64(1), gjson.GetBytes(got, "a.b.0.c.keep").Int())
}

func TestStripResponsesEncryptedContentLeavesCleanBodyUnchanged(t *testing.T) {
	body := []byte(`{"type":"response.output_text.delta","delta":"hello"}`)

	got := StripResponsesEncryptedContent(body)

	require.Equal(t, body, got)
}

func TestStripResponsesEncryptedContentInvalidJSON(t *testing.T) {
	body := []byte(`{"encrypted_content": "truncated`)

	got := StripResponsesEncryptedContent(body)

	require.Equal(t, body, got)
}

func TestStripResponsesEncryptedContentPlaintextKeywordOnly(t *testing.T) {
	// The word appears inside string values (e.g. tool output quoting code),
	// but never as a JSON key: the payload must pass through byte-identical.
	body := []byte(`{"type":"function_call_output","output":"file mentions encrypted_content in text"}`)

	got := StripResponsesEncryptedContent(body)

	require.Equal(t, body, got)
}
