package common

import (
	"testing"

	appcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
