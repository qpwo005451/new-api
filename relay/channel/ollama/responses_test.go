package ollama

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func responsesRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeResponses,
		RelayFormat:    types.RelayFormatOpenAIResponses,
		RequestURLPath: "/v1/responses",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "glm-5.3-flash",
		},
	}
}

func TestOllamaConvertOpenAIResponsesRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		request     dto.OpenAIResponsesRequest
		wantModel   string
		wantStream  bool
		wantThink   string
		wantContent string
	}{
		{
			name: "instructions plus string input maps to system and user messages",
			request: dto.OpenAIResponsesRequest{
				Model:        "glm-5.3-flash",
				Instructions: json.RawMessage(`"system rules"`),
				Input:        json.RawMessage(`"hello"`),
			},
			wantModel:   "glm-5.3-flash",
			wantStream:  false,
			wantContent: "hello",
		},
		{
			name: "stream flag maps to ollama stream",
			request: dto.OpenAIResponsesRequest{
				Model:  "glm-5.3-flash",
				Input:  json.RawMessage(`"hello"`),
				Stream: lo.ToPtr(true),
			},
			wantModel:   "glm-5.3-flash",
			wantStream:  true,
			wantContent: "hello",
		},
		{
			name: "reasoning effort maps to think option",
			request: dto.OpenAIResponsesRequest{
				Model:     "glm-5.3-flash",
				Input:     json.RawMessage(`"hello"`),
				Reasoning: &dto.Reasoning{Effort: "medium"},
			},
			wantModel:   "glm-5.3-flash",
			wantStream:  false,
			wantThink:   "medium",
			wantContent: "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

			info := responsesRelayInfo()
			converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, info, tt.request)
			require.NoError(t, err)

			chatReq, ok := converted.(*OllamaChatRequest)
			require.True(t, ok)
			assert.Equal(t, tt.wantModel, chatReq.Model)
			assert.Equal(t, tt.wantStream, chatReq.Stream)
			if tt.wantThink == "" {
				assert.Nil(t, chatReq.Think)
			} else {
				require.NotNil(t, chatReq.Think)
				assert.Equal(t, tt.wantThink, strings.Trim(string(chatReq.Think), `"`))
			}
			require.NotEmpty(t, chatReq.Messages)
			last := chatReq.Messages[len(chatReq.Messages)-1]
			assert.Equal(t, "user", last.Role)
			assert.Equal(t, tt.wantContent, last.Content)
		})
	}
}

func TestOllamaConvertOpenAIResponsesRequestRejectsStatefulFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	_, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, responsesRelayInfo(), dto.OpenAIResponsesRequest{
		Model:              "glm-5.3-flash",
		Input:              json.RawMessage(`"hello"`),
		PreviousResponseID: "resp_prev",
	})
	require.Error(t, err)
}

func TestOllamaResponsesHandlerNonStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`{"model":"glm-5.3-flash","created_at":"2026-08-31T06:00:00Z","message":{"role":"assistant","content":"Hel"},"done":false}`,
			`{"model":"glm-5.3-flash","created_at":"2026-08-31T06:00:01Z","message":{"role":"assistant","content":"lo"},"done":true,"done_reason":"stop","prompt_eval_count":2,"eval_count":1}`,
		}, "\n"))),
	}

	usage, apiErr := ollamaResponsesHandler(c, responsesRelayInfo(), resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.TotalTokens)

	var out dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &out))
	assert.Equal(t, "response", out.Object)
	assert.Equal(t, "glm-5.3-flash", out.Model)
	require.NotEmpty(t, out.Output)
	assert.Equal(t, "message", out.Output[0].Type)
	require.NotEmpty(t, out.Output[0].Content)
	assert.Equal(t, "Hello", out.Output[0].Content[0].Text)
	require.NotNil(t, out.Usage)
	assert.Equal(t, 2, out.Usage.PromptTokens)
	assert.Equal(t, 1, out.Usage.CompletionTokens)
}

func TestOllamaResponsesStreamHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	info := responsesRelayInfo()
	info.IsStream = true
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`{"model":"glm-5.3-flash","created_at":"2026-08-31T06:00:00Z","message":{"role":"assistant","content":"OK"},"done":false}`,
			`{"model":"glm-5.3-flash","created_at":"2026-08-31T06:00:01Z","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":2,"eval_count":1}`,
		}, "\n"))),
	}

	usage, apiErr := ollamaResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.TotalTokens)

	body := recorder.Body.String()
	require.Contains(t, body, "event: response.created")
	require.Contains(t, body, "event: response.output_text.delta")
	require.Contains(t, body, `"delta":"OK"`)
	require.Contains(t, body, "event: response.completed")
	require.Contains(t, body, `"input_tokens":2`)
	assert.NotContains(t, body, `"object":"chat.completion.chunk"`)
}

func TestOllamaResponsesStreamHandlerToolCallFinishReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	info := responsesRelayInfo()
	info.IsStream = true
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`{"model":"glm-5.3-flash","created_at":"2026-08-31T06:00:00Z","message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","function":{"name":"get_weather","arguments":{"city":"Paris"}}}]},"done":false}`,
			`{"model":"glm-5.3-flash","created_at":"2026-08-31T06:00:01Z","message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":2,"eval_count":1}`,
		}, "\n"))),
	}

	usage, apiErr := ollamaResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)

	body := recorder.Body.String()
	require.Contains(t, body, "event: response.output_item.added")
	require.Contains(t, body, "event: response.function_call_arguments.done")
	require.Contains(t, body, `"name":"get_weather"`)
	require.Contains(t, body, `"call_id":"call_1"`)
	require.Contains(t, body, "event: response.completed")
}