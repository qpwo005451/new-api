package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

type upstreamRateLimitTarget struct {
	rule    operation_setting.UpstreamRateLimitRule
	gateKey string
}

type upstreamRateLimitGate struct {
	mu            sync.Mutex
	nextAttemptAt time.Time
}

var upstreamRateLimitGates sync.Map

func getUpstreamRateLimitTarget(channel *model.Channel, modelName, channelKey string) (upstreamRateLimitTarget, bool) {
	if channel == nil {
		return upstreamRateLimitTarget{}, false
	}
	setting := operation_setting.GetUpstreamRateLimitSetting()
	if !setting.Enabled {
		return upstreamRateLimitTarget{}, false
	}
	baseURL, err := url.Parse(channel.GetBaseURL())
	if err != nil {
		return upstreamRateLimitTarget{}, false
	}
	host := strings.ToLower(baseURL.Hostname())
	normalizedModelName := strings.ToLower(strings.TrimSpace(modelName))

	for _, rule := range setting.Rules {
		if rule.RPM <= 0 || !strings.EqualFold(strings.TrimSpace(rule.BaseURLHost), host) {
			continue
		}
		for _, configuredModel := range rule.Models {
			if strings.EqualFold(strings.TrimSpace(configuredModel), normalizedModelName) {
				keyFingerprint := sha256.Sum256([]byte(channelKey))
				return upstreamRateLimitTarget{
					rule:    rule,
					gateKey: fmt.Sprintf("%s:%s:%x", host, normalizedModelName, keyFingerprint[:8]),
				}, true
			}
		}
	}
	return upstreamRateLimitTarget{}, false
}

func upstreamRateLimitGateFor(key string) *upstreamRateLimitGate {
	value, _ := upstreamRateLimitGates.LoadOrStore(key, &upstreamRateLimitGate{})
	return value.(*upstreamRateLimitGate)
}

func (gate *upstreamRateLimitGate) wait(ctx context.Context, rpm int) (time.Duration, error) {
	startedAt := time.Now()
	waited := false
	for {
		delay := gate.admissionDelay(time.Now(), rpm)
		if delay <= 0 {
			if !waited {
				return 0, nil
			}
			return time.Since(startedAt), nil
		}
		waited = true

		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return 0, ctx.Err()
		}
	}
}

func (gate *upstreamRateLimitGate) admissionDelay(now time.Time, rpm int) time.Duration {
	if rpm <= 0 {
		return 0
	}
	spacing := time.Minute / time.Duration(rpm)

	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.nextAttemptAt.After(now) {
		return gate.nextAttemptAt.Sub(now)
	}
	gate.nextAttemptAt = now.Add(spacing)
	return 0
}

func (gate *upstreamRateLimitGate) setCooldown(now time.Time, cooldown time.Duration) {
	if cooldown <= 0 {
		return
	}
	until := now.Add(cooldown)

	gate.mu.Lock()
	defer gate.mu.Unlock()
	if until.After(gate.nextAttemptAt) {
		gate.nextAttemptAt = until
	}
}

func (gate *upstreamRateLimitGate) retryAfterSeconds(now time.Time) int {
	gate.mu.Lock()
	defer gate.mu.Unlock()

	delay := gate.nextAttemptAt.Sub(now)
	if delay <= 0 {
		return 1
	}
	return int((delay + time.Second - 1) / time.Second)
}
