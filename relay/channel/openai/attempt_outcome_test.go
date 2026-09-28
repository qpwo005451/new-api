package openai

import (
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOaiStreamHandlerRecordsProtocolCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setChatStreamTestTimeout(t)

	body := strings.Join([]string{
		`data: {"id":"chatcmpl-outcome-ok","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"OK"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-outcome-ok","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	c, _, resp, info := newChatStreamTestContext(strings.NewReader(body))
	info.AttemptOutcome = &relaycommon.AttemptRecorder{}
	require.NoError(t, info.AttemptOutcome.Begin("attempt-1", "candidate-1"))

	_, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, apiErr)

	outcome, err := info.AttemptOutcome.Snapshot()
	require.NoError(t, err)
	assert.True(t, outcome.ProtocolCompleted)
	assert.Equal(t, relaycommon.AttemptFailureNone, outcome.FailureCategory)
	assert.Equal(t, relaycommon.AttemptUpstreamAccepted, outcome.UpstreamState)
}

func TestOaiStreamHandlerMarksTruncatedStreamCommittedAndNonReplayable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setChatStreamTestTimeout(t)

	body := strings.Join([]string{
		`data: {"id":"chatcmpl-outcome-truncated","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}`,
		``,
	}, "\n")
	c, _, resp, info := newChatStreamTestContext(strings.NewReader(body))
	info.AttemptOutcome = &relaycommon.AttemptRecorder{}
	require.NoError(t, info.AttemptOutcome.Begin("attempt-2", "candidate-2"))

	_, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, apiErr)

	outcome, err := info.AttemptOutcome.Snapshot()
	require.NoError(t, err)
	assert.False(t, outcome.ProtocolCompleted)
	assert.True(t, outcome.DownstreamCommitted)
	assert.Equal(t, relaycommon.AttemptFailureProtocol, outcome.FailureCategory)
	assert.False(t, outcome.CanRetry())
}

func TestOaiResponsesStreamHandlerRecordsTerminalProtocolCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setChatStreamTestTimeout(t)

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_outcome","model":"grok-4.5"}}`,
		`data: {"type":"response.output_text.delta","delta":"OK"}`,
		`data: {"type":"response.completed","response":{"id":"resp_outcome","model":"grok-4.5","status":"completed","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, _, resp, info := newChatStreamTestContext(strings.NewReader(body))
	info.RelayMode = relayconstant.RelayModeResponses
	info.RelayFormat = types.RelayFormatOpenAIResponses
	info.ChannelMeta.UpstreamModelName = "grok-4.5"
	info.AttemptOutcome = &relaycommon.AttemptRecorder{}
	require.NoError(t, info.AttemptOutcome.Begin("attempt-3", "candidate-3"))

	_, apiErr := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)

	outcome, err := info.AttemptOutcome.Snapshot()
	require.NoError(t, err)
	assert.True(t, outcome.ProtocolCompleted)
}

func TestOaiResponsesStreamHandlerMarksMissingTerminalAsNonReplayable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setChatStreamTestTimeout(t)

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_outcome_eof","model":"grok-4.5"}}`,
		`data: {"type":"response.output_text.delta","delta":"partial"}`,
		``,
	}, "\n")
	c, _, resp, info := newChatStreamTestContext(strings.NewReader(body))
	info.RelayMode = relayconstant.RelayModeResponses
	info.RelayFormat = types.RelayFormatOpenAIResponses
	info.ChannelMeta.UpstreamModelName = "grok-4.5"
	info.AttemptOutcome = &relaycommon.AttemptRecorder{}
	require.NoError(t, info.AttemptOutcome.Begin("attempt-4", "candidate-4"))

	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Greater(t, usage.TotalTokens, 0)

	outcome, err := info.AttemptOutcome.Snapshot()
	require.NoError(t, err)
	assert.False(t, outcome.ProtocolCompleted)
	assert.True(t, outcome.DownstreamCommitted)
	assert.False(t, outcome.CanRetry())
}

func TestOaiChatToResponsesStreamRequiresFinishReasonForCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setChatStreamTestTimeout(t)

	body := strings.Join([]string{
		`data: {"id":"chatcmpl-convert-truncated","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}`,
		``,
	}, "\n")
	c, _, resp, info := newChatStreamTestContext(strings.NewReader(body))
	info.AttemptOutcome = &relaycommon.AttemptRecorder{}
	require.NoError(t, info.AttemptOutcome.Begin("attempt-convert-truncated", "candidate-convert"))

	_, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)

	outcome, err := info.AttemptOutcome.Snapshot()
	require.NoError(t, err)
	assert.False(t, outcome.ProtocolCompleted, "EOF after partial output is not a protocol completion")
}

func TestOaiResponsesToChatStreamRequiresTerminalEventForCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setChatStreamTestTimeout(t)

	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"partial"}`,
		``,
	}, "\n")
	c, _, resp, info := newChatStreamTestContext(strings.NewReader(body))
	info.RelayMode = relayconstant.RelayModeResponses
	info.RelayFormat = types.RelayFormatOpenAI
	info.ChannelMeta.UpstreamModelName = "grok-4.5"
	info.AttemptOutcome = &relaycommon.AttemptRecorder{}
	require.NoError(t, info.AttemptOutcome.Begin("attempt-responses-truncated", "candidate-responses"))

	_, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)
	require.Nil(t, apiErr)

	outcome, err := info.AttemptOutcome.Snapshot()
	require.NoError(t, err)
	assert.False(t, outcome.ProtocolCompleted, "EOF without a terminal Responses event is not success")
}
