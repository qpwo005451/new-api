package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetChannelPreservesPinnedMultiKeyMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("channel_id", 31)
	ctx.Set("channel_type", 1)
	ctx.Set("channel_name", "multi-key")
	ctx.Set("auto_ban", true)
	common.SetContextKey(ctx, constant.ContextKeyChannelIsMultiKey, true)

	channel, err := getChannel(ctx, &relaycommon.RelayInfo{}, &service.RetryParam{})

	require.Nil(t, err)
	require.NotNil(t, channel)
	assert.True(t, channel.ChannelInfo.IsMultiKey)
	assert.True(t, channel.GetAutoBan())
}

func TestTransientRetryBackoff(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		retryIndex int
		want       time.Duration
	}{
		{name: "first 429", statusCode: http.StatusTooManyRequests, retryIndex: 0, want: 300 * time.Millisecond},
		{name: "second 502", statusCode: http.StatusBadGateway, retryIndex: 1, want: 800 * time.Millisecond},
		{name: "third 503", statusCode: http.StatusServiceUnavailable, retryIndex: 2, want: 1500 * time.Millisecond},
		{name: "later transient retry stays bounded", statusCode: http.StatusServiceUnavailable, retryIndex: 8, want: 1500 * time.Millisecond},
		{name: "non transient status has no delay", statusCode: http.StatusInternalServerError, retryIndex: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, transientRetryBackoff(tt.statusCode, tt.retryIndex))
		})
	}
}

func TestShouldRetryStopsWhenRelayContextIsDone(t *testing.T) {
	tests := []struct {
		name   string
		cancel func(context.Context) context.Context
	}{
		{
			name: "canceled",
			cancel: func(parent context.Context) context.Context {
				ctx, cancel := context.WithCancel(parent)
				cancel()
				return ctx
			},
		},
		{
			name: "deadline exceeded",
			cancel: func(parent context.Context) context.Context {
				ctx, cancel := context.WithTimeout(parent, 0)
				cancel()
				return ctx
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			ctx.Request = request.WithContext(test.cancel(request.Context()))
			apiErr := types.NewErrorWithStatusCode(
				io.EOF,
				types.ErrorCodeDoRequestFailed,
				http.StatusBadGateway,
			)

			assert.False(t, shouldRetry(ctx, apiErr, 1))
		})
	}
}

func TestShouldRetryAllowsTransportFailureWhileRetriesRemain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	apiErr := types.NewErrorWithStatusCode(
		io.EOF,
		types.ErrorCodeDoRequestFailed,
		http.StatusBadGateway,
	)

	assert.True(t, shouldRetry(ctx, apiErr, 1))
	assert.False(t, shouldRetry(ctx, apiErr, 0))
}
