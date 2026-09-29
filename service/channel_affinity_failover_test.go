package service

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// installAffinityFailoverRule activates one context_int affinity rule for the
// test and restores the previous setting afterwards.
func installAffinityFailoverRule(t *testing.T) {
	t.Helper()
	setting := operation_setting.GetChannelAffinitySetting()
	orig := *setting
	setting.Enabled = true
	setting.DefaultTTLSeconds = 300
	setting.Rules = []operation_setting.ChannelAffinityRule{{
		Name:              "failover-rule",
		ModelRegex:        []string{"^failover-model$"},
		KeySources:        []operation_setting.ChannelAffinityKeySource{{Type: "context_int", Key: "id"}},
		IncludeRuleName:   true,
		IncludeModelName:  true,
		IncludeUsingGroup: true,
	}}
	t.Cleanup(func() { *setting = orig })
	// The affinity cache is a process-wide singleton; clear it so tests do
	// not leak pins into each other.
	t.Cleanup(func() { ClearChannelAffinityCacheAll() })
}

func newAffinityFailoverContext(t *testing.T, userID int) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", userID)
	return ctx
}

func failoverCacheKey(userID int) string {
	return fmt.Sprintf("failover-rule:failover-model:default:%d", userID)
}

// A failed first attempt that was retried onto another channel must rewrite
// the affinity pin to the channel that actually served the request, so the
// next request does not return to the channel that just failed.
func TestRecordChannelAffinityFailoverRewritesToServingChannel(t *testing.T) {
	for _, switchOnSuccess := range []bool{true, false} {
		t.Run("switch_on_success="+map[bool]string{true: "true", false: "false"}[switchOnSuccess], func(t *testing.T) {
			setting := operation_setting.GetChannelAffinitySetting()
			orig := *setting
			setting.SwitchOnSuccess = switchOnSuccess
			t.Cleanup(func() { *setting = orig })
			installAffinityFailoverRule(t)

			const userID = 1
			ctx := newAffinityFailoverContext(t, userID)
			const modelName = "failover-model"
			preferred, found := GetPreferredChannelByAffinity(ctx, modelName, "default")
			require.False(t, found, "cache starts empty")

			require.NoError(t, getChannelAffinityCache().SetWithTTL(
				failoverCacheKey(userID), 2871, 300*time.Second))
			preferred, found = GetPreferredChannelByAffinity(ctx, modelName, "default")
			require.True(t, found)
			require.Equal(t, 2871, preferred)

			// The distributor defer passes the preferred channel; the relay
			// loop recorded the failover in use_channel and channel_id.
			ctx.Set("channel_id", 2872)
			ctx.Set("use_channel", []string{"2871", "2872"})
			RecordChannelAffinity(ctx, preferred)

			value, found, _ := getChannelAffinityCache().Get(failoverCacheKey(userID))
			require.True(t, found)
			assert.Equal(t, 2872, value,
				"after failover the pin must point to the serving channel regardless of switch_on_success")
		})
	}
}

// A policy-model failure must break the request's affinity pin, so sticky
// traffic does not keep returning to a channel that the circuit breaker is
// counting failures on. The pin is re-established to the serving channel on
// success (see RecordChannelAffinity failover rewrite).
func TestRecordModelHealthFailureClearsPinnedAffinity(t *testing.T) {
	installAffinityFailoverRule(t)
	setting := operation_setting.GetModelHealthPolicySetting()
	orig := *setting
	setting.Enabled = true
	setting.Rules = []operation_setting.ModelHealthPolicyRule{{
		Name:             "failover cooldown",
		Enabled:          true,
		Models:           []string{"failover-model"},
		FailureThreshold: 2,
		CooldownSeconds:  300,
		StatusCodes:      []int{503},
	}}
	t.Cleanup(func() { *setting = orig })

	const userID = 3
	ctx := newAffinityFailoverContext(t, userID)
	const modelName = "failover-model"
	require.NoError(t, getChannelAffinityCache().SetWithTTL(
		failoverCacheKey(userID), 2881, 300*time.Second))
	preferred, found := GetPreferredChannelByAffinity(ctx, modelName, "default")
	require.True(t, found)
	require.Equal(t, 2881, preferred)

	RecordModelHealthFailure(ctx, modelName, "default", 2881, 503)

	_, found, _ = getChannelAffinityCache().Get(failoverCacheKey(userID))
	assert.False(t, found, "a policy-model failure must clear the affinity pin")
}

// Failures on models outside the policy must not touch the pin.
func TestRecordModelHealthFailureKeepsPinForUnmatchedPolicy(t *testing.T) {
	installAffinityFailoverRule(t)
	setting := operation_setting.GetModelHealthPolicySetting()
	orig := *setting
	setting.Enabled = true
	setting.Rules = []operation_setting.ModelHealthPolicyRule{{
		Name:             "failover cooldown",
		Enabled:          true,
		Models:           []string{"failover-model"},
		FailureThreshold: 2,
		CooldownSeconds:  300,
		StatusCodes:      []int{503},
	}}
	t.Cleanup(func() { *setting = orig })

	const userID = 4
	ctx := newAffinityFailoverContext(t, userID)
	require.NoError(t, getChannelAffinityCache().SetWithTTL(
		failoverCacheKey(userID), 2881, 300*time.Second))
	_, found := GetPreferredChannelByAffinity(ctx, "failover-model", "default")
	require.True(t, found)

	RecordModelHealthFailure(ctx, "other-model", "default", 2881, 503)

	_, found, _ = getChannelAffinityCache().Get(failoverCacheKey(userID))
	assert.True(t, found, "non-policy failures must not clear the affinity pin")
}

// A request that succeeded on its first channel must keep the affinity pin on
// the preferred channel even when switch_on_success is enabled, so ordinary
// sticky traffic is not silently re-pinned by every request.
func TestRecordChannelAffinityWithoutFailoverKeepsPreferredChannel(t *testing.T) {
	setting := operation_setting.GetChannelAffinitySetting()
	orig := *setting
	setting.SwitchOnSuccess = true
	t.Cleanup(func() { *setting = orig })
	installAffinityFailoverRule(t)

	const userID = 2
	ctx := newAffinityFailoverContext(t, userID)
	const modelName = "failover-model"
	require.NoError(t, getChannelAffinityCache().SetWithTTL(
		failoverCacheKey(userID), 2871, 300*time.Second))
	preferred, found := GetPreferredChannelByAffinity(ctx, modelName, "default")
	require.True(t, found)
	require.Equal(t, 2871, preferred)

	ctx.Set("channel_id", 2871)
	ctx.Set("use_channel", []string{"2871"})
	RecordChannelAffinity(ctx, preferred)

	value, found, _ := getChannelAffinityCache().Get(failoverCacheKey(userID))
	require.True(t, found)
	assert.Equal(t, 2871, value,
		"without failover the preferred channel stays pinned")
}
