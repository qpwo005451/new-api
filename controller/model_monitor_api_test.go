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
		&model.SystemTask{},
		&model.SystemTaskLock{},
	))

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousSetting := *operation_setting.GetModelMonitorSetting()
	model.DB = db
	model.LOG_DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		*operation_setting.GetModelMonitorSetting() = previousSetting
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestModelMonitorSiteSummaryUsesPathHysteresis(t *testing.T) {
	db := setupModelMonitorAPITestDB(t)
	site := model.ModelMonitorSite{Name: "input", SiteType: model.ModelMonitorSiteTypeNewAPI, Enabled: true}
	require.NoError(t, db.Create(&site).Error)
	target := model.ModelMonitorTarget{SiteID: site.ID, ModelName: "gpt-5", Weight: 5, Enabled: true}
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&[]model.ModelMonitorObservation{
		{
			SiteID: site.ID, TargetID: target.ID, ChannelID: 10, ModelName: target.ModelName,
			Status: model.ModelMonitorStatusAvailable, Source: model.ModelMonitorObservationSourcePassive,
			FailureType: model.ModelMonitorFailureTypeNone, ObservedAt: 100,
		},
		{
			SiteID: site.ID, TargetID: target.ID, ChannelID: 10, ModelName: target.ModelName,
			Status: model.ModelMonitorStatusUnavailable, Source: model.ModelMonitorObservationSourceActive,
			FailureType: model.ModelMonitorFailureTypeTimeout, ObservedAt: 200,
		},
	}).Error)

	response, err := buildModelMonitorSiteResponse(site.ID, true)
	require.NoError(t, err)
	assert.Equal(t, model.ModelMonitorSiteHealthNormal, response.Summary.Health)
	require.Len(t, response.Summary.Models, 1)
	assert.Equal(t, model.ModelMonitorStatusAvailable, response.Summary.Models[0].Status)
	assert.EqualValues(t, 200, response.LatestObservedAt)
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
