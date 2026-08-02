package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInFlightRequestCancellationUsesSeparateRelayContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	const pendingLogId = 90210
	RegisterInFlightRequest(c, pendingLogId)
	t.Cleanup(func() {
		UnregisterInFlightRequest(pendingLogId)
	})

	relayCtx := RelayRequestContext(c)
	require.NotEqual(t, c.Request.Context(), relayCtx)
	require.NoError(t, relayCtx.Err())
	require.True(t, CancelInFlightRequest(pendingLogId))

	<-relayCtx.Done()
	assert.ErrorIs(t, context.Cause(relayCtx), ErrInFlightRequestCancelled)
	assert.NoError(t, c.Request.Context().Err())
	assert.True(t, IsInFlightRequestCancelled(c))

	apiErr := NewInFlightRequestCancelledError()
	assert.Equal(t, types.ErrorCodeRequestCancelled, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	assert.True(t, types.IsSkipRetryError(apiErr))
}

func TestCancelInFlightRequestRejectsUnknownLog(t *testing.T) {
	assert.False(t, CancelInFlightRequest(987654321))
}
