package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInputDeepSeekV4FlashDoResponseStreamPreservesToolCallArguments(t *testing.T) {
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
			`data: {"id":"chatcmpl_input","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_input","type":"function","function":{"name":"lookup","arguments":""}}]},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl_input","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":\"x\"}"}}]},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl_input","object":"chat.completion.chunk","created":1710000000,"model":"deepseek-v4-flash","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":2,"completion_tokens":4,"total_tokens":6}}`,
			`data: [DONE]`,
			``,
		}, "\n"))),
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	usage, err := (&Adaptor{}).DoResponse(c, resp, info)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 6, usage.(*dto.Usage).TotalTokens)

	body := recorder.Body.String()
	require.Contains(t, body, "event: response.function_call_arguments.done")
	require.Contains(t, body, `"arguments":"{\"q\":\"x\"}"`)
	require.Contains(t, body, "event: response.completed")
	require.NotContains(t, body, "chat.completion.chunk")
}
