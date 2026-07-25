package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelMonitorSchedulerLimitsGlobalAndSiteConcurrency(t *testing.T) {
	scheduler := NewModelMonitorProbeScheduler(ModelMonitorProbeSchedulerConfig{
		GlobalConcurrency: 2,
		SiteMinInterval:   0,
		BackoffBase:       time.Minute,
		BackoffMax:        16 * time.Minute,
	})
	candidates := []ModelMonitorProbeCandidate{
		{SiteID: 1, TargetID: 1, ChannelID: 1},
		{SiteID: 1, TargetID: 2, ChannelID: 2},
		{SiteID: 2, TargetID: 3, ChannelID: 3},
		{SiteID: 3, TargetID: 4, ChannelID: 4},
	}

	started := make(chan ModelMonitorProbeCandidate, len(candidates))
	release := make(chan struct{})
	done := make(chan ModelMonitorProbeScheduleResult, 1)
	var inFlight atomic.Int64
	var maxInFlight atomic.Int64

	go func() {
		done <- scheduler.Run(context.Background(), candidates, func(_ context.Context, candidate ModelMonitorProbeCandidate) {
			current := inFlight.Add(1)
			for {
				previous := maxInFlight.Load()
				if current <= previous || maxInFlight.CompareAndSwap(previous, current) {
					break
				}
			}
			started <- candidate
			<-release
			inFlight.Add(-1)
		})
	}()

	first := <-started
	second := <-started
	assert.NotEqual(t, first.SiteID, second.SiteID)
	assert.LessOrEqual(t, maxInFlight.Load(), int64(2))

	close(release)
	result := <-done
	assert.Len(t, result.Executed, len(candidates))
	assert.Empty(t, result.Skipped)
	assert.LessOrEqual(t, maxInFlight.Load(), int64(2))
}

func TestModelMonitorSchedulerHonorsSiteIntervalAndExponentialBackoff(t *testing.T) {
	now := time.Date(2026, time.July, 25, 11, 30, 0, 0, time.UTC)
	scheduler := NewModelMonitorProbeScheduler(ModelMonitorProbeSchedulerConfig{
		GlobalConcurrency: 2,
		SiteMinInterval:   10 * time.Second,
		BackoffBase:       time.Minute,
		BackoffMax:        16 * time.Minute,
		Now:               func() time.Time { return now },
	})

	assert.False(t, scheduler.ShouldRun(ModelMonitorProbeCandidate{
		SiteID:          1,
		LastSiteProbeAt: now.Add(-9 * time.Second),
	}, now))
	assert.True(t, scheduler.ShouldRun(ModelMonitorProbeCandidate{
		SiteID:          1,
		LastSiteProbeAt: now.Add(-10 * time.Second),
	}, now))

	firstRetry := ModelMonitorProbeCandidate{
		SiteID:                  2,
		LastSiteProbeAt:         now.Add(-time.Minute),
		LastFailureAt:           now.Add(-59 * time.Second),
		ConsecutiveFailureCount: 1,
		LastFailureType:         model.ModelMonitorFailureTypeRateLimited,
	}
	assert.False(t, scheduler.ShouldRun(firstRetry, now))
	firstRetry.LastFailureAt = now.Add(-time.Minute)
	assert.True(t, scheduler.ShouldRun(firstRetry, now))

	secondRetry := ModelMonitorProbeCandidate{
		SiteID:                  3,
		LastSiteProbeAt:         now.Add(-3 * time.Minute),
		LastFailureAt:           now.Add(-119 * time.Second),
		ConsecutiveFailureCount: 2,
		LastFailureType:         model.ModelMonitorFailureTypeTimeout,
	}
	assert.False(t, scheduler.ShouldRun(secondRetry, now))
	secondRetry.LastFailureAt = now.Add(-2 * time.Minute)
	assert.True(t, scheduler.ShouldRun(secondRetry, now))
	assert.Equal(t, 16*time.Minute, scheduler.BackoffDelay(8))

	result := scheduler.Run(context.Background(), []ModelMonitorProbeCandidate{{
		SiteID:          4,
		LastSiteProbeAt: now.Add(-9 * time.Second),
	}}, func(_ context.Context, _ ModelMonitorProbeCandidate) {
		require.Fail(t, "candidate inside site interval must not execute")
	})
	assert.Empty(t, result.Executed)
	assert.Len(t, result.Skipped, 1)
}
