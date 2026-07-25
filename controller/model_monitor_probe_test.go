package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelMonitorProbeRecordsFirstResponseAndTotalDuration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(server.Close)

	observation, err := runModelMonitorProbe(context.Background(), modelMonitorProbeTestChannel(server.URL), modelMonitorProbeTestTarget(string(constant.EndpointTypeOpenAI)))
	require.NoError(t, err)
	assert.Equal(t, model.ModelMonitorObservationSourceActive, observation.Source)
	assert.Equal(t, model.ModelMonitorStatusAvailable, observation.Status)
	assert.Equal(t, model.ModelMonitorFailureTypeNone, observation.FailureType)
	require.NotNil(t, observation.FirstResponseMS)
	assert.GreaterOrEqual(t, *observation.FirstResponseMS, int64(0))
	assert.GreaterOrEqual(t, observation.TotalDurationMS, *observation.FirstResponseMS)
}

func TestModelMonitorProbeClassifiesHTTPFailures(t *testing.T) {
	testCases := []struct {
		name        string
		statusCode  int
		status      string
		failureType string
	}{
		{
			name:        "unauthorized",
			statusCode:  http.StatusUnauthorized,
			status:      model.ModelMonitorStatusUnavailable,
			failureType: model.ModelMonitorFailureTypeUnauthorized,
		},
		{
			name:        "rate limited",
			statusCode:  http.StatusTooManyRequests,
			status:      model.ModelMonitorStatusLimited,
			failureType: model.ModelMonitorFailureTypeRateLimited,
		},
		{
			name:        "upstream server",
			statusCode:  http.StatusBadGateway,
			status:      model.ModelMonitorStatusUnavailable,
			failureType: model.ModelMonitorFailureTypeUpstreamServer,
		},
		{
			name:        "model not found",
			statusCode:  http.StatusNotFound,
			status:      model.ModelMonitorStatusUnavailable,
			failureType: model.ModelMonitorFailureTypeModelNotFound,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(testCase.statusCode)
			}))
			t.Cleanup(server.Close)

			observation, err := runModelMonitorProbe(context.Background(), modelMonitorProbeTestChannel(server.URL), modelMonitorProbeTestTarget(string(constant.EndpointTypeOpenAI)))
			require.NoError(t, err)
			assert.Equal(t, testCase.status, observation.Status)
			assert.Equal(t, testCase.failureType, observation.FailureType)
			assert.Nil(t, observation.FirstResponseMS)
			assert.NotEmpty(t, observation.ErrorSummary)
		})
	}
}

func TestModelMonitorProbeClassifiesTimeoutAndStreamBreak(t *testing.T) {
	t.Run("timeout before first event", func(t *testing.T) {
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}))
		t.Cleanup(func() {
			close(release)
			server.Close()
		})

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		t.Cleanup(cancel)

		observation, err := runModelMonitorProbe(ctx, modelMonitorProbeTestChannel(server.URL), modelMonitorProbeTestTarget(string(constant.EndpointTypeOpenAI)))
		require.NoError(t, err)
		assert.Equal(t, model.ModelMonitorStatusUnavailable, observation.Status)
		assert.Equal(t, model.ModelMonitorFailureTypeTimeout, observation.FailureType)
		assert.Nil(t, observation.FirstResponseMS)
	})

	t.Run("stream breaks after first event", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		}))
		t.Cleanup(server.Close)

		observation, err := runModelMonitorProbe(context.Background(), modelMonitorProbeTestChannel(server.URL), modelMonitorProbeTestTarget(string(constant.EndpointTypeOpenAI)))
		require.NoError(t, err)
		assert.Equal(t, model.ModelMonitorStatusUnavailable, observation.Status)
		assert.Equal(t, model.ModelMonitorFailureTypeStreamBreak, observation.FailureType)
		require.NotNil(t, observation.FirstResponseMS)
	})
}

func TestModelMonitorProbeUsesConfiguredEndpointWithoutFallback(t *testing.T) {
	var requests atomic.Int64
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		requestPath = r.URL.Path
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	observation, err := runModelMonitorProbe(context.Background(), modelMonitorProbeTestChannel(server.URL), modelMonitorProbeTestTarget(string(constant.EndpointTypeOpenAIResponse)))
	require.NoError(t, err)
	assert.Equal(t, model.ModelMonitorStatusUnavailable, observation.Status)
	assert.Equal(t, model.ModelMonitorFailureTypeModelNotFound, observation.FailureType)
	assert.Equal(t, int64(1), requests.Load())
	assert.Equal(t, "/v1/responses", requestPath)
}

func TestModelMonitorProbeDoesNotChangeChannelOrWriteConsumeLogs(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(server.Close)

	channel := modelMonitorProbeTestChannel(server.URL)
	channel.Id = 991
	channel.Status = 1
	channel.ResponseTime = 87
	require.NoError(t, db.Create(channel).Error)

	observation, err := runModelMonitorProbe(context.Background(), channel, modelMonitorProbeTestTarget(string(constant.EndpointTypeOpenAI)))
	require.NoError(t, err)
	assert.Equal(t, model.ModelMonitorStatusAvailable, observation.Status)

	var persisted model.Channel
	require.NoError(t, db.First(&persisted, channel.Id).Error)
	assert.Equal(t, 1, persisted.Status)
	assert.Equal(t, 87, persisted.ResponseTime)

	var consumeLogCount int64
	require.NoError(t, db.Model(&model.Log{}).Count(&consumeLogCount).Error)
	assert.Zero(t, consumeLogCount)
}

func modelMonitorProbeTestChannel(baseURL string) *model.Channel {
	service.InitHttpClient()
	return &model.Channel{
		Id:      990,
		Type:    constant.ChannelTypeOpenAI,
		Key:     "probe-test-key",
		BaseURL: &baseURL,
		Models:  "gpt-5",
	}
}

func modelMonitorProbeTestTarget(endpointType string) model.ModelMonitorTarget {
	return model.ModelMonitorTarget{
		ID:           77,
		SiteID:       12,
		ModelName:    "gpt-5",
		EndpointType: endpointType,
		Weight:       1,
		Enabled:      true,
	}
}
