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
		&ModelMonitorPathState{},
		&ModelMonitorAlertOutbox{},
	))

	previousDB := DB
	DB = db
	t.Cleanup(func() {
		DB = previousDB
	})
}

func TestRecordModelMonitorObservationQueuesOnlyStableUnavailableAndRecoveryTransitions(t *testing.T) {
	setupModelMonitorTestDB(t)

	site := ModelMonitorSite{Name: "input", SiteType: ModelMonitorSiteTypeNewAPI, Enabled: true}
	require.NoError(t, DB.Create(&site).Error)
	target := ModelMonitorTarget{SiteID: site.ID, ModelName: "gpt-5.6-luna", Weight: 5, Enabled: true}
	require.NoError(t, DB.Create(&target).Error)

	record := func(status string, observedAt int64) {
		t.Helper()
		_, err := RecordModelMonitorObservation(&ModelMonitorObservation{
			SiteID: site.ID, TargetID: target.ID, ChannelID: 9, ModelName: target.ModelName,
			Status: status, Source: ModelMonitorObservationSourceActive,
			FailureType: ModelMonitorFailureTypeTimeout, ObservedAt: observedAt,
		}, []string{ModelMonitorAlertTransportEmail, ModelMonitorAlertTransportTelegram})
		require.NoError(t, err)
	}

	record(ModelMonitorStatusAvailable, 100)
	record(ModelMonitorStatusUnavailable, 200)
	record(ModelMonitorStatusUnavailable, 300)

	var state ModelMonitorPathState
	require.NoError(t, DB.First(&state).Error)
	assert.Equal(t, ModelMonitorStatusLimited, state.Status)
	assert.Equal(t, 2, state.ConsecutiveFailures)

	var outboxCount int64
	require.NoError(t, DB.Model(&ModelMonitorAlertOutbox{}).Count(&outboxCount).Error)
	assert.Zero(t, outboxCount)

	record(ModelMonitorStatusUnavailable, 400)
	require.NoError(t, DB.First(&state).Error)
	assert.Equal(t, ModelMonitorStatusUnavailable, state.Status)
	assert.Equal(t, int64(3), state.TransitionVersion)
	require.NoError(t, DB.Model(&ModelMonitorAlertOutbox{}).Count(&outboxCount).Error)
	assert.EqualValues(t, 2, outboxCount)

	record(ModelMonitorStatusUnavailable, 500)
	require.NoError(t, DB.Model(&ModelMonitorAlertOutbox{}).Count(&outboxCount).Error)
	assert.EqualValues(t, 2, outboxCount)

	record(ModelMonitorStatusAvailable, 600)
	require.NoError(t, DB.First(&state).Error)
	assert.Equal(t, ModelMonitorStatusUnavailable, state.Status)
	record(ModelMonitorStatusAvailable, 700)
	require.NoError(t, DB.First(&state).Error)
	assert.Equal(t, ModelMonitorStatusAvailable, state.Status)
	assert.Equal(t, int64(4), state.TransitionVersion)
	require.NoError(t, DB.Model(&ModelMonitorAlertOutbox{}).Count(&outboxCount).Error)
	assert.EqualValues(t, 4, outboxCount)

	var events []ModelMonitorAlertOutbox
	require.NoError(t, DB.Order("id asc").Find(&events).Error)
	assert.Equal(t, ModelMonitorStatusUnavailable, events[0].Status)
	assert.Equal(t, ModelMonitorStatusUnavailable, events[1].Status)
	assert.Equal(t, ModelMonitorStatusAvailable, events[2].Status)
	assert.Equal(t, ModelMonitorStatusAvailable, events[3].Status)
	assert.NotEqual(t, events[0].EventKey, events[2].EventKey)
}

func TestRecordModelMonitorObservationPersistsStateAndOutboxAtomically(t *testing.T) {
	setupModelMonitorTestDB(t)

	site := ModelMonitorSite{Name: "input", SiteType: ModelMonitorSiteTypeNewAPI, Enabled: true}
	require.NoError(t, DB.Create(&site).Error)
	target := ModelMonitorTarget{SiteID: site.ID, ModelName: "gpt-5.6-sol", Weight: 5, Enabled: true}
	require.NoError(t, DB.Create(&target).Error)
	state := ModelMonitorPathState{
		SiteID: site.ID, TargetID: target.ID, ChannelID: 9, ModelName: target.ModelName,
		Status: ModelMonitorStatusLimited, ConsecutiveFailures: 2, TransitionVersion: 1,
	}
	require.NoError(t, DB.Create(&state).Error)
	require.NoError(t, DB.Create(&ModelMonitorAlertOutbox{
		EventKey: "model-monitor:1:1:9:2:email", Transport: ModelMonitorAlertTransportEmail,
		Status: ModelMonitorStatusUnavailable, DeliveryStatus: ModelMonitorAlertDeliveryPending,
	}).Error)

	_, err := RecordModelMonitorObservation(&ModelMonitorObservation{
		SiteID: site.ID, TargetID: target.ID, ChannelID: 9, ModelName: target.ModelName,
		Status: ModelMonitorStatusUnavailable, Source: ModelMonitorObservationSourceActive,
		FailureType: ModelMonitorFailureTypeTimeout, ObservedAt: 400,
	}, []string{ModelMonitorAlertTransportEmail})
	require.Error(t, err)

	var observationCount int64
	require.NoError(t, DB.Model(&ModelMonitorObservation{}).Count(&observationCount).Error)
	assert.Zero(t, observationCount)
	require.NoError(t, DB.First(&state).Error)
	assert.Equal(t, ModelMonitorStatusLimited, state.Status)
	assert.Equal(t, 2, state.ConsecutiveFailures)
}

func TestClaimModelMonitorAlertOutboxSupportsRetryAndLeaseRecovery(t *testing.T) {
	setupModelMonitorTestDB(t)
	require.NoError(t, DB.Create(&ModelMonitorAlertOutbox{
		EventKey: "event-1", Transport: ModelMonitorAlertTransportTelegram,
		Status: ModelMonitorStatusUnavailable, DeliveryStatus: ModelMonitorAlertDeliveryPending,
		NextAttemptAt: 100,
	}).Error)

	claimed, err := ClaimDueModelMonitorAlertOutbox(100, 160, "runner-a", 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, 1, claimed[0].Attempts)
	assert.Equal(t, "runner-a", claimed[0].ClaimedBy)

	claimed, err = ClaimDueModelMonitorAlertOutbox(120, 180, "runner-b", 10)
	require.NoError(t, err)
	assert.Empty(t, claimed)

	claimed, err = ClaimDueModelMonitorAlertOutbox(161, 221, "runner-b", 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, 2, claimed[0].Attempts)

	require.NoError(t, RetryModelMonitorAlertOutbox(claimed[0].ID, "runner-b", 300, "temporary failure", 5))
	claimed, err = ClaimDueModelMonitorAlertOutbox(299, 359, "runner-c", 10)
	require.NoError(t, err)
	assert.Empty(t, claimed)
	claimed, err = ClaimDueModelMonitorAlertOutbox(300, 360, "runner-c", 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.NoError(t, CompleteModelMonitorAlertOutbox(claimed[0].ID, "runner-c", 305))

	var stored ModelMonitorAlertOutbox
	require.NoError(t, DB.First(&stored, claimed[0].ID).Error)
	assert.Equal(t, ModelMonitorAlertDeliverySent, stored.DeliveryStatus)
	assert.Equal(t, int64(305), stored.SentAt)
}

func TestQueueDueModelMonitorTelegramRepeatsDeduplicatesIntervalAndStopsAfterRecovery(t *testing.T) {
	setupModelMonitorTestDB(t)
	state := ModelMonitorPathState{
		SiteID: 2, TargetID: 7, ChannelID: 9, ModelName: "gpt-5.6-sol",
		Status: ModelMonitorStatusUnavailable, LastTransitionAt: 100,
		TransitionVersion: 3, LastFailureType: ModelMonitorFailureTypeTimeout,
	}
	require.NoError(t, DB.Create(&state).Error)

	matches := func(siteID int64, channelID int, modelName string) bool {
		return siteID == 2 && channelID == 9 && modelName == "gpt-5.6-sol"
	}
	created, err := QueueDueModelMonitorTelegramRepeats(999, 900, matches)
	require.NoError(t, err)
	assert.Zero(t, created)

	created, err = QueueDueModelMonitorTelegramRepeats(1000, 900, matches)
	require.NoError(t, err)
	assert.Equal(t, 1, created)
	created, err = QueueDueModelMonitorTelegramRepeats(1099, 900, matches)
	require.NoError(t, err)
	assert.Zero(t, created)
	created, err = QueueDueModelMonitorTelegramRepeats(1899, 900, matches)
	require.NoError(t, err)
	assert.Zero(t, created)
	created, err = QueueDueModelMonitorTelegramRepeats(1901, 900, matches)
	require.NoError(t, err)
	assert.Equal(t, 1, created)

	require.NoError(t, DB.Model(&state).Update("status", ModelMonitorStatusAvailable).Error)
	created, err = QueueDueModelMonitorTelegramRepeats(2800, 900, matches)
	require.NoError(t, err)
	assert.Zero(t, created)
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
