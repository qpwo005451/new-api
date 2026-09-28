package common

import (
	"errors"
	"fmt"
	"sync"
)

type AttemptFailureCategory string

const (
	AttemptFailureNone            AttemptFailureCategory = ""
	AttemptFailureUpstreamStatus  AttemptFailureCategory = "upstream_status"
	AttemptFailureTransport       AttemptFailureCategory = "transport"
	AttemptFailureProtocol        AttemptFailureCategory = "protocol"
	AttemptFailureClientCancelled AttemptFailureCategory = "client_cancelled"
	AttemptFailureUnknown         AttemptFailureCategory = "unknown"
)

type AttemptUpstreamState string

const (
	AttemptUpstreamUnknown  AttemptUpstreamState = "unknown"
	AttemptUpstreamNotRun   AttemptUpstreamState = "not_run"
	AttemptUpstreamAccepted AttemptUpstreamState = "accepted"
)

var (
	ErrAttemptOutcomeMissing   = errors.New("attempt outcome was not initialized")
	ErrAttemptOutcomeCommitted = errors.New("attempt outcome was already committed")
)

type AttemptOutcome struct {
	AttemptID           string
	CandidateKey        string
	DownstreamCommitted bool
	UpstreamState       AttemptUpstreamState
	ProtocolCompleted   bool
	FailureCategory     AttemptFailureCategory
	ReplaySafe          bool
	UsageSettled        bool
}

func (outcome AttemptOutcome) IsCompleteSuccess() bool {
	return outcome.ProtocolCompleted &&
		outcome.FailureCategory == AttemptFailureNone &&
		outcome.DownstreamCommitted
}

func (outcome AttemptOutcome) CanRetry() bool {
	return !outcome.DownstreamCommitted &&
		outcome.ReplaySafe &&
		outcome.UpstreamState == AttemptUpstreamNotRun
}

type AttemptRecorder struct {
	mu       sync.Mutex
	active   bool
	outcome  AttemptOutcome
	revision uint64
}

func (recorder *AttemptRecorder) IsActive() bool {
	if recorder == nil {
		return false
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return recorder.active
}

func (recorder *AttemptRecorder) Begin(attemptID string, candidateKey string) error {
	if recorder == nil {
		return ErrAttemptOutcomeMissing
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if recorder.active {
		return ErrAttemptOutcomeCommitted
	}
	recorder.active = true
	recorder.revision++
	recorder.outcome = AttemptOutcome{
		AttemptID:     attemptID,
		CandidateKey:  candidateKey,
		UpstreamState: AttemptUpstreamUnknown,
	}
	return nil
}

func (recorder *AttemptRecorder) MarkDownstreamCommitted() error {
	return recorder.update(func(outcome *AttemptOutcome) {
		outcome.DownstreamCommitted = true
	})
}

func (recorder *AttemptRecorder) MarkUpstreamState(state AttemptUpstreamState) error {
	return recorder.update(func(outcome *AttemptOutcome) {
		outcome.UpstreamState = state
	})
}

func (recorder *AttemptRecorder) MarkProtocolCompleted() error {
	return recorder.update(func(outcome *AttemptOutcome) {
		outcome.ProtocolCompleted = true
		outcome.FailureCategory = AttemptFailureNone
	})
}

func (recorder *AttemptRecorder) MarkFailure(category AttemptFailureCategory, replaySafe bool, state AttemptUpstreamState) error {
	return recorder.update(func(outcome *AttemptOutcome) {
		outcome.FailureCategory = category
		outcome.ReplaySafe = replaySafe
		outcome.UpstreamState = state
	})
}

func (recorder *AttemptRecorder) MarkSuccess() error {
	return recorder.update(func(outcome *AttemptOutcome) {
		outcome.ProtocolCompleted = true
		outcome.FailureCategory = AttemptFailureNone
		outcome.ReplaySafe = false
		if outcome.UpstreamState == AttemptUpstreamUnknown {
			outcome.UpstreamState = AttemptUpstreamAccepted
		}
	})
}

func (recorder *AttemptRecorder) MarkUsageSettled() error {
	return recorder.update(func(outcome *AttemptOutcome) {
		outcome.UsageSettled = true
	})
}

func (recorder *AttemptRecorder) Snapshot() (AttemptOutcome, error) {
	if recorder == nil {
		return AttemptOutcome{}, ErrAttemptOutcomeMissing
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if !recorder.active {
		return AttemptOutcome{}, ErrAttemptOutcomeMissing
	}
	return recorder.outcome, nil
}

func (recorder *AttemptRecorder) Commit() (AttemptOutcome, error) {
	if recorder == nil {
		return AttemptOutcome{}, ErrAttemptOutcomeMissing
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if !recorder.active {
		return AttemptOutcome{}, ErrAttemptOutcomeMissing
	}
	outcome := recorder.outcome
	recorder.active = false
	return outcome, nil
}

func (recorder *AttemptRecorder) update(update func(*AttemptOutcome)) error {
	if recorder == nil {
		return ErrAttemptOutcomeMissing
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if !recorder.active {
		return fmt.Errorf("%w: %v", ErrAttemptOutcomeCommitted, recorder.outcome.AttemptID)
	}
	update(&recorder.outcome)
	return nil
}
