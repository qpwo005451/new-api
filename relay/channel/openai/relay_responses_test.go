package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
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
