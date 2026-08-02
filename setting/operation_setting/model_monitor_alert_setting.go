package operation_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type ModelMonitorAlertRule struct {
	SiteID      int64  `json:"site_id"`
	ChannelID   int    `json:"channel_id"`
	ModelPrefix string `json:"model_prefix,omitempty"`
	ModelName   string `json:"model_name,omitempty"`
	Enabled     bool   `json:"enabled"`
}

type ModelMonitorAlertSetting struct {
	Enabled                bool                    `json:"enabled"`
	EmailEnabled           bool                    `json:"email_enabled"`
	EmailRecipients        string                  `json:"email_recipients"`
	TelegramEnabled        bool                    `json:"telegram_enabled"`
	TelegramRepeatEnabled  bool                    `json:"telegram_repeat_enabled"`
	TelegramRepeatMinutes  int                     `json:"telegram_repeat_minutes"`
	TelegramNotifyBotToken string                  `json:"TelegramNotifyBotToken"`
	TelegramChatID         string                  `json:"telegram_chat_id"`
	Rules                  []ModelMonitorAlertRule `json:"rules"`
}

func DefaultModelMonitorAlertSetting() ModelMonitorAlertSetting {
	return ModelMonitorAlertSetting{
		TelegramRepeatMinutes: 15,
		Rules:                 []ModelMonitorAlertRule{},
	}
}

var modelMonitorAlertSetting = DefaultModelMonitorAlertSetting()

func init() {
	config.GlobalConfig.Register("model_monitor_alert_setting", &modelMonitorAlertSetting)
}

func GetModelMonitorAlertSetting() *ModelMonitorAlertSetting {
	return &modelMonitorAlertSetting
}

func (setting *ModelMonitorAlertSetting) Matches(siteID int64, channelID int, modelName string) bool {
	if setting == nil || !setting.Enabled {
		return false
	}
	for _, rule := range setting.Rules {
		if !rule.Enabled || rule.SiteID != siteID || rule.ChannelID != channelID {
			continue
		}
		if exact := strings.TrimSpace(rule.ModelName); exact != "" && modelName == exact {
			return true
		}
		if prefix := strings.TrimSpace(rule.ModelPrefix); prefix != "" && strings.HasPrefix(modelName, prefix) {
			return true
		}
	}
	return false
}

func (setting *ModelMonitorAlertSetting) EnabledTransports(siteID int64, channelID int, modelName string) []string {
	if !setting.Matches(siteID, channelID, modelName) {
		return nil
	}
	transports := make([]string, 0, 2)
	if setting.EmailEnabled && strings.TrimSpace(setting.EmailRecipients) != "" {
		transports = append(transports, "email")
	}
	if setting.TelegramEnabled &&
		strings.TrimSpace(setting.TelegramNotifyBotToken) != "" &&
		strings.TrimSpace(setting.TelegramChatID) != "" {
		transports = append(transports, "telegram")
	}
	return transports
}
