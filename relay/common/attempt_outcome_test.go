package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttemptRecorderRequiresProtocolCompletionForSuccess(t *testing.T) {
	recorder := &AttemptRecorder{}
	require.NoError(t, recorder.Begin("attempt-1", "channel:1|model"))
	require.NoError(t, recorder.MarkDownstreamCommitted())

	outcome, err := recorder.Snapshot()
	require.NoError(t, err)
	assert.False(t, outcome.IsCompleteSuccess())
	assert.False(t, outcome.CanRetry(), "a committed response must not be retried")

	require.NoError(t, recorder.MarkProtocolCompleted())
	outcome, err = recorder.Commit()
	require.NoError(t, err)
	assert.True(t, outcome.IsCompleteSuccess())
}

func TestAttemptRecorderAllowsRetryOnlyBeforeUpstreamOrDownstream(t *testing.T) {
	recorder := &AttemptRecorder{}
	require.NoError(t, recorder.Begin("attempt-2", "channel:1|model"))
	require.NoError(t, recorder.MarkFailure(AttemptFailureTransport, true, AttemptUpstreamNotRun))

	outcome, err := recorder.Snapshot()
	require.NoError(t, err)
	assert.True(t, outcome.CanRetry())

	require.NoError(t, recorder.MarkUpstreamState(AttemptUpstreamAccepted))
	outcome, err = recorder.Snapshot()
	require.NoError(t, err)
	assert.False(t, outcome.CanRetry(), "accepted-but-unknown work must not be replayed")
}

func TestAttemptRecorderRejectsDoubleBeginAndWriteAfterCommit(t *testing.T) {
	recorder := &AttemptRecorder{}
	require.NoError(t, recorder.Begin("attempt-3", "channel:1|model"))
	require.ErrorIs(t, recorder.Begin("attempt-4", "channel:2|model"), ErrAttemptOutcomeCommitted)

	_, err := recorder.Commit()
	require.NoError(t, err)
	require.ErrorIs(t, recorder.MarkProtocolCompleted(), ErrAttemptOutcomeCommitted)
	_, err = recorder.Snapshot()
	require.ErrorIs(t, err, ErrAttemptOutcomeMissing)
}

func TestAttemptRecorderMarkSuccessFinalizesAcceptedAttempt(t *testing.T) {
	recorder := &AttemptRecorder{}
	require.NoError(t, recorder.Begin("attempt-success", "candidate"))
	require.NoError(t, recorder.MarkSuccess())

	outcome, err := recorder.Snapshot()
	require.NoError(t, err)
	assert.True(t, outcome.ProtocolCompleted)
	assert.Equal(t, AttemptFailureNone, outcome.FailureCategory)
	assert.Equal(t, AttemptUpstreamAccepted, outcome.UpstreamState)
	assert.False(t, outcome.ReplaySafe)
}
