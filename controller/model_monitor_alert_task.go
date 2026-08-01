package controller

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

type modelMonitorAlertHandler struct{}

func (modelMonitorAlertHandler) Type() string {
	return model.SystemTaskTypeModelMonitorAlert
}

func (modelMonitorAlertHandler) Enabled() bool {
	if !operation_setting.GetModelMonitorAlertSetting().Enabled {
		return false
	}
	hasDue, err := model.HasDueModelMonitorAlertOutbox(common.GetTimestamp())
	if err != nil {
		common.SysError("model monitor alert outbox lookup failed")
		return false
	}
	return hasDue
}

func (modelMonitorAlertHandler) Interval() time.Duration {
	return 15 * time.Second
}

func (modelMonitorAlertHandler) NewPayload() any {
	return nil
}

func (modelMonitorAlertHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	total := service.ModelMonitorAlertDispatchSummary{}
	for {
		summary, err := service.DispatchDueModelMonitorAlerts(ctx, runnerID, common.GetTimestamp())
		total.Claimed += summary.Claimed
		total.Sent += summary.Sent
		total.Retrying += summary.Retrying
		total.Dead += summary.Dead
		if err != nil {
			finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusFailed, total, err)
			return
		}
		if summary.Claimed < 20 {
			break
		}
	}
	finishSystemTaskHandler(task, runnerID, model.SystemTaskStatusSucceeded, total, nil)
}
