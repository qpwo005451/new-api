package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelMonitorAlertSettingMatchesFocusedPrefixAndExactModel(t *testing.T) {
	setting := ModelMonitorAlertSetting{
		Enabled: true,
		Rules: []ModelMonitorAlertRule{
			{SiteID: 2, ChannelID: 9, ModelPrefix: "gpt-", Enabled: true},
			{SiteID: 2, ChannelID: 9, ModelName: "kimi-k2.7-code", Enabled: true},
		},
	}

	assert.True(t, setting.Matches(2, 9, "gpt-5.6-luna"))
	assert.True(t, setting.Matches(2, 9, "kimi-k2.7-code"))
	assert.False(t, setting.Matches(2, 9, "kimi-k2.7"))
	assert.False(t, setting.Matches(2, 10, "gpt-5.6-luna"))
	assert.False(t, setting.Matches(3, 9, "gpt-5.6-luna"))
}

func TestModelMonitorAlertSettingReturnsOnlyConfiguredTransports(t *testing.T) {
	setting := ModelMonitorAlertSetting{
		Enabled:         true,
		EmailEnabled:    true,
		EmailRecipients: "ops@example.com",
		TelegramEnabled: true,
		TelegramChatID:  "12345",
		Rules: []ModelMonitorAlertRule{
			{SiteID: 2, ChannelID: 9, ModelPrefix: "gpt-", Enabled: true},
		},
	}

	assert.Equal(t, []string{"email"}, setting.EnabledTransports(2, 9, "gpt-5.6-sol"))
	setting.TelegramNotifyBotToken = "notify-token"
	assert.Equal(t, []string{"email", "telegram"}, setting.EnabledTransports(2, 9, "gpt-5.6-sol"))
	assert.Empty(t, setting.EnabledTransports(2, 9, "grok-4.5"))
}

func TestDefaultModelMonitorAlertSettingUsesFifteenMinuteRepeatInterval(t *testing.T) {
	setting := DefaultModelMonitorAlertSetting()

	assert.Equal(t, 15, setting.TelegramRepeatMinutes)
	assert.False(t, setting.TelegramRepeatEnabled)
}

func TestGetModelMonitorAlertSettingNormalizesMissingRepeatInterval(t *testing.T) {
	previous := modelMonitorAlertSetting
	modelMonitorAlertSetting = ModelMonitorAlertSetting{}
	t.Cleanup(func() { modelMonitorAlertSetting = previous })

	setting := GetModelMonitorAlertSetting()

	assert.Equal(t, 15, setting.TelegramRepeatMinutes)
}
