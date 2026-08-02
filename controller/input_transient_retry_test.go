package controller

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsInputTransientRetryTarget(t *testing.T) {
	inputURL := "https://AI.Input.IM/v1"
	otherURL := "https://example.com/v1"
	tests := []struct {
		name    string
		channel *model.Channel
		model   string
		want    bool
	}{
		{name: "GLM on Input", channel: &model.Channel{BaseURL: &inputURL}, model: "glm-5.2", want: true},
		{name: "Gemma on Input", channel: &model.Channel{BaseURL: &inputURL}, model: "GEMMA-4", want: true},
		{name: "unrelated Input model", channel: &model.Channel{BaseURL: &inputURL}, model: "gpt-5.6-sol", want: false},
		{name: "target model on another provider", channel: &model.Channel{BaseURL: &otherURL}, model: "glm-5.2", want: false},
		{name: "nil channel", channel: nil, model: "glm-5.2", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isInputTransientRetryTarget(tt.channel, tt.model))
		})
	}
}

func TestInputTransientRetryErrorAndCooldown(t *testing.T) {
	tests := []struct {
		name       string
		err        *types.NewAPIError
		retryIndex int
		want       bool
		wantDelay  time.Duration
	}{
		{
			name:       "rate limited GLM",
			err:        types.NewOpenAIError(errors.New("rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests),
			retryIndex: 0,
			want:       true,
			wantDelay:  15 * time.Second,
		},
		{
			name:       "transport EOF",
			err:        types.NewOpenAIError(errors.New("EOF"), types.ErrorCodeDoRequestFailed, http.StatusInternalServerError),
			retryIndex: 1,
			want:       true,
			wantDelay:  30 * time.Second,
		},
		{
			name:       "ordinary upstream 500",
			err:        types.NewOpenAIError(errors.New("bad upstream response"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError),
			retryIndex: 0,
			want:       false,
			wantDelay:  0,
		},
		{
			name:       "invalid request",
			err:        types.NewOpenAIError(errors.New("invalid request"), types.ErrorCodeInvalidRequest, http.StatusBadRequest),
			retryIndex: 0,
			want:       false,
			wantDelay:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isInputTransientRetryError(tt.err))
			if tt.want {
				assert.Equal(t, tt.wantDelay, inputTransientRetryCooldown(tt.retryIndex))
			}
		})
	}
}

func TestInputTransientRetryGateHonorsCooldownAndSpacing(t *testing.T) {
	gate := &inputTransientRetryGate{}
	now := time.Date(2026, time.July, 21, 16, 30, 0, 0, time.UTC)

	require.Zero(t, gate.admissionDelay(now))
	assert.Equal(t, inputTransientRetryRequestSpacing, gate.admissionDelay(now))

	gate.setCooldown(now, 15*time.Second)
	assert.Equal(t, 15*time.Second, gate.admissionDelay(now))
	assert.Equal(t, 15, gate.retryAfterSeconds(now))

	require.Zero(t, gate.admissionDelay(now.Add(15*time.Second)))
	assert.Equal(t, inputTransientRetryRequestSpacing, gate.admissionDelay(now.Add(15*time.Second)))
}

func TestInputTransientRetryGateHonorsCancellation(t *testing.T) {
	gate := &inputTransientRetryGate{}
	gate.setCooldown(time.Now(), time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := gate.wait(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
