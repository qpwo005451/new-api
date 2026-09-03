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
			wantPath:  "/v1/responses",
		},
		{
			name:      "matches host case insensitively",
			info:      inputDeepSeekV4FlashInfo("https://AI.INPUT.IM"),
			model:     inputDeepSeekV4FlashModel,
			wantMatch: true,
			wantPath:  "/v1/responses",
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

func TestInputDeepSeekV4FlashResponsesRequestPassesThrough(t *testing.T) {
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
	responsesRequest, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok, "%T", converted)
	assert.Equal(t, inputDeepSeekV4FlashModel, responsesRequest.Model)
	assert.Equal(t, `"reply with OK"`, string(responsesRequest.Input))
	require.NotNil(t, responsesRequest.MaxOutputTokens)
	assert.Equal(t, uint(32), *responsesRequest.MaxOutputTokens)
}

func TestInputDeepSeekV4FlashResponsesRequestPassesThroughWithReasoning(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	info := inputDeepSeekV4FlashInfo("https://ai.input.im")
	request := dto.OpenAIResponsesRequest{
		Model: inputDeepSeekV4FlashModel,
		Input: []byte(`[
			{"type":"reasoning","summary":[{"type":"summary_text","text":"prior thought"}]},
			{"type":"message","role":"assistant","content":[{"type":"output_text","text":"prior answer"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]}
		]`),
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, info, request)
	require.NoError(t, err)
	responsesRequest, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok, "%T", converted)
	assert.Equal(t, inputDeepSeekV4FlashModel, responsesRequest.Model)
	assert.Contains(t, string(responsesRequest.Input), "prior thought")
	assert.Contains(t, string(responsesRequest.Input), "continue")
}

func TestInputDeepSeekV4FlashDoResponseKeepsResponsesProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	info := inputDeepSeekV4FlashInfo("https://ai.input.im")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(`{"id":"resp_input","object":"response","created_at":1710000000,"model":"deepseek-v4-flash","status":"completed","output":[{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`)),
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
			`data: {"type":"response.created","response":{"id":"resp_input","object":"response","model":"deepseek-v4-flash","status":"in_progress"}}`,
			`data: {"type":"response.output_text.delta","item_id":"msg_1","output_index":0,"content_index":0,"delta":"OK"}`,
			`data: {"type":"response.completed","response":{"id":"resp_input","object":"response","model":"deepseek-v4-flash","status":"completed","output":[{"id":"msg_1","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"OK"}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`,
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
