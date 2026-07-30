package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelMonitorTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&ModelMonitorSite{},
		&ModelMonitorSiteChannel{},
		&ModelMonitorTarget{},
		&ModelMonitorPriceSnapshot{},
		&ModelMonitorObservation{},
	))

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})
}

func TestModelMonitorAggregateDeduplicatesPathsAndWeights(t *testing.T) {
	targets := []ModelMonitorTarget{
		{SiteID: 1, ModelName: "gpt-5", Weight: 5, Enabled: true},
		{SiteID: 1, ModelName: "grok-4", Weight: 1, Enabled: true},
	}
	observations := []ModelMonitorObservation{
		{SiteID: 1, ChannelID: 10, ModelName: "gpt-5", Status: ModelMonitorStatusUnavailable},
		{SiteID: 1, ChannelID: 11, ModelName: "gpt-5", Status: ModelMonitorStatusAvailable},
		{SiteID: 1, ChannelID: 12, ModelName: "grok-4", Status: ModelMonitorStatusLimited},
	}

	summary := BuildModelMonitorSiteSummary(targets, observations, 1_000, 300)

	require.Len(t, summary.Models, 2)
	assert.Equal(t, ModelMonitorStatusLimited, summary.Models[0].Status)
	assert.Equal(t, ModelMonitorStatusLimited, summary.Models[1].Status)
	assert.Equal(t, 50, summary.Score)
	assert.Equal(t, ModelMonitorSiteHealthDegraded, summary.Health)
}

func TestModelMonitorAggregateKeepsAllAvailablePathsNormal(t *testing.T) {
	targets := []ModelMonitorTarget{
		{ID: 1, SiteID: 1, ModelName: "gpt-5", Weight: 5, Enabled: true},
	}
	observations := []ModelMonitorObservation{
		{SiteID: 1, TargetID: 1, ChannelID: 10, ModelName: "gpt-5", Status: ModelMonitorStatusAvailable, ObservedAt: 100},
		{SiteID: 1, TargetID: 1, ChannelID: 11, ModelName: "gpt-5", Status: ModelMonitorStatusAvailable, ObservedAt: 200},
	}

	summary := BuildModelMonitorSiteSummary(targets, observations, 250, 300)

	require.Len(t, summary.Models, 1)
	assert.Equal(t, ModelMonitorStatusAvailable, summary.Models[0].Status)
	assert.Equal(t, 100, summary.Score)
	assert.Equal(t, ModelMonitorSiteHealthNormal, summary.Health)
}

func TestModelMonitorAggregateCapsScoreByRecentQuality(t *testing.T) {
	targets := []ModelMonitorTarget{
		{ID: 1, SiteID: 1, ModelName: "gpt-5", Weight: 5, Enabled: true},
	}
	observations := []ModelMonitorObservation{
		{
			SiteID: 1, TargetID: 1, ChannelID: 10, ModelName: "gpt-5",
			Status: ModelMonitorStatusAvailable, ObservedAt: 100,
		},
		{
			SiteID: 1, TargetID: 1, ChannelID: 11, ModelName: "gpt-5",
			Status: ModelMonitorStatusUnavailable, ObservedAt: 200,
		},
		{
			SiteID: 1, TargetID: 1, ChannelID: 11, ModelName: "gpt-5",
			Status: ModelMonitorStatusAvailable, ObservedAt: 300,
		},
		{
			SiteID: 1, TargetID: 1, ChannelID: 11, ModelName: "gpt-5",
			Status: ModelMonitorStatusAvailable, ObservedAt: 400,
		},
	}

	summary := BuildModelMonitorSiteSummary(targets, observations, 500, 300)

	require.Len(t, summary.Models, 1)
	assert.Equal(t, ModelMonitorStatusAvailable, summary.Models[0].Status)
	assert.Equal(t, 75, summary.Score)
	assert.Equal(t, ModelMonitorSiteHealthDegraded, summary.Health)
}

func TestModelMonitorAggregateExcludesOldFailuresFromRecentQuality(t *testing.T) {
	const now int64 = 100_000
	targets := []ModelMonitorTarget{
		{ID: 1, SiteID: 1, ModelName: "gpt-5", Weight: 5, Enabled: true},
	}
	observations := []ModelMonitorObservation{
		{
			SiteID: 1, TargetID: 1, ChannelID: 10, ModelName: "gpt-5",
			Status: ModelMonitorStatusUnavailable, ObservedAt: now - 24*60*60 - 1,
		},
		{
			SiteID: 1, TargetID: 1, ChannelID: 10, ModelName: "gpt-5",
			Status: ModelMonitorStatusAvailable, ObservedAt: now - 100,
		},
		{
			SiteID: 1, TargetID: 1, ChannelID: 10, ModelName: "gpt-5",
			Status: ModelMonitorStatusAvailable, ObservedAt: now - 50,
		},
	}

	summary := BuildModelMonitorSiteSummary(targets, observations, now, 300)

	require.Len(t, summary.Models, 1)
	assert.Equal(t, ModelMonitorStatusAvailable, summary.Models[0].Status)
	assert.Equal(t, 100, summary.Score)
	assert.Equal(t, ModelMonitorSiteHealthNormal, summary.Health)
}

func TestModelMonitorAggregateTreatsExpiredUnknownAsLimited(t *testing.T) {
	targets := []ModelMonitorTarget{
		{SiteID: 1, ModelName: "gpt-5", Weight: 2, Enabled: true},
	}
	observations := []ModelMonitorObservation{
		{
			SiteID:      1,
			ChannelID:   10,
			ModelName:   "gpt-5",
			Status:      ModelMonitorStatusUnknown,
			ObservedAt:  600,
			Source:      ModelMonitorObservationSourceActive,
			FailureType: ModelMonitorFailureTypeNone,
		},
	}

	summary := BuildModelMonitorSiteSummary(targets, observations, 1_000, 300)

	require.Len(t, summary.Models, 1)
	assert.True(t, summary.Models[0].Stale)
	assert.Equal(t, 50, summary.Score)
	assert.Equal(t, ModelMonitorSiteHealthDegraded, summary.Health)
}

func TestModelMonitorAggregateAppliesPathHysteresis(t *testing.T) {
	targets := []ModelMonitorTarget{
		{ID: 1, SiteID: 1, ModelName: "gpt-5", Weight: 5, Enabled: true},
	}
	base := []ModelMonitorObservation{
		{
			SiteID: 1, TargetID: 1, ChannelID: 10, ModelName: "gpt-5",
			Status: ModelMonitorStatusAvailable, FailureType: ModelMonitorFailureTypeNone, ObservedAt: 100,
		},
	}

	oneFailure := append(append([]ModelMonitorObservation{}, base...), ModelMonitorObservation{
		SiteID: 1, TargetID: 1, ChannelID: 10, ModelName: "gpt-5",
		Status: ModelMonitorStatusUnavailable, FailureType: ModelMonitorFailureTypeTimeout, ObservedAt: 200,
	})
	summary := BuildModelMonitorSiteSummary(targets, oneFailure, 250, 300)
	require.Len(t, summary.Models, 1)
	assert.Equal(t, ModelMonitorStatusLimited, summary.Models[0].Status)
	assert.Equal(t, ModelMonitorStatusUnavailable, summary.Models[0].LatestStatus)
	assert.Equal(t, ModelMonitorFailureTypeTimeout, summary.Models[0].LatestFailureType)
	assert.Equal(t, 50, summary.Score)
	assert.Equal(t, ModelMonitorSiteHealthDegraded, summary.Health)

	twoFailures := append(oneFailure, ModelMonitorObservation{
		SiteID: 1, TargetID: 1, ChannelID: 10, ModelName: "gpt-5",
		Status: ModelMonitorStatusUnavailable, FailureType: ModelMonitorFailureTypeConnection, ObservedAt: 300,
	})
	summary = BuildModelMonitorSiteSummary(targets, twoFailures, 350, 300)
	assert.Equal(t, ModelMonitorStatusLimited, summary.Models[0].Status)

	threeFailures := append(twoFailures,
		ModelMonitorObservation{
			SiteID: 1, TargetID: 1, ChannelID: 10, ModelName: "gpt-5",
			Status: ModelMonitorStatusUnavailable, FailureType: ModelMonitorFailureTypeUpstreamServer, ObservedAt: 400,
		},
	)
	summary = BuildModelMonitorSiteSummary(targets, threeFailures, 450, 300)
	assert.Equal(t, ModelMonitorStatusUnavailable, summary.Models[0].Status)

	oneRecovery := append(threeFailures, ModelMonitorObservation{
		SiteID: 1, TargetID: 1, ChannelID: 10, ModelName: "gpt-5",
		Status: ModelMonitorStatusAvailable, FailureType: ModelMonitorFailureTypeNone, ObservedAt: 500,
	})
	summary = BuildModelMonitorSiteSummary(targets, oneRecovery, 550, 300)
	assert.Equal(t, ModelMonitorStatusUnavailable, summary.Models[0].Status)

	twoRecoveries := append(oneRecovery, ModelMonitorObservation{
		SiteID: 1, TargetID: 1, ChannelID: 10, ModelName: "gpt-5",
		Status: ModelMonitorStatusAvailable, FailureType: ModelMonitorFailureTypeNone, ObservedAt: 600,
	})
	summary = BuildModelMonitorSiteSummary(targets, twoRecoveries, 650, 300)
	assert.Equal(t, ModelMonitorStatusAvailable, summary.Models[0].Status)
}

func TestModelMonitorAggregateDerivesLimitedImmediatelyPerPath(t *testing.T) {
	targets := []ModelMonitorTarget{
		{ID: 1, SiteID: 1, ModelName: "gpt-5", Weight: 5, Enabled: true},
	}
	observations := []ModelMonitorObservation{
		{
			SiteID: 1, TargetID: 1, ChannelID: 10, ModelName: "gpt-5",
			Status: ModelMonitorStatusAvailable, FailureType: ModelMonitorFailureTypeNone, ObservedAt: 100,
		},
		{
			SiteID: 1, TargetID: 1, ChannelID: 10, ModelName: "gpt-5",
			Status: ModelMonitorStatusLimited, FailureType: ModelMonitorFailureTypeRateLimited, ObservedAt: 200,
		},
	}

	summary := BuildModelMonitorSiteSummary(targets, observations, 250, 300)
	require.Len(t, summary.Models, 1)
	assert.Equal(t, ModelMonitorStatusLimited, summary.Models[0].Status)
	assert.Equal(t, 50, summary.Score)
}

func TestModelMonitorObservationKeepsPriceSnapshotReference(t *testing.T) {
	setupModelMonitorTestDB(t)

	site := ModelMonitorSite{Name: "input", SiteType: ModelMonitorSiteTypeNewAPI}
	require.NoError(t, DB.Create(&site).Error)

	target := ModelMonitorTarget{
		SiteID:    site.ID,
		ModelName: "gpt-5",
		Weight:    3,
		Enabled:   true,
	}
	require.NoError(t, DB.Create(&target).Error)

	snapshot := ModelMonitorPriceSnapshot{
		SiteID:      site.ID,
		ModelName:   "gpt-5",
		Source:      ModelMonitorPriceSourceUpstreamCatalog,
		Version:     "2026-07-25T08:00:00Z",
		CapturedAt:  common.GetTimestamp(),
		PricingData: `{"input_price":1.5,"group_ratio":0.1}`,
	}
	require.NoError(t, DB.Create(&snapshot).Error)

	observation := ModelMonitorObservation{
		SiteID:          site.ID,
		ChannelID:       7,
		TargetID:        target.ID,
		ModelName:       target.ModelName,
		Status:          ModelMonitorStatusAvailable,
		Source:          ModelMonitorObservationSourceActive,
		PriceSnapshotID: snapshot.ID,
		ObservedAt:      common.GetTimestamp(),
	}
	require.NoError(t, DB.Create(&observation).Error)

	require.NoError(t, DB.Model(&ModelMonitorPriceSnapshot{}).
		Where("id = ?", snapshot.ID).
		Update("pricing_data", `{"input_price":2.0,"group_ratio":0.2}`).Error)

	var stored ModelMonitorObservation
	require.NoError(t, DB.First(&stored, observation.ID).Error)
	assert.Equal(t, snapshot.ID, stored.PriceSnapshotID)
}

func TestValidateModelMonitorTarget(t *testing.T) {
	valid := ModelMonitorTarget{
		SiteID:    1,
		ModelName: "gpt-5",
		Weight:    3,
		Enabled:   true,
	}
	require.NoError(t, ValidateModelMonitorTarget(valid))

	invalidWeight := valid
	invalidWeight.Weight = 6
	assert.Error(t, ValidateModelMonitorTarget(invalidWeight))

	missingModel := valid
	missingModel.ModelName = ""
	assert.Error(t, ValidateModelMonitorTarget(missingModel))
}

func TestListEnabledModelMonitorProbePathsUsesConfirmedEnabledSiteRelations(t *testing.T) {
	setupModelMonitorTestDB(t)

	enabledSite := ModelMonitorSite{Name: "input", SiteType: ModelMonitorSiteTypeNewAPI, Enabled: true}
	disabledSite := ModelMonitorSite{Name: "disabled", SiteType: ModelMonitorSiteTypeNewAPI, Enabled: false}
	require.NoError(t, DB.Create(&enabledSite).Error)
	require.NoError(t, DB.Create(&disabledSite).Error)
	require.NoError(t, DB.Create(&ModelMonitorSiteChannel{SiteID: enabledSite.ID, ChannelID: 101}).Error)
	require.NoError(t, DB.Create(&ModelMonitorSiteChannel{SiteID: disabledSite.ID, ChannelID: 102}).Error)
	require.NoError(t, DB.Create(&ModelMonitorTarget{
		SiteID: enabledSite.ID, ModelName: "gpt-5", EndpointType: "openai", Weight: 5, Enabled: true,
	}).Error)
	require.NoError(t, DB.Create(&ModelMonitorTarget{
		SiteID: enabledSite.ID, ModelName: "grok-4", EndpointType: "openai", Weight: 1, Enabled: false,
	}).Error)
	require.NoError(t, DB.Create(&ModelMonitorTarget{
		SiteID: disabledSite.ID, ModelName: "gpt-5", EndpointType: "openai", Weight: 5, Enabled: true,
	}).Error)

	paths, err := ListEnabledModelMonitorProbePaths()
	require.NoError(t, err)
	require.Len(t, paths, 1)
	assert.Equal(t, enabledSite.ID, paths[0].SiteID)
	assert.Equal(t, 101, paths[0].ChannelID)
	assert.Equal(t, "gpt-5", paths[0].ModelName)
}

func TestModelMonitorProbeScheduleStateUsesActiveObservationsOnly(t *testing.T) {
	setupModelMonitorTestDB(t)

	site := ModelMonitorSite{Name: "input", SiteType: ModelMonitorSiteTypeNewAPI, Enabled: true}
	require.NoError(t, DB.Create(&site).Error)
	target := ModelMonitorTarget{SiteID: site.ID, ModelName: "gpt-5", Weight: 5, Enabled: true}
	require.NoError(t, DB.Create(&target).Error)
	observations := []ModelMonitorObservation{
		{
			SiteID: site.ID, TargetID: target.ID, ChannelID: 7, ModelName: target.ModelName,
			Status: ModelMonitorStatusAvailable, Source: ModelMonitorObservationSourceActive,
			FailureType: ModelMonitorFailureTypeNone, ObservedAt: 100,
		},
		{
			SiteID: site.ID, TargetID: target.ID, ChannelID: 7, ModelName: target.ModelName,
			Status: ModelMonitorStatusUnavailable, Source: ModelMonitorObservationSourceActive,
			FailureType: ModelMonitorFailureTypeTimeout, ObservedAt: 200,
		},
		{
			SiteID: site.ID, TargetID: target.ID, ChannelID: 7, ModelName: target.ModelName,
			Status: ModelMonitorStatusLimited, Source: ModelMonitorObservationSourceActive,
			FailureType: ModelMonitorFailureTypeRateLimited, ObservedAt: 300,
		},
		{
			SiteID: site.ID, TargetID: target.ID, ChannelID: 7, ModelName: target.ModelName,
			Status: ModelMonitorStatusUnavailable, Source: ModelMonitorObservationSourcePassive,
			FailureType: ModelMonitorFailureTypeUpstreamServer, ObservedAt: 400,
		},
	}
	for _, observation := range observations {
		require.NoError(t, DB.Create(&observation).Error)
	}

	state, err := GetModelMonitorProbeScheduleState(site.ID, target.ID, 7)
	require.NoError(t, err)
	assert.Equal(t, int64(300), state.LastSiteProbeAt)
	assert.Equal(t, int64(300), state.LastFailureAt)
	assert.Equal(t, ModelMonitorFailureTypeRateLimited, state.LastFailureType)
	assert.Equal(t, 2, state.ConsecutiveFailureCount)
}
