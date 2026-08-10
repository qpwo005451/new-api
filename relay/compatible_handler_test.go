package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	openairelay "github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type bufferedRetryTestAdaptor struct {
	responses []*http.Response
	calls     int
	bodies    []string
}

func (a *bufferedRetryTestAdaptor) DoRequest(_ *gin.Context, _ *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	body, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, err
	}
	a.bodies = append(a.bodies, string(body))
	resp := a.responses[a.calls]
	a.calls++
	return resp, nil
}

type bufferedRetryUnexpectedEOFReader struct {
	data []byte
}

func (r *bufferedRetryUnexpectedEOFReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, io.ErrUnexpectedEOF
	}
	return n, nil
}

func newBufferedRetryTestResponse(body string) *http.Response {
	return newBufferedRetryTestResponseBody(strings.NewReader(body))
}

func newBufferedRetryTestResponseBody(body io.Reader) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(body),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
}

func TestBufferedOpencodeStreamRetryDropsFirstPartialStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setBufferedRetryTestTimeout(t)

	c, recorder, info := newBufferedRetryTestContext("opencode-go", "deepseek-v4-flash", "https://api.opencode.ai")
	requestBody := `{"model":"deepseek-v4-flash"}`
	storage, err := common.CreateBodyStorage([]byte(requestBody))
	require.NoError(t, err)
	defer storage.Close()

	firstBody := strings.Join([]string{
		`data: {"id":"chatcmpl-first","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"reasoning_content":"first-partial"},"finish_reason":null}]}`,
		``,
	}, "\n")
	secondBody := strings.Join([]string{
		`data: {"id":"chatcmpl-second","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"second-ok"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-second","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	adaptor := &bufferedRetryTestAdaptor{
		responses: []*http.Response{
			newBufferedRetryTestResponse(secondBody),
		},
	}

	bufferedResp, newAPIError := doBufferedOpencodeStreamRetry(
		c,
		info,
		adaptor,
		storage,
		newBufferedRetryTestResponseBody(&bufferedRetryUnexpectedEOFReader{data: []byte(firstBody)}),
		"",
	)

	require.Nil(t, newAPIError)
	require.NotNil(t, bufferedResp)
	assert.Equal(t, 1, adaptor.calls)
	assert.Equal(t, []string{requestBody}, adaptor.bodies)

	usage, newAPIError := openairelay.OaiStreamHandler(c, info, bufferedResp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.TotalTokens)
	body := recorder.Body.String()
	assert.NotContains(t, body, "first-partial")
	assert.Contains(t, body, "second-ok")
	assert.Contains(t, body, "data: [DONE]")
}

func TestBufferedOpencodeStreamRetryDoesNotRetryFinishedStreamWithUnexpectedEOF(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _, info := newBufferedRetryTestContext("opencode-go", "deepseek-v4-flash", "https://api.opencode.ai")
	storage, err := common.CreateBodyStorage([]byte(`{"model":"deepseek-v4-flash"}`))
	require.NoError(t, err)
	defer storage.Close()

	completeWithoutDone := strings.Join([]string{
		`data: {"id":"chatcmpl-ok","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl-ok","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
		``,
	}, "\n")
	adaptor := &bufferedRetryTestAdaptor{
		responses: []*http.Response{
			newBufferedRetryTestResponse("should-not-be-used"),
		},
	}

	bufferedResp, newAPIError := doBufferedOpencodeStreamRetry(
		c,
		info,
		adaptor,
		storage,
		newBufferedRetryTestResponseBody(&bufferedRetryUnexpectedEOFReader{data: []byte(completeWithoutDone)}),
		"",
	)

	require.Nil(t, newAPIError)
	require.NotNil(t, bufferedResp)
	assert.Equal(t, 0, adaptor.calls)
	body, err := io.ReadAll(bufferedResp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"finish_reason":"stop"`)
}

func TestBufferedOpencodeStreamRetryNormalizesDoneWithoutFinishReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setBufferedRetryTestTimeout(t)

	c, recorder, info := newBufferedRetryTestContext("opencode-go", "deepseek-v4-flash", "https://api.opencode.ai")
	storage, err := common.CreateBodyStorage([]byte(`{"model":"deepseek-v4-flash"}`))
	require.NoError(t, err)
	defer storage.Close()

	completeWithDone := strings.Join([]string{
		`data: {"id":"chatcmpl-done","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":null}]}`,
		`data: [DONE]`,
		``,
	}, "\n")
	adaptor := &bufferedRetryTestAdaptor{
		responses: []*http.Response{
			newBufferedRetryTestResponse("should-not-be-used"),
		},
	}

	bufferedResp, newAPIError := doBufferedOpencodeStreamRetry(
		c,
		info,
		adaptor,
		storage,
		newBufferedRetryTestResponseBody(&bufferedRetryUnexpectedEOFReader{data: []byte(completeWithDone)}),
		"",
	)

	require.Nil(t, newAPIError)
	require.NotNil(t, bufferedResp)
	assert.Equal(t, 0, adaptor.calls)
	usage, newAPIError := openairelay.OaiStreamHandler(c, info, bufferedResp)
	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	assert.Contains(t, recorder.Body.String(), `"finish_reason":"stop"`)
	assert.Contains(t, recorder.Body.String(), "data: [DONE]")
	assert.NotContains(t, recorder.Body.String(), "upstream stream terminated unexpectedly")
}

func TestShouldUseBufferedOpencodeStreamRetryTargetsOnlyFlashOpenCodeStreams(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name      string
		stream    bool
		mode      int
		format    types.RelayFormat
		channel   string
		baseURL   string
		model     string
		upstream  string
		wantMatch bool
	}{
		{
			name:      "deepseek flash through opencode channel",
			stream:    true,
			mode:      relayconstant.RelayModeChatCompletions,
			format:    types.RelayFormatOpenAI,
			channel:   "provider-OpenCode-Go",
			model:     "deepseek-v4-flash",
			wantMatch: true,
		},
		{
			name:      "deepseek flash through opencode base url",
			stream:    true,
			mode:      relayconstant.RelayModeChatCompletions,
			format:    types.RelayFormatOpenAI,
			baseURL:   "https://gateway.example/opencode/v1",
			upstream:  "nvidia/deepseek-v4-flash",
			model:     "alias-model",
			wantMatch: true,
		},
		{
			name:      "deepseek pro is not buffered",
			stream:    true,
			mode:      relayconstant.RelayModeChatCompletions,
			format:    types.RelayFormatOpenAI,
			channel:   "opencode-go",
			model:     "deepseek-v4-pro",
			wantMatch: false,
		},
		{
			name:      "non opencode channel is not buffered",
			stream:    true,
			mode:      relayconstant.RelayModeChatCompletions,
			format:    types.RelayFormatOpenAI,
			channel:   "deepseek",
			model:     "deepseek-v4-flash",
			wantMatch: false,
		},
		{
			name:      "non stream request is not buffered",
			stream:    false,
			mode:      relayconstant.RelayModeChatCompletions,
			format:    types.RelayFormatOpenAI,
			channel:   "opencode-go",
			model:     "deepseek-v4-flash",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _, info := newBufferedRetryTestContext(tt.channel, tt.model, tt.baseURL)
			info.IsStream = tt.stream
			info.RelayMode = tt.mode
			info.RelayFormat = tt.format
			if tt.upstream != "" {
				info.UpstreamModelName = tt.upstream
			}

			assert.Equal(t, tt.wantMatch, shouldUseBufferedOpencodeStreamRetry(c, info))
		})
	}
}

func setBufferedRetryTestTimeout(t *testing.T) {
	t.Helper()
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})
}

func newBufferedRetryTestContext(channelName, modelName, baseURL string) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+modelName+`"}`))
	common.SetContextKey(c, constant.ContextKeyChannelName, channelName)

	info := &relaycommon.RelayInfo{
		IsStream:           true,
		RelayMode:          relayconstant.RelayModeChatCompletions,
		RelayFormat:        types.RelayFormatOpenAI,
		OriginModelName:    modelName,
		DisablePing:        true,
		ShouldIncludeUsage: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    baseURL,
			UpstreamModelName: modelName,
		},
	}
	return c, recorder, info
}
