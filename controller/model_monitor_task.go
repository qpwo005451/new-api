package controller

import (
	"context"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

type modelMonitorHandler struct{}

func (modelMonitorHandler) Type() string {
	return model.SystemTaskTypeModelMonitor
}

func (modelMonitorHandler) Enabled() bool {
	return operation_setting.GetModelMonitorSetting().Enabled
}

func (modelMonitorHandler) Interval() time.Duration {
	return time.Duration(operation_setting.GetModelMonitorSetting().AutoProbeIntervalMinutes) * time.Minute
}

func (modelMonitorHandler) NewPayload() any {
	return modelMonitorTaskPayload{}
}

type modelMonitorTaskPayload struct {
	Manual bool `json:"manual,omitempty"`
}

type modelMonitorTaskSummary struct {
	Targets      int `json:"targets"`
	Paths        int `json:"paths"`
	ProbedPaths  int `json:"probed_paths"`
	SkippedPaths int `json:"skipped_paths"`
}

func (modelMonitorHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := modelMonitorTaskPayload{}
	if err := task.DecodePayload(&payload); err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	if !payload.Manual && !operation_setting.GetModelMonitorSetting().AutoProbeEnabled {
		if err := service.MaintainModelMonitorAggregates(common.GetTimestamp()); err != nil {
			finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
			return
		}
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, modelMonitorTaskSummary{}, nil)
		return
	}

	summary, err := runModelMonitorTask(ctx, service.NewSystemTaskProgressReporter(task, runnerID))
	if err != nil {
		finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, nil, err)
		return
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, summary, nil)
}

func enqueueModelMonitorRun() (*model.SystemTask, bool, error) {
	return service.EnqueueSystemTask(model.SystemTaskTypeModelMonitor, modelMonitorTaskPayload{Manual: true})
}

func runModelMonitorTask(ctx context.Context, report func(processed, total int)) (modelMonitorTaskSummary, error) {
	targetCount, err := model.CountEnabledModelMonitorTargets()
	if err != nil {
		return modelMonitorTaskSummary{}, err
	}
	if ctx != nil && ctx.Err() != nil {
		return modelMonitorTaskSummary{}, ctx.Err()
	}

	summary := modelMonitorTaskSummary{Targets: int(targetCount)}
	paths, err := model.ListEnabledModelMonitorProbePaths()
	if err != nil {
		return modelMonitorTaskSummary{}, err
	}
	eligiblePaths := make([]model.ModelMonitorProbePath, 0, len(paths))
	channelsByID := make(map[int]*model.Channel, len(paths))
	for _, path := range paths {
		channel, exists := channelsByID[path.ChannelID]
		if !exists {
			channel, err = model.GetChannelById(path.ChannelID, true)
			if err == nil {
				channelsByID[path.ChannelID] = channel
			}
		}
		if channel != nil && !channel.SupportsModel(path.ModelName) {
			continue
		}
		eligiblePaths = append(eligiblePaths, path)
	}

	summary.Paths = len(eligiblePaths)
	candidates := make([]service.ModelMonitorProbeCandidate, 0, len(eligiblePaths))
	pathsByTargetAndChannel := make(map[[2]int64]model.ModelMonitorProbePath, len(eligiblePaths))
	for _, path := range eligiblePaths {
		state, err := model.GetModelMonitorProbeScheduleState(path.SiteID, path.TargetID, path.ChannelID)
		if err != nil {
			return modelMonitorTaskSummary{}, err
		}
		candidates = append(candidates, service.ModelMonitorProbeCandidate{
			SiteID:                  path.SiteID,
			TargetID:                path.TargetID,
			ChannelID:               path.ChannelID,
			LastSiteProbeAt:         time.Unix(state.LastSiteProbeAt, 0),
			LastFailureAt:           time.Unix(state.LastFailureAt, 0),
			ConsecutiveFailureCount: state.ConsecutiveFailureCount,
			LastFailureType:         state.LastFailureType,
		})
		pathsByTargetAndChannel[[2]int64{path.TargetID, int64(path.ChannelID)}] = path
	}

	var progressMutex sync.Mutex
	processed := 0
	scheduleResult := service.DefaultModelMonitorProbeScheduler().Run(ctx, candidates, func(probeContext context.Context, candidate service.ModelMonitorProbeCandidate) {
		path := pathsByTargetAndChannel[[2]int64{candidate.TargetID, int64(candidate.ChannelID)}]
		observation := model.ModelMonitorObservation{
			SiteID:            path.SiteID,
			TargetID:          path.TargetID,
			ChannelID:         path.ChannelID,
			ModelName:         path.ModelName,
			UpstreamModelName: path.ModelName,
			Source:            model.ModelMonitorObservationSourceActive,
			Status:            model.ModelMonitorStatusUnknown,
			FailureType:       model.ModelMonitorFailureTypeConfiguration,
			ErrorSummary:      "model monitor probe preparation failed",
			CostKind:          model.ModelMonitorCostKindUnknown,
			ObservedAt:        common.GetTimestamp(),
		}
		channel := channelsByID[path.ChannelID]
		if channel != nil {
			target := model.ModelMonitorTarget{
				ID:           path.TargetID,
				SiteID:       path.SiteID,
				ModelName:    path.ModelName,
				EndpointType: path.EndpointType,
				Weight:       path.Weight,
				Enabled:      true,
			}
			probeObservation, probeErr := runModelMonitorProbe(probeContext, channel, target)
			if probeErr == nil {
				observation = probeObservation
			} else {
				common.SysError("model monitor probe preparation failed")
			}
		} else {
			common.SysError("model monitor probe channel lookup failed")
		}
		if err := service.RecordModelMonitorObservation(&observation); err != nil {
			common.SysError("model monitor observation persistence failed")
		}

		progressMutex.Lock()
		processed++
		if report != nil {
			report(processed, len(eligiblePaths))
		}
		progressMutex.Unlock()
	})
	summary.ProbedPaths = len(scheduleResult.Executed)
	summary.SkippedPaths = len(scheduleResult.Skipped)
	if len(eligiblePaths) == 0 && report != nil {
		report(0, 0)
	}
	if err := service.MaintainModelMonitorAggregates(common.GetTimestamp()); err != nil {
		return summary, err
	}
	return summary, nil
}
