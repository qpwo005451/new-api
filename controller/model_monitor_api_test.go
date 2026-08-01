package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelMonitorAPITestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf(
		"file:%s?mode=memory&cache=shared",
		strings.ReplaceAll(t.Name(), "/", "_"),
	)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Option{},
		&model.Channel{},
		&model.ModelMonitorSite{},
		&model.ModelMonitorSiteChannel{},
		&model.ModelMonitorTarget{},
		&model.ModelMonitorPriceSnapshot{},
		&model.ModelMonitorObservation{},
		&model.ModelMonitorAggregateHourly{},
		&model.ModelMonitorPathState{},
		&model.ModelMonitorAlertOutbox{},
		&model.SystemTask{},
		&model.SystemTaskLock{},
	))

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousSetting := *operation_setting.GetModelMonitorSetting()
	previousAlertSetting := *operation_setting.GetModelMonitorAlertSetting()
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		*operation_setting.GetModelMonitorSetting() = previousSetting
		*operation_setting.GetModelMonitorAlertSetting() = previousAlertSetting
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestModelMonitorAlertConfigNeverReturnsStoredTelegramToken(t *testing.T) {
	setupModelMonitorAPITestDB(t)
	setting := operation_setting.GetModelMonitorAlertSetting()
	setting.Enabled = true
	setting.TelegramEnabled = true
	setting.TelegramNotifyBotToken = "stored-notification-token"
	setting.TelegramChatID = "12345"

	config := loadModelMonitorAlertConfig()
	assert.True(t, config.TelegramBotTokenConfigured)
	assert.Empty(t, config.TelegramBotToken)
}

func TestSaveModelMonitorAlertConfigPreservesTokenWhenRequestLeavesItEmpty(t *testing.T) {
	db := setupModelMonitorAPITestDB(t)
	site := model.ModelMonitorSite{Name: "input", SiteType: model.ModelMonitorSiteTypeNewAPI, Enabled: true}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Create(&model.ModelMonitorSiteChannel{SiteID: site.ID, ChannelID: 9}).Error)
	operation_setting.GetModelMonitorAlertSetting().TelegramNotifyBotToken = "stored-notification-token"

	request := modelMonitorAlertConfig{
		Enabled:         true,
		EmailEnabled:    true,
		EmailRecipients: "ops@example.com",
		TelegramEnabled: true,
		TelegramChatID:  "12345",
		Rules: []operation_setting.ModelMonitorAlertRule{
			{SiteID: site.ID, ChannelID: 9, ModelPrefix: "gpt-", Enabled: true},
			{SiteID: site.ID, ChannelID: 9, ModelName: "kimi-k2.7-code", Enabled: true},
		},
	}
	require.NoError(t, saveModelMonitorAlertConfig(request))

	setting := operation_setting.GetModelMonitorAlertSetting()
	assert.Equal(t, "stored-notification-token", setting.TelegramNotifyBotToken)
	assert.Equal(t, "ops@example.com", setting.EmailRecipients)
	require.Len(t, setting.Rules, 2)

	var tokenOption model.Option
	require.NoError(t, db.Where("key = ?", "model_monitor_alert_setting.TelegramNotifyBotToken").First(&tokenOption).Error)
	assert.Equal(t, "stored-notification-token", tokenOption.Value)
	var rulesOption model.Option
	require.NoError(t, db.Where("key = ?", "model_monitor_alert_setting.rules").First(&rulesOption).Error)
	assert.Contains(t, rulesOption.Value, "kimi-k2.7-code")
}

func TestValidateModelMonitorAlertConfigRequiresUsableFocusedRule(t *testing.T) {
	config := modelMonitorAlertConfig{
		Enabled:         true,
		EmailEnabled:    true,
		EmailRecipients: "ops@example.com",
		Rules: []operation_setting.ModelMonitorAlertRule{
			{SiteID: 2, ChannelID: 9, Enabled: true},
		},
	}
	assert.ErrorContains(t, validateModelMonitorAlertConfig(config, ""), "model selector")

	config.Rules[0].ModelPrefix = "gpt-"
	require.NoError(t, validateModelMonitorAlertConfig(config, ""))
}

func TestModelMonitorSiteSummaryUsesPathHysteresis(t *testing.T) {
	db := setupModelMonitorAPITestDB(t)
	site := model.ModelMonitorSite{Name: "input", SiteType: model.ModelMonitorSiteTypeNewAPI, Enabled: true}
	require.NoError(t, db.Create(&site).Error)
	channel := model.Channel{Name: "input", Models: "gpt-5", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	incompatibleChannel := model.Channel{Name: "grok-only", Models: "grok-4.5", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&incompatibleChannel).Error)
	require.NoError(t, db.Create(&model.ModelMonitorSiteChannel{SiteID: site.ID, ChannelID: channel.Id}).Error)
	require.NoError(t, db.Create(&model.ModelMonitorSiteChannel{SiteID: site.ID, ChannelID: incompatibleChannel.Id}).Error)
	target := model.ModelMonitorTarget{SiteID: site.ID, ModelName: "gpt-5", Weight: 5, Enabled: true}
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&[]model.ModelMonitorObservation{
		{
			SiteID: site.ID, TargetID: target.ID, ChannelID: channel.Id, ModelName: target.ModelName,
			Status: model.ModelMonitorStatusAvailable, Source: model.ModelMonitorObservationSourcePassive,
			FailureType: model.ModelMonitorFailureTypeNone, ObservedAt: 100,
		},
		{
			SiteID: site.ID, TargetID: target.ID, ChannelID: channel.Id, ModelName: target.ModelName,
			Status: model.ModelMonitorStatusUnavailable, Source: model.ModelMonitorObservationSourceActive,
			FailureType: model.ModelMonitorFailureTypeTimeout, ObservedAt: 200,
		},
		{
			SiteID: site.ID, TargetID: target.ID, ChannelID: incompatibleChannel.Id, ModelName: target.ModelName,
			Status: model.ModelMonitorStatusUnavailable, Source: model.ModelMonitorObservationSourceActive,
			FailureType: model.ModelMonitorFailureTypeModelNotFound, ObservedAt: 300,
		},
	}).Error)

	response, err := buildModelMonitorSiteResponse(site.ID, true)
	require.NoError(t, err)
	assert.Equal(t, model.ModelMonitorSiteHealthDegraded, response.Summary.Health)
	require.Len(t, response.Summary.Models, 1)
	assert.Equal(t, model.ModelMonitorStatusLimited, response.Summary.Models[0].Status)
	assert.Equal(t, 50, response.Summary.Score)
	assert.EqualValues(t, 200, response.LatestObservedAt)
	require.Len(t, response.Observations, 2)
}

func TestGetModelMonitorModelFiltersUnsupportedChannelHistory(t *testing.T) {
	db := setupModelMonitorAPITestDB(t)
	site := model.ModelMonitorSite{Name: "input", SiteType: model.ModelMonitorSiteTypeNewAPI, Enabled: true}
	require.NoError(t, db.Create(&site).Error)
	supportedChannel := model.Channel{Name: "gpt", Models: "gpt-5", Status: common.ChannelStatusEnabled}
	unsupportedChannel := model.Channel{Name: "grok", Models: "grok-4.5", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&supportedChannel).Error)
	require.NoError(t, db.Create(&unsupportedChannel).Error)
	require.NoError(t, db.Create(&model.ModelMonitorSiteChannel{SiteID: site.ID, ChannelID: supportedChannel.Id}).Error)
	require.NoError(t, db.Create(&model.ModelMonitorSiteChannel{SiteID: site.ID, ChannelID: unsupportedChannel.Id}).Error)
	target := model.ModelMonitorTarget{SiteID: site.ID, ModelName: "gpt-5", Weight: 5, Enabled: true}
	require.NoError(t, db.Create(&target).Error)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&[]model.ModelMonitorObservation{
		{
			SiteID: site.ID, TargetID: target.ID, ChannelID: supportedChannel.Id, ModelName: target.ModelName,
			Status: model.ModelMonitorStatusAvailable, Source: model.ModelMonitorObservationSourcePassive,
			FailureType: model.ModelMonitorFailureTypeNone, ObservedAt: now - 60,
		},
		{
			SiteID: site.ID, TargetID: target.ID, ChannelID: unsupportedChannel.Id, ModelName: target.ModelName,
			Status: model.ModelMonitorStatusUnavailable, Source: model.ModelMonitorObservationSourceActive,
			FailureType: model.ModelMonitorFailureTypeModelNotFound, ObservedAt: now,
		},
	}).Error)
	require.NoError(t, db.Create(&[]model.ModelMonitorAggregateHourly{
		{
			SiteID: site.ID, TargetID: target.ID, ChannelID: supportedChannel.Id, ModelName: target.ModelName,
			HourStart: now - now%3600, ObservationCount: 1, AvailableCount: 1, AvailabilityBasisPoints: 10_000,
		},
		{
			SiteID: site.ID, TargetID: target.ID, ChannelID: unsupportedChannel.Id, ModelName: target.ModelName,
			HourStart: now - now%3600, ObservationCount: 1, UnavailableCount: 1,
		},
	}).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{
		{Key: "site_id", Value: fmt.Sprintf("%d", site.ID)},
		{Key: "model", Value: target.ModelName},
	}
	GetModelMonitorModel(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Observations []model.ModelMonitorObservation     `json:"observations"`
			Aggregates   []model.ModelMonitorAggregateHourly `json:"aggregates"`
			Summary      model.ModelMonitorSiteSummary       `json:"summary"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	require.Len(t, response.Data.Observations, 1)
	assert.Equal(t, supportedChannel.Id, response.Data.Observations[0].ChannelID)
	require.Len(t, response.Data.Aggregates, 1)
	assert.Equal(t, supportedChannel.Id, response.Data.Aggregates[0].ChannelID)
	require.Len(t, response.Data.Summary.Models, 1)
	assert.Equal(t, model.ModelMonitorStatusAvailable, response.Data.Summary.Models[0].Status)
}

func TestSaveModelMonitorConfigPreservesHistoryAndDisablesRemovedEntities(t *testing.T) {
	db := setupModelMonitorAPITestDB(t)
	channel := model.Channel{Name: "input-channel", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	site := model.ModelMonitorSite{Name: "input", SiteType: model.ModelMonitorSiteTypeNewAPI, Enabled: true}
	removedSite := model.ModelMonitorSite{Name: "removed", SiteType: model.ModelMonitorSiteTypeNewAPI, Enabled: true}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Create(&removedSite).Error)
	require.NoError(t, db.Create(&model.ModelMonitorSiteChannel{
		SiteID: removedSite.ID, ChannelID: channel.Id,
	}).Error)
	target := model.ModelMonitorTarget{SiteID: site.ID, ModelName: "gpt-5", Weight: 5, Enabled: true}
	removedTarget := model.ModelMonitorTarget{SiteID: site.ID, ModelName: "grok-4", Weight: 3, Enabled: true}
	removedSiteTarget := model.ModelMonitorTarget{
		SiteID: removedSite.ID, ModelName: "claude-4", Weight: 2, Enabled: true,
	}
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&removedTarget).Error)
	require.NoError(t, db.Create(&removedSiteTarget).Error)
	observation := model.ModelMonitorObservation{
		SiteID: site.ID, TargetID: target.ID, ChannelID: channel.Id, ModelName: target.ModelName,
		Status: model.ModelMonitorStatusAvailable, Source: model.ModelMonitorObservationSourcePassive,
		FailureType: model.ModelMonitorFailureTypeNone,
	}
	require.NoError(t, db.Create(&observation).Error)

	request := modelMonitorConfigResponse{
		Setting: operation_setting.ModelMonitorSetting{
			Enabled:                  true,
			AutoProbeEnabled:         false,
			AutoProbeIntervalMinutes: 15,
			UnknownGraceMinutes:      5,
			PricingImportUserIDs:     []int{42},
		},
		Sites: []modelMonitorSiteConfig{
			{
				ID: site.ID, Name: site.Name, SiteType: site.SiteType, PricingGroup: "plus", Enabled: true,
				ChannelIDs: []int{channel.Id},
				Targets: []modelMonitorTargetConfig{
					{ID: target.ID, ModelName: target.ModelName, EndpointType: "openai", Weight: 4, Enabled: true},
				},
			},
		},
	}
	require.NoError(t, saveModelMonitorConfig(request))

	var storedSite model.ModelMonitorSite
	require.NoError(t, db.First(&storedSite, site.ID).Error)
	assert.Equal(t, "plus", storedSite.PricingGroup)
	assert.True(t, storedSite.Enabled)
	var storedRemovedSite model.ModelMonitorSite
	require.NoError(t, db.First(&storedRemovedSite, removedSite.ID).Error)
	assert.False(t, storedRemovedSite.Enabled)
	var removedSiteChannelCount int64
	require.NoError(t, db.Model(&model.ModelMonitorSiteChannel{}).
		Where("site_id = ?", removedSite.ID).
		Count(&removedSiteChannelCount).Error)
	assert.Zero(t, removedSiteChannelCount)

	var storedTarget model.ModelMonitorTarget
	require.NoError(t, db.First(&storedTarget, target.ID).Error)
	assert.Equal(t, 4, storedTarget.Weight)
	var storedRemovedTarget model.ModelMonitorTarget
	require.NoError(t, db.First(&storedRemovedTarget, removedTarget.ID).Error)
	assert.False(t, storedRemovedTarget.Enabled)
	var storedRemovedSiteTarget model.ModelMonitorTarget
	require.NoError(t, db.First(&storedRemovedSiteTarget, removedSiteTarget.ID).Error)
	assert.False(t, storedRemovedSiteTarget.Enabled)

	var storedObservation model.ModelMonitorObservation
	require.NoError(t, db.First(&storedObservation, observation.ID).Error)
	assert.Equal(t, target.ID, storedObservation.TargetID)
	assert.True(t, operation_setting.GetModelMonitorSetting().Enabled)
	assert.Equal(t, []int{42}, operation_setting.GetModelMonitorSetting().PricingImportUserIDs)

	var enabledOption model.Option
	require.NoError(t, db.Where("key = ?", "model_monitor_setting.enabled").First(&enabledOption).Error)
	assert.Equal(t, "true", enabledOption.Value)

	loaded, err := loadModelMonitorConfig()
	require.NoError(t, err)
	require.Len(t, loaded.Sites, 1)
	assert.Equal(t, site.ID, loaded.Sites[0].ID)
}

func TestValidateModelMonitorConfigRejectsChannelAssignedToMultipleSites(t *testing.T) {
	config := modelMonitorConfigResponse{
		Setting: operation_setting.ModelMonitorSetting{
			AutoProbeIntervalMinutes: 15,
			UnknownGraceMinutes:      5,
		},
		Sites: []modelMonitorSiteConfig{
			{Name: "site-a", SiteType: model.ModelMonitorSiteTypeNewAPI, ChannelIDs: []int{10}},
			{Name: "site-b", SiteType: model.ModelMonitorSiteTypeSub2API, ChannelIDs: []int{10}},
		},
	}

	assert.ErrorContains(t, validateModelMonitorConfig(config), "already assigned")
}

func TestEnqueueModelMonitorRunRequiresEnabledMonitorAndMergesActiveTask(t *testing.T) {
	setupModelMonitorAPITestDB(t)
	operation_setting.GetModelMonitorSetting().Enabled = false

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/model-monitor/runs", nil)
	EnqueueModelMonitorRun(context)
	assert.Equal(t, http.StatusConflict, recorder.Code)

	operation_setting.GetModelMonitorSetting().Enabled = true
	first, created, err := enqueueModelMonitorRun()
	require.NoError(t, err)
	assert.True(t, created)
	require.NotNil(t, first)
	second, created, err := enqueueModelMonitorRun()
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.TaskID, second.TaskID)
}
