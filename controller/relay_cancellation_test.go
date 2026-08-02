package controller

import (
	"net/http/httptest"
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
