package openai

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type unexpectedEOFReader struct {
	data []byte
}

func (r *unexpectedEOFReader) Read(p []byte) (int, error) {
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

func newChatStreamTestContext(body io.Reader) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(body),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	info := &relaycommon.RelayInfo{
		IsStream:    true,
		RelayMode:   relayconstant.RelayModeChatCompletions,
		RelayFormat: types.RelayFormatOpenAI,
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "deepseek-v4-flash",
		},
	}
	return c, recorder, resp, info
}

func setChatStreamTestTimeout(t *testing.T) {
	t.Helper()
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})
}

func TestOaiStreamHandlerReportsUnexpectedEOFWithoutDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setChatStreamTestTimeout(t)

	body := strings.Join([]string{
		`data: {"id":"chatcmpl-truncated","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"reasoning_content":"partial"},"finish_reason":null}]}`,
		``,
	}, "\n")
	c, recorder, resp, info := newChatStreamTestContext(&unexpectedEOFReader{data: []byte(body)})

	usage, newAPIError := OaiStreamHandler(c, info, resp)

	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	require.NotNil(t, info.StreamStatus)
	assert.ErrorIs(t, info.StreamStatus.EndError, io.ErrUnexpectedEOF)
	assert.Contains(t, recorder.Body.String(), `"error"`)
	assert.Contains(t, recorder.Body.String(), "upstream stream terminated unexpectedly")
	assert.NotContains(t, recorder.Body.String(), "data: [DONE]")
}

func TestOaiStreamHandlerKeepsNormalDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setChatStreamTestTimeout(t)

	body := strings.Join([]string{
		`data: {"id":"chatcmpl-ok","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"OK"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-ok","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp, info := newChatStreamTestContext(strings.NewReader(body))

	usage, newAPIError := OaiStreamHandler(c, info, resp)

	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.TotalTokens)
	assert.Contains(t, recorder.Body.String(), `"content":"OK"`)
	assert.Contains(t, recorder.Body.String(), "data: [DONE]")
	assert.NotContains(t, recorder.Body.String(), `"error"`)
	assert.False(t, errors.Is(info.StreamStatus.EndError, io.ErrUnexpectedEOF))
}

func TestOaiStreamHandlerAcceptsFinishReasonFollowedByEOF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setChatStreamTestTimeout(t)

	body := strings.Join([]string{
		`data: {"id":"chatcmpl-eof","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"OK"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-eof","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
		``,
	}, "\n")
	c, recorder, resp, info := newChatStreamTestContext(strings.NewReader(body))

	usage, newAPIError := OaiStreamHandler(c, info, resp)

	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.TotalTokens)
	assert.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)
	assert.Contains(t, recorder.Body.String(), "data: [DONE]")
	assert.NotContains(t, recorder.Body.String(), `"error"`)
}

func TestOaiStreamHandlerReportsCleanEOFWithoutFinishReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setChatStreamTestTimeout(t)

	body := strings.Join([]string{
		`data: {"id":"chatcmpl-no-finish","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}`,
		``,
	}, "\n")
	c, recorder, resp, info := newChatStreamTestContext(strings.NewReader(body))

	usage, newAPIError := OaiStreamHandler(c, info, resp)

	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	assert.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)
	assert.Contains(t, recorder.Body.String(), "upstream stream terminated unexpectedly")
	assert.NotContains(t, recorder.Body.String(), "data: [DONE]")
}

func TestOaiStreamHandlerAcceptsFinishReasonBeforeUnexpectedEOF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setChatStreamTestTimeout(t)

	body := strings.Join([]string{
		`data: {"id":"chatcmpl-finished","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"content":"OK"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl-finished","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`,
		``,
	}, "\n")
	c, recorder, resp, info := newChatStreamTestContext(&unexpectedEOFReader{data: []byte(body)})

	usage, newAPIError := OaiStreamHandler(c, info, resp)

	require.Nil(t, newAPIError)
	require.NotNil(t, usage)
	assert.Equal(t, 3, usage.TotalTokens)
	assert.ErrorIs(t, info.StreamStatus.EndError, io.ErrUnexpectedEOF)
	assert.Contains(t, recorder.Body.String(), "data: [DONE]")
	assert.NotContains(t, recorder.Body.String(), `"error"`)
}
