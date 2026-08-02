package common

import (
	"testing"

	appcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesDiagnosticsCapturesMetadataOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	req := &dto.OpenAIResponsesRequest{
		Input: []byte(`[
			{"type":"reasoning","encrypted_content":"secret-ciphertext"},
			{"type":"function_call_output","call_id":"call_1","output":"secret tool result"}
		]`),
		PreviousResponseID: "resp_secret",
		Tools:              []byte(`[{"type":"function","name":"shell"}]`),
	}

	CaptureResponsesRequestDiagnostics(ctx, req)
	info := &RelayInfo{ChannelMeta: &ChannelMeta{ChannelId: 27}}
	RecordResponsesStreamDiagnostic(ctx, info, dto.ResponsesStreamResponse{Type: "response.created"})
	RecordResponsesStreamDiagnostic(ctx, info, dto.ResponsesStreamResponse{
		Type: "response.output_item.added",
		Item: &dto.ResponsesOutput{Type: "function_call", ID: "item_secret"},
	})
	RecordResponsesStreamDiagnostic(ctx, info, dto.ResponsesStreamResponse{
		Type:    "response.failed",
		Message: "Encrypted function output content could not be decrypted or decoded.",
		Code:    "invalid_encrypted_content",
	})

	adminInfo := map[string]interface{}{}
	apiErr := types.WithOpenAIError(types.OpenAIError{
		Message: "Encrypted function output content could not be decrypted or decoded.",
		Code:    "invalid_encrypted_content",
	}, 502)
	AppendResponsesDiagnosticsAdminInfo(ctx, apiErr, adminInfo)

	rawDiag, ok := adminInfo["responses_diagnostics"]
	require.True(t, ok)
	diag, ok := rawDiag.(*ResponsesDiagnostics)
	require.True(t, ok)
	require.NotNil(t, diag.Request)
	require.NotNil(t, diag.Stream)

	assert.Equal(t, 2, diag.Request.InputItemCount)
	assert.Equal(t, []string{"reasoning", "function_call_output"}, diag.Request.InputItemTypes)
	assert.Equal(t, 1, diag.Request.EncryptedContentCount)
	assert.Equal(t, 1, diag.Request.FunctionCallOutputCount)
	assert.Equal(t, 1, diag.Request.FunctionOutputPresentCount)
	assert.Equal(t, 1, diag.Request.FunctionOutputStringCount)
	assert.Equal(t, 1, diag.Request.FunctionOutputJSONTypes["string"])
	assert.True(t, diag.Request.PreviousResponseIDPresent)
	assert.Equal(t, 3, diag.Stream.EventCount)
	assert.Equal(t, 1, diag.Stream.ResponseFailedEventCount)
	assert.Len(t, diag.Stream.FirstEvents, 3)
	assert.Equal(t, 27, diag.Stream.FirstEvents[0].ChannelID)

	data, err := appcommon.Marshal(diag)
	require.NoError(t, err)
	serialized := string(data)
	assert.NotContains(t, serialized, "secret-ciphertext")
	assert.NotContains(t, serialized, "secret tool result")
	assert.NotContains(t, serialized, "resp_secret")
	assert.NotContains(t, serialized, "item_secret")
	assert.Contains(t, serialized, "metadata_only_no_prompt_no_tool_output_no_encrypted_content")
}

func TestResponsesDiagnosticsSkippedForOrdinaryErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	CaptureResponsesRequestDiagnostics(ctx, &dto.OpenAIResponsesRequest{
		Input: []byte(`[{"type":"message","content":"hello"}]`),
	})

	adminInfo := map[string]interface{}{}
	AppendResponsesDiagnosticsAdminInfo(ctx, types.NewOpenAIError(
		assert.AnError,
		types.ErrorCodeBadResponse,
		502,
	), adminInfo)

	_, exists := adminInfo["responses_diagnostics"]
	assert.False(t, exists)
}

func TestRemoveResponsesInputItemStatus(t *testing.T) {
	body := []byte(`{"model":"gpt-test","status":"top-level","metadata":{"status":"kept"},"input":[{"type":"message","role":"assistant","status":"completed","content":[]},{"type":"reasoning","id":"rs_1","status":"completed","summary":[]},{"type":"function_call","id":"fc_1","status":"completed","call_id":"call_1","name":"shell","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)

	got, changed, err := RemoveResponsesInputItemStatus(body)
	require.NoError(t, err)
	require.True(t, changed)

	assert.Equal(t, "top-level", gjson.GetBytes(got, "status").String())
	assert.Equal(t, "kept", gjson.GetBytes(got, "metadata.status").String())
	assert.False(t, gjson.GetBytes(got, "input.0.status").Exists())
	assert.False(t, gjson.GetBytes(got, "input.1.status").Exists())
	assert.False(t, gjson.GetBytes(got, "input.2.status").Exists())
	assert.False(t, gjson.GetBytes(got, "input.3.status").Exists())
	assert.Equal(t, "function_call_output", gjson.GetBytes(got, "input.3.type").String())
}

func TestRemoveResponsesInputItemStatusLeavesCleanBodyUnchanged(t *testing.T) {
	body := []byte(`{"model":"gpt-test","input":[{"type":"message","role":"user","content":"hello"}]}`)

	got, changed, err := RemoveResponsesInputItemStatus(body)
	require.NoError(t, err)

	assert.False(t, changed)
	assert.Equal(t, body, got)
}
