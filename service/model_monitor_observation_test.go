package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelMonitorObservationTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.ModelMonitorSite{},
		&model.ModelMonitorSiteChannel{},
		&model.ModelMonitorTarget{},
		&model.ModelMonitorObservation{},
		&model.ModelMonitorPathState{},
		&model.ModelMonitorAlertOutbox{},
	))

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousAlertSetting := *operation_setting.GetModelMonitorAlertSetting()
	model.DB = db
	model.LOG_DB = db
	*operation_setting.GetModelMonitorAlertSetting() = operation_setting.DefaultModelMonitorAlertSetting()
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		*operation_setting.GetModelMonitorAlertSetting() = previousAlertSetting
	})
	return db
}

func setModelMonitorObservationEnabled(t *testing.T, enabled bool) {
	t.Helper()

	original := *operation_setting.GetModelMonitorSetting()
	t.Cleanup(func() {
		*operation_setting.GetModelMonitorSetting() = original
	})
	operation_setting.GetModelMonitorSetting().Enabled = enabled
}

func createModelMonitorPassivePath(t *testing.T, db *gorm.DB, channelID int, modelName string) (model.ModelMonitorSite, model.ModelMonitorTarget) {
	t.Helper()

	site := model.ModelMonitorSite{Name: "input", SiteType: model.ModelMonitorSiteTypeNewAPI, Enabled: true}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Create(&model.ModelMonitorSiteChannel{SiteID: site.ID, ChannelID: channelID}).Error)
	target := model.ModelMonitorTarget{
		SiteID: site.ID, ModelName: modelName, EndpointType: "openai", Weight: 5, Enabled: true,
	}
	require.NoError(t, db.Create(&target).Error)
	return site, target
}

func modelMonitorPassiveRelayInfo(channelID int, modelName string) *relaycommon.RelayInfo {
	startedAt := time.Now().Add(-900 * time.Millisecond)
	return &relaycommon.RelayInfo{
		OriginModelName:   modelName,
		StartTime:         startedAt,
		FirstResponseTime: startedAt.Add(120 * time.Millisecond),
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         channelID,
			UpstreamModelName: "upstream/" + modelName,
		},
	}
}

func TestModelMonitorPassiveSuccessRecordsBusinessObservation(t *testing.T) {
	db := setupModelMonitorObservationTestDB(t)
	setModelMonitorObservationEnabled(t, true)
	site, target := createModelMonitorPassivePath(t, db, 701, "gpt-5")

	err := RecordModelMonitorPassiveSuccess(modelMonitorPassiveRelayInfo(701, "gpt-5"), &dto.Usage{
		PromptTokens:     17,
		CompletionTokens: 9,
		TotalTokens:      26,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         3,
			CachedCreationTokens: 2,
		},
	})
	require.NoError(t, err)

	var observation model.ModelMonitorObservation
	require.NoError(t, db.First(&observation).Error)
	assert.Equal(t, site.ID, observation.SiteID)
	assert.Equal(t, target.ID, observation.TargetID)
	assert.Equal(t, 701, observation.ChannelID)
	assert.Equal(t, "gpt-5", observation.ModelName)
	assert.Equal(t, "upstream/gpt-5", observation.UpstreamModelName)
	assert.Equal(t, model.ModelMonitorObservationSourcePassive, observation.Source)
	assert.Equal(t, model.ModelMonitorStatusAvailable, observation.Status)
	assert.Equal(t, model.ModelMonitorCostKindUnknown, observation.CostKind)
	assert.Equal(t, 17, observation.PromptTokens)
	assert.Equal(t, 9, observation.CompletionTokens)
	assert.Equal(t, 3, observation.CacheReadTokens)
	assert.Equal(t, 2, observation.CacheCreationTokens)
	require.NotNil(t, observation.FirstResponseMS)
	assert.Equal(t, int64(120), *observation.FirstResponseMS)
	assert.GreaterOrEqual(t, observation.TotalDurationMS, *observation.FirstResponseMS)
}

func TestModelMonitorPassiveUsesUpstreamActualCostWhenProvided(t *testing.T) {
	db := setupModelMonitorObservationTestDB(t)
	setModelMonitorObservationEnabled(t, true)
	createModelMonitorPassivePath(t, db, 704, "gpt-5")

	require.NoError(t, RecordModelMonitorPassiveSuccess(modelMonitorPassiveRelayInfo(704, "gpt-5"), &dto.Usage{
		PromptTokens:     17,
		CompletionTokens: 9,
		TotalTokens:      26,
		Cost:             0.0042,
	}))

	var observation model.ModelMonitorObservation
	require.NoError(t, db.First(&observation).Error)
	assert.Equal(t, model.ModelMonitorCostKindActualUpstream, observation.CostKind)
	assert.EqualValues(t, 4200, observation.CostMicrousd)
	assert.Zero(t, observation.PriceSnapshotID)
}

func TestModelMonitorPassiveDoesNothingWhenDisabled(t *testing.T) {
	db := setupModelMonitorObservationTestDB(t)
	setModelMonitorObservationEnabled(t, false)
	createModelMonitorPassivePath(t, db, 702, "gpt-5")

	require.NoError(t, RecordModelMonitorPassiveSuccess(modelMonitorPassiveRelayInfo(702, "gpt-5"), &dto.Usage{}))

	var count int64
	require.NoError(t, db.Model(&model.ModelMonitorObservation{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestModelMonitorPassiveClassifiesUpstreamHTTPFailures(t *testing.T) {
	db := setupModelMonitorObservationTestDB(t)
	setModelMonitorObservationEnabled(t, true)
	_, target := createModelMonitorPassivePath(t, db, 703, "gpt-5")

	require.NoError(t, RecordModelMonitorPassiveHTTPFailure(modelMonitorPassiveRelayInfo(703, "gpt-5"), 429))

	var observation model.ModelMonitorObservation
	require.NoError(t, db.First(&observation).Error)
	assert.Equal(t, target.ID, observation.TargetID)
	assert.Equal(t, model.ModelMonitorObservationSourcePassive, observation.Source)
	assert.Equal(t, model.ModelMonitorStatusLimited, observation.Status)
	assert.Equal(t, model.ModelMonitorFailureTypeRateLimited, observation.FailureType)
	assert.Nil(t, observation.FirstResponseMS)
}
