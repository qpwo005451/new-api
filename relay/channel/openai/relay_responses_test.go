package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOaiResponsesStreamHandlerConvertsCompleteTextToolCall(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	tools, err := common.Marshal([]map[string]any{{"type": "function", "name": "shell_command"}})
	require.NoError(t, err)

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"nvidia/deepseek-v4-flash"}}`,
		`data: {"type":"response.output_text.delta","delta":"<TOOLCALL>[{\"name\":\"shell_command\",\"arguments\":{\"command\":\"echo ok\"}}]</TOOLCALL>"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"nvidia/deepseek-v4-flash","status":"completed"}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "responses-toolcall-test")
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "nvidia/deepseek-v4-flash"}, Request: &dto.OpenAIResponsesRequest{Tools: tools}, IsStream: true, RelayFormat: types.RelayFormatOpenAI, DisablePing: true}

	usage, streamErr := OaiResponsesStreamHandler(c, info, resp)
	require.NotNil(t, usage)
	require.Nil(t, streamErr)

	got := recorder.Body.String()
	require.NotContains(t, got, "<TOOLCALL>")
	require.Contains(t, got, `event: response.output_item.added`)
	require.Contains(t, got, `"type":"function_call"`)
	require.Contains(t, got, `"name":"shell_command"`)
	require.Contains(t, got, `event: response.function_call_arguments.delta`)
	require.Contains(t, got, `"delta":"{\"command\":\"echo ok\"}"`)
	require.Contains(t, got, `event: response.function_call_arguments.done`)
	require.Contains(t, got, `event: response.output_item.done`)
}

func TestOaiResponsesStreamHandlerReturnsUpstreamStreamError(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"type":"error","code":null,"message":"The model is currently at capacity","param":null}`,
		`data: {"type":"response.failed","response":{"id":"resp_1","model":"grok-4.5","status":"failed","error":{"code":"upstream_error","message":"Upstream request failed"}}}`,
		``,
	}, "\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "responses-failed-test")
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-4.5"}, IsStream: true, RelayFormat: types.RelayFormatOpenAI, DisablePing: true}

	usage, streamErr := OaiResponsesStreamHandler(c, info, resp)
	require.NotNil(t, usage)
	require.Error(t, streamErr)
	require.Contains(t, streamErr.Error(), "currently at capacity")
	require.True(t, types.IsSkipRetryError(streamErr))
	require.NotNil(t, info.StreamStatus)
	require.True(t, info.StreamStatus.HasErrors())

	got := recorder.Body.String()
	require.Contains(t, got, `event: error`)
	require.Contains(t, got, `event: response.failed`)
}

func TestOaiResponsesStreamHandlerRejectsEOFBeforeTerminalEvent(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"grok-4.5"}}`,
		``,
	}, "\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "responses-eof-test")
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-4.5"}, IsStream: true, RelayFormat: types.RelayFormatOpenAI, DisablePing: true}

	usage, streamErr := OaiResponsesStreamHandler(c, info, resp)
	require.NotNil(t, usage)
	require.Error(t, streamErr)
	require.Contains(t, streamErr.Error(), "ended before response.completed")
	require.True(t, types.IsSkipRetryError(streamErr))
	require.NotNil(t, info.StreamStatus)
	require.True(t, info.StreamStatus.HasErrors())
}

func TestOaiResponsesStreamHandlerAcceptsIncompleteTerminalEvent(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"grok-4.5"}}`,
		`data: {"type":"response.incomplete","response":{"id":"resp_1","model":"grok-4.5","status":"incomplete","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`,
		``,
	}, "\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "responses-incomplete-test")
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-4.5"}, IsStream: true, RelayFormat: types.RelayFormatOpenAI, DisablePing: true}

	usage, streamErr := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, streamErr)
	require.Equal(t, 5, usage.TotalTokens)
	require.Contains(t, recorder.Body.String(), `event: response.incomplete`)
}

func TestOaiResponsesStreamHandlerNormalizesGrok45ShellCommandTimeout(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	tools := grok45ShellCommandTools(t)
	body := strings.Join([]string{
		`data: {"type":"response.output_item.added","output_index":0,"item_id":"fc_1","item":{"type":"function_call","id":"fc_1","status":"in_progress","call_id":"call_1","name":"shell_command","arguments":""}}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"{\"command\":\"echo ok\",\"timeout_ms\":60"}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"item_id":"fc_1","delta":"000.0}"}`,
		`data: {"type":"response.function_call_arguments.done","output_index":0,"item_id":"fc_1","arguments":"{\"command\":\"echo ok\",\"timeout_ms\":60000.0}"}`,
		`data: {"type":"response.output_item.done","output_index":0,"item_id":"fc_1","item":{"type":"function_call","id":"fc_1","status":"completed","call_id":"call_1","name":"shell_command","arguments":"{\"command\":\"echo ok\",\"timeout_ms\":60000.0}"}}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"grok-4.5","status":"completed"}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "responses-grok45-timeout-test")
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"text/event-stream"}}}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: grok45ModelName},
		Request:     &dto.OpenAIResponsesRequest{Tools: tools},
		IsStream:    true,
		RelayFormat: types.RelayFormatOpenAI,
		DisablePing: true,
	}

	usage, streamErr := OaiResponsesStreamHandler(c, info, resp)
	require.NotNil(t, usage)
	require.Nil(t, streamErr)

	got := recorder.Body.String()
	require.NotContains(t, got, "60000.0")
	require.Contains(t, got, `"delta":"{\"command\":\"echo ok\",\"timeout_ms\":60000}"`)
	require.Equal(t, 1, strings.Count(got, "event: response.function_call_arguments.delta"))
	require.Contains(t, got, `event: response.function_call_arguments.done`)
	require.Contains(t, got, `event: response.output_item.done`)
}

func TestOaiResponsesHandlerNormalizesGrok45ShellCommandTimeout(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	body := `{"id":"resp_1","model":"grok-4.5","output":[{"type":"function_call","id":"fc_1","status":"completed","call_id":"call_1","name":"shell_command","arguments":"{\"command\":\"echo ok\",\"timeout_ms\":60000.0}"}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5,"input_tokens_details":{"cache_write_tokens":2}}}`
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: grok45ModelName},
		Request:     &dto.OpenAIResponsesRequest{Tools: grok45ShellCommandTools(t)},
		RelayFormat: types.RelayFormatOpenAI,
	}

	usage, responseErr := OaiResponsesHandler(c, info, resp)
	require.NotNil(t, usage)
	require.Nil(t, responseErr)
	require.Equal(t, 5, usage.TotalTokens)
	require.Equal(t, 2, usage.PromptTokensDetails.CacheWriteTokens)
	require.NotContains(t, recorder.Body.String(), "60000.0")
	require.Contains(t, recorder.Body.String(), `\"timeout_ms\":60000`)
}

func TestNormalizeGrok45ShellCommandTimeoutRejectsInvalidValues(t *testing.T) {
	timeoutSchema := gjson.Parse(`{"type":"integer","minimum":1,"maximum":600000}`)
	tests := []struct {
		name      string
		arguments string
	}{
		{name: "fractional", arguments: `{"timeout_ms":60000.5}`},
		{name: "out of range", arguments: `{"timeout_ms":600001.0}`},
		{name: "u64 overflow", arguments: `{"timeout_ms":18446744073709551616.0}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := normalizeGrok45ShellCommandTimeout([]byte(tt.arguments), timeoutSchema)
			require.False(t, changed)
			require.Equal(t, tt.arguments, string(got))
		})
	}
}

func TestGrok45ShellCommandTimeoutSchemaIsModelScoped(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-4.4"},
		Request:     &dto.OpenAIResponsesRequest{Tools: grok45ShellCommandTools(t)},
	}

	_, ok := grok45ShellCommandTimeoutSchema(info)
	require.False(t, ok)
}

func TestGrok45ShellCommandTimeoutSchemaAcceptsNumberSchema(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: grok45ModelName},
		Request:     &dto.OpenAIResponsesRequest{Tools: grok45ShellCommandToolsWithTimeoutType(t, "number")},
	}

	_, ok := grok45ShellCommandTimeoutSchema(info)
	require.True(t, ok)
}

func grok45ShellCommandTools(t *testing.T) []byte {
	return grok45ShellCommandToolsWithTimeoutType(t, "integer")
}

func grok45ShellCommandToolsWithTimeoutType(t *testing.T, timeoutType string) []byte {
	t.Helper()
	tools, err := common.Marshal([]map[string]any{
		{
			"type": "function",
			"name": grok45ShellCommandToolName,
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{"type": "string"},
					"timeout_ms": map[string]any{
						"type":    timeoutType,
						"minimum": 1,
						"maximum": 600000,
					},
				},
			},
		},
	})
	require.NoError(t, err)
	return tools
}
