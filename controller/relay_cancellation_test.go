package controller

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestWriteRetryableCancellationStreamUsesResponsesErrorEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	writeRetryableCancellationStream(
		c,
		types.RelayFormatOpenAIResponses,
		service.NewInFlightRequestCancelledError(),
	)

	body := recorder.Body.String()
	assert.Contains(t, body, "event: error")
	assert.Contains(t, body, `"code":"request_cancelled"`)
	assert.Contains(t, body, `"retry_after":1`)
	assert.Contains(t, body, "retry the request")
}

func TestOllamaToolLimitStreamReportsErrorWithoutSuccess(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses} {
		t.Run(string(format), func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			apiErr := types.NewOpenAIError(errors.New("too many tool calls"), types.ErrorCode("ollama_tool_call_limit"), 502, types.ErrOptionWithSkipRetry())
			writeOllamaToolLimitStream(c, format, apiErr)
			body := w.Body.String()
			assert.True(t, strings.HasSuffix(body, "\n\n"))
			assert.NotContains(t, body, "[DONE]")
			assert.NotContains(t, body, "retry_after")
			assert.True(t, types.IsSkipRetryError(apiErr))
			if format == types.RelayFormatOpenAIResponses {
				assert.Contains(t, body, "event: error\n")
			}
			_, data, ok := strings.Cut(body, "data: ")
			require.True(t, ok)
			var payload map[string]any
			require.NoError(t, common.Unmarshal([]byte(strings.TrimSpace(data)), &payload))
			if format == types.RelayFormatOpenAIResponses {
				assert.Equal(t, "ollama_tool_call_limit", payload["code"])
			} else {
				require.Contains(t, payload, "error")
				assert.Contains(t, data, `"code":"ollama_tool_call_limit"`)
			}
		})
	}
}
