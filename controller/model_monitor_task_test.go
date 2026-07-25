package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelMonitorHandlerIsDisabledUntilBothSwitchesAreEnabled(t *testing.T) {
	original := *operation_setting.GetModelMonitorSetting()
	t.Cleanup(func() {
		*operation_setting.GetModelMonitorSetting() = original
	})

	*operation_setting.GetModelMonitorSetting() = operation_setting.ModelMonitorSetting{
		Enabled:                  true,
		AutoProbeEnabled:         false,
		AutoProbeIntervalMinutes: 15,
		UnknownGraceMinutes:      5,
	}
	assert.False(t, modelMonitorHandler{}.Enabled())

	operation_setting.GetModelMonitorSetting().AutoProbeEnabled = true
	assert.True(t, modelMonitorHandler{}.Enabled())
}

func TestModelMonitorTaskCountsConfiguredTargetsAndMergesManualRuns(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.SystemTask{},
		&model.SystemTaskLock{},
		&model.ModelMonitorSite{},
		&model.ModelMonitorSiteChannel{},
		&model.ModelMonitorTarget{},
		&model.ModelMonitorObservation{},
		&model.Channel{},
	))

	site := model.ModelMonitorSite{Name: "input", SiteType: model.ModelMonitorSiteTypeNewAPI}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Create(&model.ModelMonitorTarget{
		SiteID: site.ID, ModelName: "gpt-5", Weight: 5, Enabled: true,
	}).Error)
	require.NoError(t, db.Create(&model.ModelMonitorTarget{
		SiteID: site.ID, ModelName: "grok-4", Weight: 1, Enabled: false,
	}).Error)

	summary, err := runModelMonitorTask(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Targets)

	first, created, err := enqueueModelMonitorRun()
	require.NoError(t, err)
	assert.True(t, created)
	require.NotNil(t, first)
	assert.Equal(t, model.SystemTaskTypeModelMonitor, first.Type)

	second, created, err := enqueueModelMonitorRun()
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.TaskID, second.TaskID)
}

func TestModelMonitorTaskProbesConfirmedChannelPathsAndPersistsObservations(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.ModelMonitorSite{},
		&model.ModelMonitorSiteChannel{},
		&model.ModelMonitorTarget{},
		&model.ModelMonitorObservation{},
	))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(server.Close)

	channel := modelMonitorProbeTestChannel(server.URL)
	channel.Id = 801
	channel.Type = constant.ChannelTypeOpenAI
	require.NoError(t, db.Create(channel).Error)

	site := model.ModelMonitorSite{Name: "input", SiteType: model.ModelMonitorSiteTypeNewAPI, Enabled: true}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Create(&model.ModelMonitorSiteChannel{SiteID: site.ID, ChannelID: channel.Id}).Error)
	target := model.ModelMonitorTarget{
		SiteID: site.ID, ModelName: "gpt-5", EndpointType: string(constant.EndpointTypeOpenAI), Weight: 5, Enabled: true,
	}
	require.NoError(t, db.Create(&target).Error)

	summary, err := runModelMonitorTask(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Targets)
	assert.Equal(t, 1, summary.Paths)

	var observation model.ModelMonitorObservation
	require.NoError(t, db.First(&observation).Error)
	assert.Equal(t, site.ID, observation.SiteID)
	assert.Equal(t, target.ID, observation.TargetID)
	assert.Equal(t, channel.Id, observation.ChannelID)
	assert.Equal(t, model.ModelMonitorStatusAvailable, observation.Status)
	assert.Equal(t, model.ModelMonitorObservationSourceActive, observation.Source)
}
