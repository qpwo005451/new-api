package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayReturnsBadRequestForInvalidResponsesTokenLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/responses",
		bytes.NewBufferString(`{"model":"gpt-5.4-mini","input":"ping","max_output_tokens":1073741824}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	Relay(c, types.RelayFormatOpenAIResponses)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "max_output_tokens is invalid")
	assert.Contains(t, recorder.Body.String(), `"code":"invalid_request"`)
}
