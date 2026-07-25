package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModelMonitorSettingDefaultsToDisabled(t *testing.T) {
	original := modelMonitorSetting
	t.Cleanup(func() {
		modelMonitorSetting = original
	})

	modelMonitorSetting = DefaultModelMonitorSetting()

	setting := GetModelMonitorSetting()

	assert.False(t, setting.Enabled)
	assert.False(t, setting.AutoProbeEnabled)
	assert.Equal(t, 15, setting.AutoProbeIntervalMinutes)
	assert.Equal(t, 5, setting.UnknownGraceMinutes)
	assert.Empty(t, setting.PricingImportUserIDs)
}

func TestModelMonitorPricingImportUserRequiresExplicitConfiguration(t *testing.T) {
	original := modelMonitorSetting
	t.Cleanup(func() {
		modelMonitorSetting = original
	})

	modelMonitorSetting = DefaultModelMonitorSetting()
	assert.False(t, IsModelMonitorPricingImportUser(42))

	modelMonitorSetting.PricingImportUserIDs = []int{42}
	assert.True(t, IsModelMonitorPricingImportUser(42))
	assert.False(t, IsModelMonitorPricingImportUser(43))
}
