package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcquireOpenCodeRouteLease(t *testing.T) {
	var request OpenCodeRouteLeaseRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, decodeOpenCodeRouteFeedbackJSON(r.Body, &request))
		assert.Equal(t, "req-1", request.RequestID)
		assert.NotEmpty(t, r.Header.Get(openCodeRouteFeedbackSignatureHeader))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"lease_id":"lease-1","service":"opencode_go","route_key":"Yifen::NewYork","route_generation":7,"expires_at":2000}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv(openCodeRouteFeedbackURLEnv, server.URL)
	t.Setenv(openCodeRouteFeedbackSecretEnv, "shared-secret")

	lease, err := acquireOpenCodeRouteLease(context.Background(), "req-1")

	require.NoError(t, err)
	require.NotNil(t, lease)
	assert.Equal(t, "lease-1", lease.LeaseID)
	assert.Equal(t, "Yifen::NewYork", lease.RouteKey)
	assert.Equal(t, int64(7), lease.RouteGeneration)
}

func TestBuildOpenCodeRouteFeedback(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RequestId:                   "req-2",
		OriginModelName:             "deepseek-v4-flash",
		ReceivedResponseCount:       1298,
		OpenCodeStreamRetryCount:    1,
		OpenCodeRecoveredAfterRetry: false,
		LastError: types.NewOpenAIError(
			assert.AnError,
			types.ErrorCodeReadResponseBodyFailed,
			http.StatusBadGateway,
			types.ErrOptionWithUpstreamErrorInfo("upstream_stream_terminated", ""),
		),
	}
	lease := &OpenCodeRouteLease{LeaseID: "lease-2", RouteKey: "Yifen::NewYork", RouteGeneration: 8}

	feedback := buildOpenCodeRouteFeedback(info, lease, false)

	require.NotNil(t, feedback)
	assert.Equal(t, "stream_incomplete", feedback.Outcome)
	assert.Equal(t, "upstream_stream_terminated", feedback.TransportKind)
	assert.Equal(t, 1298, feedback.ReceivedEvents)
	assert.Equal(t, 1, feedback.RetryCount)
	assert.False(t, feedback.RecoveredAfterRetry)
}

func TestBuildOpenCodeRouteFeedbackIgnoresClientCancelPenalty(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RequestId:       "req-3",
		OriginModelName: "deepseek-v4-flash",
		LastError: types.NewOpenAIError(
			context.Canceled,
			types.ErrorCodeDoRequestFailed,
			499,
			types.ErrOptionWithUpstreamErrorInfo("client_canceled", ""),
		),
	}

	feedback := buildOpenCodeRouteFeedback(info, &OpenCodeRouteLease{LeaseID: "lease-3"}, false)

	require.NotNil(t, feedback)
	assert.Equal(t, "client_canceled", feedback.Outcome)
	assert.Equal(t, "client_canceled", feedback.TransportKind)
}

func TestBuildOpenCodeRouteFeedbackReleasesLeaseForServiceFailure(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RequestId:       "req-service",
		OriginModelName: "deepseek-v4-flash",
		LastError: types.NewOpenAIError(
			assert.AnError,
			types.ErrorCodeBadResponseStatusCode,
			http.StatusTooManyRequests,
			types.ErrOptionWithUpstreamErrorInfo("rate_limited", ""),
		),
	}

	feedback := buildOpenCodeRouteFeedback(info, &OpenCodeRouteLease{LeaseID: "lease-service"}, false)

	require.NotNil(t, feedback)
	assert.Equal(t, "service_failure", feedback.Outcome)
	assert.Equal(t, "rate_limited", feedback.TransportKind)
}

func TestReportOpenCodeRouteFeedbackAsync(t *testing.T) {
	var mu sync.Mutex
	var received OpenCodeRouteFeedback
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		require.NoError(t, decodeOpenCodeRouteFeedbackJSON(r.Body, &received))
		close(done)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	t.Setenv(openCodeRouteFeedbackURLEnv, server.URL)
	t.Setenv(openCodeRouteFeedbackSecretEnv, "shared-secret")

	reportOpenCodeRouteFeedbackAsync(OpenCodeRouteFeedback{
		LeaseID:   "lease-4",
		RequestID: "req-4",
		Outcome:   "success",
	})

	<-done
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "lease-4", received.LeaseID)
}

func TestShouldUseOpenCodeRouteFeedback(t *testing.T) {
	base := &relaycommon.RelayInfo{
		IsStream:        true,
		OriginModelName: "deepseek-v4-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: "https://opencode.ai/zen/go",
			ChannelSetting: dto.ChannelSettings{Proxy: "http://192.168.1.7:7893"},
		},
	}

	assert.True(t, ShouldUseOpenCodeRouteFeedback(base))

	noProxy := *base
	noProxy.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelBaseUrl: base.ChannelBaseUrl,
		ChannelSetting: dto.ChannelSettings{},
	}
	assert.False(t, ShouldUseOpenCodeRouteFeedback(&noProxy))

	notStream := *base
	notStream.IsStream = false
	assert.True(t, ShouldUseOpenCodeRouteFeedback(&notStream))

	assert.NotPanics(t, func() {
		assert.False(t, ShouldUseOpenCodeRouteFeedback(&relaycommon.RelayInfo{
			IsStream:        true,
			OriginModelName: "deepseek-v4-flash",
		}))
	})
}
