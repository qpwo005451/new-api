package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func inputDeepSeekV4FlashInfo(baseURL string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeResponses,
		RelayFormat:    types.RelayFormatOpenAIResponses,
		RequestURLPath: "/v1/responses",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelBaseUrl:    baseURL,
			UpstreamModelName: inputDeepSeekV4FlashModel,
		},
	}
}

func TestInputDeepSeekV4FlashResponsesAdapterPredicate(t *testing.T) {
	tests := []struct {
		name      string
		info      *relaycommon.RelayInfo
		model     string
		wantMatch bool
		wantPath  string
	}{
		{
			name:      "matches Input Responses request",
			info:      inputDeepSeekV4FlashInfo("https://ai.input.im"),
			model:     inputDeepSeekV4FlashModel,
			wantMatch: true,
			wantPath:  "/v1/chat/completions",
		},
		{
			name:      "matches host case insensitively",
			info:      inputDeepSeekV4FlashInfo("https://AI.INPUT.IM"),
			model:     inputDeepSeekV4FlashModel,
			wantMatch: true,
			wantPath:  "/v1/chat/completions",
		},
		{
			name: "does not match another model",
			info: func() *relaycommon.RelayInfo {
				info := inputDeepSeekV4FlashInfo("https://ai.input.im")
				info.ChannelMeta.UpstreamModelName = "deepseek-v4-pro"
				return info
			}(),
			model:     "deepseek-v4-pro",
			wantMatch: false,
			wantPath:  "/v1/responses",
		},
		{
			name:      "does not match another host",
			info:      inputDeepSeekV4FlashInfo("https://api.example.com"),
			model:     inputDeepSeekV4FlashModel,
			wantMatch: false,
			wantPath:  "/v1/responses",
		},
		{
			name: "does not match non OpenAI channel type",
			info: func() *relaycommon.RelayInfo {
				info := inputDeepSeekV4FlashInfo("https://ai.input.im")
				info.ChannelMeta.ChannelType = constant.ChannelTypeAILS
				return info
			}(),
			model:     inputDeepSeekV4FlashModel,
			wantMatch: false,
			wantPath:  "/v1/responses",
		},
		{
			name: "does not match Responses compact",
			info: func() *relaycommon.RelayInfo {
				info := inputDeepSeekV4FlashInfo("https://ai.input.im")
				info.RelayMode = relayconstant.RelayModeResponsesCompact
				return info
			}(),
			model:     inputDeepSeekV4FlashModel,
			wantMatch: false,
			wantPath:  "/v1/responses",
		},
		{
			name: "does not match channel body passthrough",
			info: func() *relaycommon.RelayInfo {
				info := inputDeepSeekV4FlashInfo("https://ai.input.im")
				info.ChannelSetting.PassThroughBodyEnabled = true
				return info
			}(),
			model:     inputDeepSeekV4FlashModel,
			wantMatch: false,
			wantPath:  "/v1/responses",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantMatch, isInputDeepSeekV4FlashResponsesRequest(tt.info, tt.model))
			requestURL, err := (&Adaptor{}).GetRequestURL(tt.info)
			require.NoError(t, err)
			require.True(t, strings.HasSuffix(requestURL, tt.wantPath), requestURL)
		})
	}
}

func TestInputDeepSeekV4FlashResponsesRequestConvertsToChat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	info := inputDeepSeekV4FlashInfo("https://ai.input.im")
	maxOutputTokens := uint(32)
	request := dto.OpenAIResponsesRequest{
		Model:           inputDeepSeekV4FlashModel,
		Input:           []byte("\"reply with OK\""),
		MaxOutputTokens: &maxOutputTokens,
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, info, request)
	require.NoError(t, err)
	chatRequest, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok, "%T", converted)
	info.AppendRequestConversion(types.RelayFormatOpenAI)
	require.Equal(t, types.RelayFormatOpenAI, info.GetFinalRequestRelayFormat())
	require.Len(t, chatRequest.Messages, 1)
	assert.Equal(t, inputDeepSeekV4FlashModel, chatRequest.Model)
	assert.Equal(t, "user", chatRequest.Messages[0].Role)
	assert.Equal(t, "reply with OK", chatRequest.Messages[0].StringContent())
	require.NotNil(t, chatRequest.MaxCompletionTokens)
	assert.Equal(t, uint(32), *chatRequest.MaxCompletionTokens)
}

func TestInputDeepSeekV4FlashDoResponseKeepsResponsesProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	info := inputDeepSeekV4FlashInfo("https://ai.input.im")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("{\"id\":\"chatcmpl_input\",\"object\":\"chat.completion\",\"created\":1710000000,\"model\":\"deepseek-v4-flash\",\"choices\":[{\"index\":0,\"message\":{\"role\":\"assistant\",\"content\":\"OK\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}")),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}

	usage, err := (&Adaptor{}).DoResponse(c, resp, info)
	require.Nil(t, err)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.(*dto.Usage).TotalTokens)
	body := recorder.Body.String()
	assert.Contains(t, body, "\"object\":\"response\"")
	assert.Contains(t, body, "\"output_text\"")
	assert.Contains(t, body, "\"OK\"")
	assert.NotContains(t, body, "\"choices\"")
}
func TestInputDeepSeekV4FlashDoResponseStreamKeepsResponsesProtocol(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	info := inputDeepSeekV4FlashInfo("https://ai.input.im")
	info.IsStream = true
	info.DisablePing = true
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"id":"chatcmpl_input","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl_input","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"OK"},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl_input","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
			`data: [DONE]`,
			``,
		}, "\n"))),
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	usage, err := (&Adaptor{}).DoResponse(c, resp, info)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 3, usage.(*dto.Usage).TotalTokens)
	body := recorder.Body.String()
	require.Contains(t, body, "event: response.created")
	require.Contains(t, body, "event: response.output_text.delta")
	require.Contains(t, body, "\"delta\":\"OK\"")
	require.Contains(t, body, "event: response.completed")
	require.Contains(t, body, "\"input_tokens\":2")
	require.NotContains(t, body, "chat.completion.chunk")
}
