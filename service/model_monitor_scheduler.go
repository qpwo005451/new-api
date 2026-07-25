package service

import (
	"context"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
)

const (
	modelMonitorProbeDefaultGlobalConcurrency = 2
	// Observations persist second-resolution timestamps. Eleven seconds keeps
	// the actual interval at or above the required ten seconds across tasks.
	modelMonitorProbeDefaultSiteMinInterval = 11 * time.Second
	modelMonitorProbeDefaultBackoffBase     = time.Minute
	modelMonitorProbeDefaultBackoffMax      = 30 * time.Minute
)

type ModelMonitorProbeSchedulerConfig struct {
	GlobalConcurrency int
	SiteMinInterval   time.Duration
	BackoffBase       time.Duration
	BackoffMax        time.Duration
	Now               func() time.Time
}

type ModelMonitorProbeCandidate struct {
	SiteID                  int64
	TargetID                int64
	ChannelID               int
	LastSiteProbeAt         time.Time
	LastFailureAt           time.Time
	ConsecutiveFailureCount int
	LastFailureType         string
}

type ModelMonitorProbeScheduleResult struct {
	Executed []ModelMonitorProbeCandidate
	Skipped  []ModelMonitorProbeCandidate
}

type ModelMonitorProbeScheduler struct {
	config ModelMonitorProbeSchedulerConfig
}

func DefaultModelMonitorProbeScheduler() *ModelMonitorProbeScheduler {
	return NewModelMonitorProbeScheduler(ModelMonitorProbeSchedulerConfig{
		GlobalConcurrency: modelMonitorProbeDefaultGlobalConcurrency,
		SiteMinInterval:   modelMonitorProbeDefaultSiteMinInterval,
		BackoffBase:       modelMonitorProbeDefaultBackoffBase,
		BackoffMax:        modelMonitorProbeDefaultBackoffMax,
	})
}

func NewModelMonitorProbeScheduler(config ModelMonitorProbeSchedulerConfig) *ModelMonitorProbeScheduler {
	if config.GlobalConcurrency <= 0 {
		config.GlobalConcurrency = modelMonitorProbeDefaultGlobalConcurrency
	}
	if config.BackoffBase <= 0 {
		config.BackoffBase = modelMonitorProbeDefaultBackoffBase
	}
	if config.BackoffMax < config.BackoffBase {
		config.BackoffMax = modelMonitorProbeDefaultBackoffMax
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &ModelMonitorProbeScheduler{config: config}
}

func (scheduler *ModelMonitorProbeScheduler) ShouldRun(candidate ModelMonitorProbeCandidate, now time.Time) bool {
	if scheduler == nil {
		return false
	}
	if !candidate.LastSiteProbeAt.IsZero() && now.Before(candidate.LastSiteProbeAt.Add(scheduler.config.SiteMinInterval)) {
		return false
	}
	if candidate.ConsecutiveFailureCount <= 0 || candidate.LastFailureAt.IsZero() || !isRetryableModelMonitorFailure(candidate.LastFailureType) {
		return true
	}
	return !now.Before(candidate.LastFailureAt.Add(scheduler.BackoffDelay(candidate.ConsecutiveFailureCount)))
}

func (scheduler *ModelMonitorProbeScheduler) BackoffDelay(consecutiveFailureCount int) time.Duration {
	if scheduler == nil || consecutiveFailureCount <= 0 {
		return 0
	}
	delay := scheduler.config.BackoffBase
	for failureIndex := 1; failureIndex < consecutiveFailureCount; failureIndex++ {
		if delay >= scheduler.config.BackoffMax/2 {
			return scheduler.config.BackoffMax
		}
		delay *= 2
	}
	if delay > scheduler.config.BackoffMax {
		return scheduler.config.BackoffMax
	}
	return delay
}

func (scheduler *ModelMonitorProbeScheduler) Run(ctx context.Context, candidates []ModelMonitorProbeCandidate, execute func(context.Context, ModelMonitorProbeCandidate)) ModelMonitorProbeScheduleResult {
	if scheduler == nil {
		return ModelMonitorProbeScheduleResult{Skipped: candidates}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	now := scheduler.config.Now()
	siteCandidates := make(map[int64][]ModelMonitorProbeCandidate)
	siteOrder := make([]int64, 0)
	siteNextStart := make(map[int64]time.Time)
	result := ModelMonitorProbeScheduleResult{
		Executed: make([]ModelMonitorProbeCandidate, 0, len(candidates)),
		Skipped:  make([]ModelMonitorProbeCandidate, 0),
	}
	for _, candidate := range candidates {
		if !scheduler.ShouldRun(candidate, now) {
			result.Skipped = append(result.Skipped, candidate)
			continue
		}
		if _, exists := siteCandidates[candidate.SiteID]; !exists {
			siteOrder = append(siteOrder, candidate.SiteID)
		}
		siteCandidates[candidate.SiteID] = append(siteCandidates[candidate.SiteID], candidate)
		if nextStart := candidate.LastSiteProbeAt.Add(scheduler.config.SiteMinInterval); nextStart.After(siteNextStart[candidate.SiteID]) {
			siteNextStart[candidate.SiteID] = nextStart
		}
	}

	globalSlots := make(chan struct{}, scheduler.config.GlobalConcurrency)
	var resultMutex sync.Mutex
	var workers sync.WaitGroup
	for _, siteID := range siteOrder {
		candidatesForSite := siteCandidates[siteID]
		nextStart := siteNextStart[siteID]
		workers.Add(1)
		go func() {
			defer workers.Done()
			for _, candidate := range candidatesForSite {
				if delay := nextStart.Sub(scheduler.config.Now()); delay > 0 {
					timer := time.NewTimer(delay)
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
				}

				select {
				case <-ctx.Done():
					return
				case globalSlots <- struct{}{}:
				}
				startedAt := scheduler.config.Now()
				resultMutex.Lock()
				result.Executed = append(result.Executed, candidate)
				resultMutex.Unlock()
				if execute != nil {
					execute(ctx, candidate)
				}
				<-globalSlots
				nextStart = startedAt.Add(scheduler.config.SiteMinInterval)
			}
		}()
	}
	workers.Wait()
	return result
}

func isRetryableModelMonitorFailure(failureType string) bool {
	switch failureType {
	case model.ModelMonitorFailureTypeRateLimited,
		model.ModelMonitorFailureTypeTimeout,
		model.ModelMonitorFailureTypeUpstreamServer,
		model.ModelMonitorFailureTypeConnection,
		model.ModelMonitorFailureTypeStreamBreak:
		return true
	default:
		return false
	}
}
