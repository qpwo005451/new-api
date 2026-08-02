package controller

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
)

const (
	inputTransientRetryRequestSpacing = time.Second
	inputTransientRetryHost           = "ai.input.im"
)

var (
	inputTransientRetryModels = map[string]struct{}{
		"glm-5.2": {},
		"gemma-4": {},
	}
	inputTransientRetryCooldowns = [...]time.Duration{
		15 * time.Second,
		30 * time.Second,
		45 * time.Second,
	}
	inputTransientRetryGates sync.Map
)

// inputTransientRetryGate serializes starts for a rate-limited Input channel
// and keeps new requests behind a cooldown raised by a transient upstream error.
type inputTransientRetryGate struct {
	mu            sync.Mutex
	nextAttemptAt time.Time
}

func isInputTransientRetryTarget(channel *model.Channel, modelName string) bool {
	if channel == nil {
		return false
	}
	if _, ok := inputTransientRetryModels[strings.ToLower(strings.TrimSpace(modelName))]; !ok {
		return false
	}
	baseURL, err := url.Parse(channel.GetBaseURL())
	if err != nil {
		return false
	}
	return strings.EqualFold(baseURL.Hostname(), inputTransientRetryHost)
}

func isInputTransientRetryError(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	switch err.StatusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable:
		return true
	case http.StatusInternalServerError:
		return err.GetErrorCode() == types.ErrorCodeDoRequestFailed
	default:
		return false
	}
}

func inputTransientRetryCooldown(retryIndex int) time.Duration {
	if retryIndex < 0 {
		retryIndex = 0
	}
	if retryIndex >= len(inputTransientRetryCooldowns) {
		return inputTransientRetryCooldowns[len(inputTransientRetryCooldowns)-1]
	}
	return inputTransientRetryCooldowns[retryIndex]
}

func inputTransientRetryGateFor(channelID int) *inputTransientRetryGate {
	value, _ := inputTransientRetryGates.LoadOrStore(channelID, &inputTransientRetryGate{})
	return value.(*inputTransientRetryGate)
}

func (gate *inputTransientRetryGate) wait(ctx context.Context) (time.Duration, error) {
	startedAt := time.Now()
	waited := false
	for {
		delay := gate.admissionDelay(time.Now())
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

func (gate *inputTransientRetryGate) admissionDelay(now time.Time) time.Duration {
	gate.mu.Lock()
	defer gate.mu.Unlock()

	if gate.nextAttemptAt.After(now) {
		return gate.nextAttemptAt.Sub(now)
	}
	gate.nextAttemptAt = now.Add(inputTransientRetryRequestSpacing)
	return 0
}

func (gate *inputTransientRetryGate) setCooldown(now time.Time, cooldown time.Duration) {
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

func (gate *inputTransientRetryGate) retryAfterSeconds(now time.Time) int {
	gate.mu.Lock()
	defer gate.mu.Unlock()

	delay := gate.nextAttemptAt.Sub(now)
	if delay <= 0 {
		return 1
	}
	return int((delay + time.Second - 1) / time.Second)
}
