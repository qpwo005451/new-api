package ollama

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

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
	out, _, _, err := parseOllamaChatResponse(body, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "x"}})
	require.NoError(t, err)
	require.Len(t, out.Choices, 1)
	assert.Equal(t, "length", out.Choices[0].FinishReason)
}

func TestParseOllamaChatResponseDoesNotDuplicateRepeatedToolCallFrames(t *testing.T) {
	body := []byte(`{"model":"x","message":{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"f","arguments":{"a":1}}}]}}
{"model":"x","message":{"role":"assistant","tool_calls":[{"id":"c1","function":{"name":"f","arguments":{"a":1}}}]}}
{"model":"x","done":true,"done_reason":"stop"}`)
	out, _, _, err := parseOllamaChatResponse(body, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "x"}})
	require.NoError(t, err)
	var calls []dto.ToolCallResponse
	require.NoError(t, common.Unmarshal(out.Choices[0].Message.ToolCalls, &calls))
	require.Len(t, calls, 1)
}

func TestParseOllamaChatResponseDedupesNoIDToolCallFrames(t *testing.T) {
	body := []byte(`{"model":"x","message":{"role":"assistant","tool_calls":[{"function":{"name":"f","arguments":{"a":1}}}]}}
{"model":"x","message":{"role":"assistant","tool_calls":[{"function":{"name":"f","arguments":{"a":1}}}]}}
{"model":"x","message":{"role":"assistant","tool_calls":[{"function":{"name":"f","arguments":{"a":1}}}]}}
{"model":"x","done":true,"done_reason":"stop"}`)
	out, _, truncated, err := parseOllamaChatResponse(body, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "x"}})
	require.NoError(t, err)
	assert.False(t, truncated)
	var calls []dto.ToolCallResponse
	require.NoError(t, common.Unmarshal(out.Choices[0].Message.ToolCalls, &calls))
	require.Len(t, calls, 1)
	assert.Equal(t, "call_0", calls[0].ID)
}

func TestParseOllamaChatResponseTruncatesExcessiveToolCalls(t *testing.T) {
	calls := make([]string, maxOllamaToolCalls+5)
	for i := range calls {
		calls[i] = `{"function":{"name":"f","arguments":{"i":` + fmt.Sprint(i) + `}}}`
	}
	body := []byte(`{"model":"x","message":{"role":"assistant","tool_calls":[` + strings.Join(calls, ",") + `]}}` + "\n" + `{"model":"x","done":true,"done_reason":"stop","prompt_eval_count":11,"eval_count":13}`)
	out, usage, truncated, err := parseOllamaChatResponse(body, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "x"}})
	require.NoError(t, err)
	assert.True(t, truncated)
	require.NotNil(t, usage)
	var converted []dto.ToolCallResponse
	require.NoError(t, common.Unmarshal(out.Choices[0].Message.ToolCalls, &converted))
	assert.Len(t, converted, maxOllamaToolCalls)
	assert.Equal(t, constant.FinishReasonToolCalls, out.Choices[0].FinishReason)
}

func TestOllamaStreamHandlerTruncatesExcessiveToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	items := make([]string, maxOllamaToolCalls+5)
	for i := range items {
		items[i] = `{"function":{"name":"f","arguments":{"i":` + fmt.Sprint(i) + `}}}`
	}
	body := `{"model":"x","message":{"role":"assistant","tool_calls":[` + strings.Join(items, ",") + `]}}` + "\n" + `{"model":"x","done":true,"done_reason":"stop","prompt_eval_count":11,"eval_count":13}`
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	usage, apiErr := ollamaStreamHandler(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "x"}}, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 11, usage.PromptTokens)
	assert.Equal(t, 13, usage.CompletionTokens)
	out := w.Body.String()
	assert.Equal(t, maxOllamaToolCalls, strings.Count(out, `"name":"f"`))
	assert.Equal(t, 1, strings.Count(out, `"finish_reason":"tool_calls"`))
	assert.Contains(t, out, "[DONE]")
}

func TestOllamaStreamHandlerDedupesRepeatedNoIDToolCallFrames(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	items := make([]string, 130)
	for i := range items {
		items[i] = `{"function":{"name":"f","arguments":{"a":1}}}`
	}
	body := `{"model":"x","message":{"role":"assistant","tool_calls":[` + strings.Join(items, ",") + `]}}` + "\n" + `{"model":"x","done":true,"done_reason":"stop","prompt_eval_count":3,"eval_count":5}`
	resp := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
	usage, apiErr := ollamaStreamHandler(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "x"}}, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.PromptTokens)
	assert.Equal(t, 5, usage.CompletionTokens)
	out := w.Body.String()
	assert.Equal(t, 1, strings.Count(out, `"name":"f"`))
	assert.Contains(t, out, `"finish_reason":"tool_calls"`)
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

func TestOllamaStreamHandlerIdleTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 1
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	done := make(chan *types.NewAPIError, 1)
	go func() {
		_, apiErr := ollamaStreamHandler(c, &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "x"},
		}, &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: pr})
		done <- apiErr
	}()

	select {
	case apiErr := <-done:
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusGatewayTimeout, apiErr.StatusCode)
		assert.True(t, errors.Is(apiErr, helper.ErrStreamIdleTimeout))
	case <-time.After(3 * time.Second):
		t.Fatal("ollama stream handler did not return after idle timeout")
	}
	assert.NotContains(t, w.Body.String(), "[DONE]")
}

func TestBuildOllamaChatStreamDeltaDedupesNoIDRepeatedCalls(t *testing.T) {
	seen := make(map[string]struct{})
	var chunk ollamaChatStreamChunk
	require.NoError(t, common.Unmarshal([]byte(`{"message":{"tool_calls":[{"function":{"name":"f","arguments":{"a":1}}}]}}`), &chunk))
	delta, next, truncated := buildOllamaChatStreamDelta(chunk, "x", "response", 1, 0, seen)
	require.False(t, truncated)
	require.Len(t, delta.Choices[0].Delta.ToolCalls, 1)
	assert.Equal(t, 1, next)

	// Cumulative upstream frames repeat the same call without an id and must collapse.
	delta, next, truncated = buildOllamaChatStreamDelta(chunk, "x", "response", 1, next, seen)
	require.False(t, truncated)
	assert.Empty(t, delta.Choices[0].Delta.ToolCalls)
	assert.Equal(t, 1, next)
}

func TestBuildOllamaChatStreamDeltaKeepsDistinctNoIDCalls(t *testing.T) {
	seen := make(map[string]struct{})
	var first, second ollamaChatStreamChunk
	require.NoError(t, common.Unmarshal([]byte(`{"message":{"tool_calls":[{"function":{"name":"f","arguments":{"a":1}}}]}}`), &first))
	require.NoError(t, common.Unmarshal([]byte(`{"message":{"tool_calls":[{"function":{"name":"f","arguments":{"a":2}}}]}}`), &second))
	delta, next, truncated := buildOllamaChatStreamDelta(first, "x", "response", 1, 0, seen)
	require.False(t, truncated)
	require.Len(t, delta.Choices[0].Delta.ToolCalls, 1)
	delta, next, truncated = buildOllamaChatStreamDelta(second, "x", "response", 1, next, seen)
	require.False(t, truncated)
	require.Len(t, delta.Choices[0].Delta.ToolCalls, 1)
	assert.Equal(t, 2, next)
}

func TestBuildOllamaChatStreamDeltaTruncatesAtLimit(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < maxOllamaToolCalls; i++ {
		var chunk ollamaChatStreamChunk
		require.NoError(t, common.Unmarshal([]byte(`{"message":{"tool_calls":[{"function":{"name":"f","arguments":{"i":`+fmt.Sprint(i)+`}}}]}}`), &chunk))
		delta, next, truncated := buildOllamaChatStreamDelta(chunk, "x", "response", 1, i, seen)
		require.False(t, truncated)
		require.Len(t, delta.Choices[0].Delta.ToolCalls, 1)
		assert.Equal(t, i+1, next)
	}
	var chunk ollamaChatStreamChunk
	require.NoError(t, common.Unmarshal([]byte(`{"message":{"tool_calls":[{"function":{"name":"f","arguments":{"i":`+fmt.Sprint(maxOllamaToolCalls)+`}}}]}}`), &chunk))
	delta, next, truncated := buildOllamaChatStreamDelta(chunk, "x", "response", 1, maxOllamaToolCalls, seen)
	require.True(t, truncated)
	assert.Empty(t, delta.Choices[0].Delta.ToolCalls)
	assert.Equal(t, maxOllamaToolCalls, next)
}

func TestBuildOllamaStreamDelta(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		startIndex    int
		wantContent   string
		wantReasoning string
		wantToolCalls []struct {
			id        string
			name      string
			index     int
			arguments string
		}
		wantNextIndex int
		wantPayload   bool
	}{
		{
			name:        "chat content",
			raw:         `{"message":{"content":"hello"}}`,
			wantContent: "hello",
			wantPayload: true,
		},
		{
			name:        "generate content",
			raw:         `{"response":"hello"}`,
			wantContent: "hello",
			wantPayload: true,
		},
		{
			name:          "thinking json string",
			raw:           `{"message":{"thinking":"consider this"}}`,
			wantReasoning: "consider this",
			wantPayload:   true,
		},
		{
			name:          "thinking raw fallback",
			raw:           `{"message":{"thinking":{"step":"consider this"}}}`,
			wantReasoning: `{"step":"consider this"}`,
			wantPayload:   true,
		},
		{
			name:       "multiple tool calls",
			raw:        `{"message":{"tool_calls":[{"id":"call_upstream","function":{"name":"get_weather","arguments":{"city":"Paris"}}},{"function":{"name":"get_time","arguments":{"timezone":"UTC"}}}]}}`,
			startIndex: 2,
			wantToolCalls: []struct {
				id        string
				name      string
				index     int
				arguments string
			}{
				{id: "call_upstream", name: "get_weather", index: 2, arguments: `{"city":"Paris"}`},
				{id: "call_3", name: "get_time", index: 3, arguments: `{"timezone":"UTC"}`},
			},
			wantNextIndex: 4,
			wantPayload:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var chunk ollamaChatStreamChunk
			require.NoError(t, common.Unmarshal([]byte(tt.raw), &chunk))

			toolCallIndex := tt.startIndex
			delta, hasPayload := buildOllamaStreamDelta(&chunk, "response-id", 123, "model", &toolCallIndex)
			assert.Equal(t, tt.wantPayload, hasPayload)
			assert.Equal(t, tt.wantContent, delta.Choices[0].Delta.GetContentString())
			assert.Equal(t, tt.wantReasoning, delta.Choices[0].Delta.GetReasoningContent())
			assert.Equal(t, tt.wantNextIndex, toolCallIndex)

			if tt.wantToolCalls == nil {
				assert.Empty(t, delta.Choices[0].Delta.ToolCalls)
				return
			}
			require.Len(t, delta.Choices[0].Delta.ToolCalls, len(tt.wantToolCalls))
			for i, want := range tt.wantToolCalls {
				got := delta.Choices[0].Delta.ToolCalls[i]
				assert.Equal(t, want.id, got.ID)
				assert.Equal(t, "function", got.Type)
				assert.Equal(t, want.name, got.Function.Name)
				assert.Equal(t, want.arguments, got.Function.Arguments)
				require.NotNil(t, got.Index)
				assert.Equal(t, want.index, *got.Index)
			}
		})
	}
}

func TestOllamaStreamHandlerDoneToolCalls(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"model":"qwen3-coder","created_at":"2026-05-27T12:00:00Z","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"get_weather","arguments":{"city":"北京"}}},{"function":{"name":"get_time","arguments":{"timezone":"Asia/Shanghai"}}}]},"done":true,"done_reason":"stop","prompt_eval_count":5,"eval_count":7}`)),
	}

	usage, apiErr := ollamaStreamHandler(c, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "fallback-model"},
	}, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 12, usage.TotalTokens)

	var chunks []dto.ChatCompletionsStreamResponse
	for _, line := range strings.Split(w.Body.String(), "\n") {
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok || data == "[DONE]" {
			continue
		}
		var chunk dto.ChatCompletionsStreamResponse
		require.NoError(t, common.Unmarshal([]byte(data), &chunk))
		chunks = append(chunks, chunk)
	}

	var toolChunkIndex, stopChunkIndex int = -1, -1
	for i := range chunks {
		if len(chunks[i].Choices) == 0 {
			continue
		}
		if len(chunks[i].Choices[0].Delta.ToolCalls) > 0 {
			toolChunkIndex = i
		}
		if chunks[i].Choices[0].FinishReason != nil {
			stopChunkIndex = i
		}
	}
	require.NotEqual(t, -1, toolChunkIndex)
	require.NotEqual(t, -1, stopChunkIndex)
	assert.Less(t, toolChunkIndex, stopChunkIndex)

	toolCalls := chunks[toolChunkIndex].Choices[0].Delta.ToolCalls
	require.Len(t, toolCalls, 2)
	assert.Equal(t, "call_0", toolCalls[0].ID)
	assert.Equal(t, "call_1", toolCalls[1].ID)
	require.NotNil(t, toolCalls[0].Index)
	require.NotNil(t, toolCalls[1].Index)
	assert.Equal(t, 0, *toolCalls[0].Index)
	assert.Equal(t, 1, *toolCalls[1].Index)
	assert.Equal(t, constant.FinishReasonToolCalls, *chunks[stopChunkIndex].Choices[0].FinishReason)
	assert.Contains(t, w.Body.String(), "data: [DONE]")
}
