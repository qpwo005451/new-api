package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

type ModelMonitorSetting struct {
	Enabled                  bool  `json:"enabled"`
	AutoProbeEnabled         bool  `json:"auto_probe_enabled"`
	AutoProbeIntervalMinutes int   `json:"auto_probe_interval_minutes"`
	UnknownGraceMinutes      int   `json:"unknown_grace_minutes"`
	PricingImportUserIDs     []int `json:"pricing_import_user_ids"`
}

func DefaultModelMonitorSetting() ModelMonitorSetting {
	return ModelMonitorSetting{
		Enabled:                  false,
		AutoProbeEnabled:         false,
		AutoProbeIntervalMinutes: 15,
		UnknownGraceMinutes:      5,
		PricingImportUserIDs:     []int{},
	}
}

var modelMonitorSetting = DefaultModelMonitorSetting()

func init() {
	config.GlobalConfig.Register("model_monitor_setting", &modelMonitorSetting)
}

func GetModelMonitorSetting() *ModelMonitorSetting {
	if modelMonitorSetting.AutoProbeIntervalMinutes < 1 {
		modelMonitorSetting.AutoProbeIntervalMinutes = DefaultModelMonitorSetting().AutoProbeIntervalMinutes
	}
	if modelMonitorSetting.UnknownGraceMinutes < 1 {
		modelMonitorSetting.UnknownGraceMinutes = DefaultModelMonitorSetting().UnknownGraceMinutes
	}
	return &modelMonitorSetting
}

func IsModelMonitorPricingImportUser(userID int) bool {
	for _, configuredUserID := range GetModelMonitorSetting().PricingImportUserIDs {
		if configuredUserID == userID {
			return true
		}
	}
	return false
}
