package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldRetryRequiresReplaySafeUncommittedOutcome(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	apiErr := types.NewErrorWithStatusCode(
		assert.AnError,
		types.ErrorCodeDoRequestFailed,
		http.StatusBadGateway,
	)

	committed := relaycommon.AttemptOutcome{
		DownstreamCommitted: true,
		ReplaySafe:          true,
		UpstreamState:       relaycommon.AttemptUpstreamNotRun,
	}
	assert.False(t, committed.CanRetry())
	assert.True(t, shouldRetry(ctx, apiErr, 1), "the existing status policy remains intact")

	uncommitted := relaycommon.AttemptOutcome{
		ReplaySafe:    true,
		UpstreamState: relaycommon.AttemptUpstreamNotRun,
	}
	require.True(t, uncommitted.CanRetry())
}

func TestVirtualPoolRetryConsumesFailedBindingAndLeavesSuccessConfirmed(t *testing.T) {
	installVirtualPoolControllerTestSetting(t)
	bindings := service.NewVirtualPoolMemoryBindingStore()
	capacity := service.NewVirtualPoolMemoryCapacityStore()
	scheduler := service.NewVirtualPoolScheduler(bindings, capacity)
	now := time.Now()
	candidates := []service.VirtualPoolCandidate{
		{AccountIdentity: "channel:1", FinalMappedModel: "model", Capacity: 1},
		{AccountIdentity: "channel:2", FinalMappedModel: "model", Capacity: 1},
	}

	first, err := scheduler.Select(context.Background(), "session", "owner-1", candidates, now)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NoError(t, scheduler.Abort(context.Background(), first, now))
	require.False(t, bindings.Release(context.Background(), "session", "owner-1", first.Binding.Generation, now))

	second, err := scheduler.Select(context.Background(), "session", "owner-2", candidates, now.Add(time.Second))
	require.NoError(t, err)
	require.NotNil(t, second)
	require.Equal(t, "channel:1", second.Candidate.AccountIdentity,
		"releasing a failed pending binding permits a fresh claim")
	require.True(t, scheduler.Confirm(context.Background(), second, now.Add(2*time.Second)))
	require.NoError(t, scheduler.Release(context.Background(), second))

	third, err := scheduler.Select(context.Background(), "session", "owner-3", candidates, now.Add(3*time.Second))
	require.NoError(t, err)
	require.NotNil(t, third)
	assert.Equal(t, "channel:1", third.Candidate.AccountIdentity,
		"after protocol success the confirmed binding must remain sticky")
}

func installVirtualPoolControllerTestSetting(t *testing.T) {
	t.Helper()
	retrySetting := operation_setting.GetModelRetryPolicySetting()
	original := retrySetting.VirtualPoolSticky
	retrySetting.VirtualPoolSticky = operation_setting.VirtualPoolStickySetting{
		Enabled:        true,
		BindingMode:    operation_setting.VirtualPoolBindingModeMemory,
		SessionMode:    operation_setting.VirtualPoolSessionModeThread,
		MultiKeyPolicy: operation_setting.VirtualPoolMultiKeyPolicyBindIndex,
	}
	t.Cleanup(func() {
		retrySetting.VirtualPoolSticky = original
	})
}

func TestShouldRetryVirtualPoolAttemptStopsAfterDownstreamCommit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	apiErr := types.NewErrorWithStatusCode(
		assert.AnError,
		types.ErrorCodeDoRequestFailed,
		http.StatusBadGateway,
	)

	committed := relaycommon.AttemptOutcome{
		DownstreamCommitted: true,
		ReplaySafe:          true,
		UpstreamState:       relaycommon.AttemptUpstreamNotRun,
	}
	assert.False(t, shouldRetryVirtualPoolAttempt(ctx, apiErr, committed, 3))

	uncommitted := relaycommon.AttemptOutcome{
		ReplaySafe:    true,
		UpstreamState: relaycommon.AttemptUpstreamNotRun,
	}
	assert.True(t, shouldRetryVirtualPoolAttempt(ctx, apiErr, uncommitted, 3))
}

func TestVirtualPoolCommittedProtocolFailureNeverRetries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	apiErr := types.NewErrorWithStatusCode(
		assert.AnError,
		types.ErrorCodeDoRequestFailed,
		http.StatusBadGateway,
	)

	outcome := relaycommon.AttemptOutcome{
		DownstreamCommitted: true,
		ProtocolCompleted:   false,
		ReplaySafe:          true,
		UpstreamState:       relaycommon.AttemptUpstreamAccepted,
	}
	assert.False(t, shouldRetryVirtualPoolAttempt(ctx, apiErr, outcome, 3),
		"a stream that already produced client-visible bytes must not fail over")
	assert.False(t, outcome.CanRetry())
}

func TestVirtualPoolAttemptWithoutDownstreamBytesCanRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	apiErr := types.NewErrorWithStatusCode(
		assert.AnError,
		types.ErrorCodeDoRequestFailed,
		http.StatusBadGateway,
	)
	outcome := relaycommon.AttemptOutcome{
		DownstreamCommitted: false,
		ProtocolCompleted:   false,
		ReplaySafe:          true,
		UpstreamState:       relaycommon.AttemptUpstreamNotRun,
	}

	assert.True(t, shouldRetryVirtualPoolAttempt(ctx, apiErr, outcome, 3),
		"a pre-send failure may move to another candidate")
}

func TestShouldRetryVirtualPoolAttemptRequiresOutcome(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	apiErr := types.NewErrorWithStatusCode(
		assert.AnError,
		types.ErrorCodeDoRequestFailed,
		http.StatusBadGateway,
	)
	unknown := relaycommon.AttemptOutcome{UpstreamState: relaycommon.AttemptUpstreamUnknown}

	assert.False(t, shouldRetryVirtualPoolAttempt(ctx, apiErr, unknown, 3),
		"without a complete outcome the gateway must treat the attempt as unknown")
}
