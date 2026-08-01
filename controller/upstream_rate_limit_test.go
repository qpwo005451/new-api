package controller

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUpstreamRateLimitTarget(t *testing.T) {
	setting := operation_setting.GetUpstreamRateLimitSetting()
	original := *setting
	*setting = operation_setting.UpstreamRateLimitSetting{
		Enabled: true,
		Rules: []operation_setting.UpstreamRateLimitRule{
			{
				Name:        "Input Kimi",
				BaseURLHost: "ai.input.im",
				Models:      []string{"kimi-k2.7-code"},
				RPM:         10,
			},
		},
	}
	t.Cleanup(func() {
		*setting = original
	})

	inputURL := "https://AI.Input.IM/v1"
	otherURL := "https://example.com/v1"
	target, ok := getUpstreamRateLimitTarget(inputURL, "KIMI-K2.7-CODE", "key-a")
	require.True(t, ok)
	assert.Equal(t, 10, target.rule.RPM)

	sameKeyTarget, ok := getUpstreamRateLimitTarget(inputURL, "kimi-k2.7-code", "key-a")
	require.True(t, ok)
	assert.Equal(t, target.gateKey, sameKeyTarget.gateKey)

	_, ok = getUpstreamRateLimitTarget(inputURL, "other-model", "key-a")
	assert.False(t, ok)

	_, ok = getUpstreamRateLimitTarget(otherURL, "kimi-k2.7-code", "key-a")
	assert.False(t, ok)

	otherKeyTarget, ok := getUpstreamRateLimitTarget(inputURL, "kimi-k2.7-code", "key-b")
	require.True(t, ok)
	assert.NotEqual(t, target.gateKey, otherKeyTarget.gateKey)
}

func TestUpstreamRateLimitGateHonorsRPMCooldownAndCancellation(t *testing.T) {
	gate := &upstreamRateLimitGate{}
	now := time.Date(2026, time.August, 1, 10, 0, 0, 0, time.UTC)

	require.Zero(t, gate.admissionDelay(now, 10))
	assert.Equal(t, 6*time.Second, gate.admissionDelay(now, 10))

	gate.setCooldown(now, time.Minute)
	assert.Equal(t, time.Minute, gate.admissionDelay(now, 10))
	assert.Equal(t, 60, gate.retryAfterSeconds(now))

	cancelledGate := &upstreamRateLimitGate{}
	cancelledGate.setCooldown(time.Now(), time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := cancelledGate.wait(ctx, 10)
	require.ErrorIs(t, err, context.Canceled)
}
