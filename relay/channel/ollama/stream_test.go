package ollama

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaChatHandlerNonStreamToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name   string
		raw    string
		wantID string
	}{
		{
			name:   "compact json per-line parse path",
			raw:    `{"model":"llama3.1","created_at":"2026-05-27T12:00:00Z","message":{"role":"assistant","content":"","tool_calls":[{"id":"call_upstream","function":{"name":"get_weather","arguments":{"city":"Paris","days":0}}}]},"done":true,"done_reason":"stop","prompt_eval_count":5,"eval_count":7}`,
			wantID: "call_upstream",
		},
		{
			name: "pretty json fallback parse path",
			raw: `{
  "model": "llama3.1",
  "created_at": "2026-05-27T12:00:00Z",
  "message": {
    "role": "assistant",
    "content": "",
    "tool_calls": [
      {
        "function": {
          "name": "get_weather",
          "arguments": {
            "city": "Paris",
            "days": 0
          }
        }
      }
    ]
  },
  "done": true,
  "done_reason": "stop",
  "prompt_eval_count": 5,
  "eval_count": 7
}`,
			wantID: "call_0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(tt.raw)),
			}

			usage, apiErr := ollamaChatHandler(c, &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "fallback-model"},
			}, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			assert.Equal(t, 12, usage.TotalTokens)

			var out dto.OpenAITextResponse
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &out))
			require.Len(t, out.Choices, 1)
			assert.Equal(t, constant.FinishReasonToolCalls, out.Choices[0].FinishReason)

			var toolCalls []dto.ToolCallResponse
			require.NoError(t, common.Unmarshal(out.Choices[0].Message.ToolCalls, &toolCalls))
			require.Len(t, toolCalls, 1)
			assert.Equal(t, tt.wantID, toolCalls[0].ID)
			assert.Equal(t, "function", toolCalls[0].Type)
			assert.Equal(t, "get_weather", toolCalls[0].Function.Name)
			assert.Nil(t, toolCalls[0].Index)

			var args map[string]any
			require.NoError(t, common.Unmarshal([]byte(toolCalls[0].Function.Arguments), &args))
			assert.Equal(t, "Paris", args["city"])
			assert.Equal(t, float64(0), args["days"])
		})
	}
}

func TestParseOllamaChatResponsePreservesLengthWithToolCalls(t *testing.T) {
	body := []byte(`{"model":"x","message":{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"f","arguments":{}}}]},"done":true,"done_reason":"length","prompt_eval_count":1,"eval_count":2}`)
	out, _, err := parseOllamaChatResponse(body, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "x"}})
	require.NoError(t, err)
	require.Len(t, out.Choices, 1)
	assert.Equal(t, "length", out.Choices[0].FinishReason)
}

func TestParseOllamaChatResponseDoesNotDuplicateRepeatedToolCallFrames(t *testing.T) {
	body := []byte(`{"model":"x","message":{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"f","arguments":{"a":1}}}]}}
{"model":"x","message":{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"f","arguments":{"a":1}}}]}}
{"model":"x","done":true,"done_reason":"stop"}`)
	out, _, err := parseOllamaChatResponse(body, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "x"}})
	require.NoError(t, err)
	var calls []dto.ToolCallResponse
	require.NoError(t, common.Unmarshal(out.Choices[0].Message.ToolCalls, &calls))
	require.Len(t, calls, 1)
}

func TestOllamaStreamHandlerRejectsExcessiveToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	items := make([]string, maxOllamaToolCalls+1)
	for i := range items {
		items[i] = `{"id":"c` + fmt.Sprint(i) + `","function":{"name":"f","arguments":{}}}`
	}
	body := `{"model":"x","message":{"role":"assistant","tool_calls":[` + strings.Join(items, ",") + `]}}` + "\n" + `{"model":"x","done":true,"done_reason":"stop"}`
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	_, apiErr := ollamaStreamHandler(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "x"}}, resp)
	require.NotNil(t, apiErr)
	assert.Contains(t, apiErr.Error(), "too many tool calls")
}

func TestOllamaStreamHandlerPreservesLengthWithToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := `{"model":"x","message":{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"f","arguments":{}}}]}}
{"model":"x","done":true,"done_reason":"length","prompt_eval_count":1,"eval_count":2}
`
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	_, apiErr := ollamaStreamHandler(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "x"}}, resp)
	require.Nil(t, apiErr)
	assert.Contains(t, w.Body.String(), `"finish_reason":"length"`)
	assert.NotContains(t, w.Body.String(), `"finish_reason":"tool_calls"`)
}

func TestParseOllamaChatResponseRejectsExcessiveToolCalls(t *testing.T) {
	calls := make([]string, maxOllamaToolCalls+1)
	for i := range calls {
		calls[i] = `{"id":"c` + fmt.Sprint(i) + `","function":{"name":"f","arguments":{}}}`
	}
	body := []byte(`{"model":"x","message":{"role":"assistant","tool_calls":[` + strings.Join(calls, ",") + `]}}
{"model":"x","done":true,"done_reason":"stop"}`)
	_, _, err := parseOllamaChatResponse(body, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "x"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many tool calls")
}

func TestOllamaStreamToolCallBudgetAcrossFrames(t *testing.T) {
	seen := make(map[string]struct{})
	var chunk ollamaChatStreamChunk
	require.NoError(t, common.Unmarshal([]byte(`{"message":{"tool_calls":[{"function":{"name":"f","arguments":{}}}]}}`), &chunk))
	index := 0
	for i := 0; i < 128; i++ {
		delta, next, err := buildOllamaChatStreamDelta(chunk, "x", "response", 1, index, seen)
		require.NoError(t, err)
		require.Len(t, delta.Choices[0].Delta.ToolCalls, 1)
		assert.Equal(t, i+1, next)
		index = next
	}
	delta, next, err := buildOllamaChatStreamDelta(chunk, "x", "response", 1, index, seen)
	require.ErrorContains(t, err, "too many tool calls")
	assert.Equal(t, index, next)
	assert.Empty(t, delta.Choices[0].Delta.ToolCalls)
}
