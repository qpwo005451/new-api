package service

import (
	"context"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const inFlightUsageLogSweepInterval = time.Minute

func StartInFlightUsageLogSweepTask() {
	if !model.InFlightUsageLogSupported() {
		common.SysLog("in-flight usage log sweep disabled: log database does not support pending updates")
		return
	}
	go func() {
		time.Sleep(30 * time.Second)
		ticker := time.NewTicker(inFlightUsageLogSweepInterval)
		defer ticker.Stop()
		for range ticker.C {
			n, err := model.FinalizeStaleInFlightLogs(context.Background(), int64(common.InFlightUsageLogStaleSeconds), 200)
			if err != nil {
				common.SysError("failed to finalize stale in-flight usage logs: " + err.Error())
				continue
			}
			if n > 0 {
				common.SysLog("finalized stale in-flight usage logs: " + strconv.FormatInt(n, 10))
			}
		}
	}()
}
