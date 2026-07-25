package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelMonitorAggregateTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.ModelMonitorObservation{},
		&model.ModelMonitorAggregateHourly{},
	))
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
	})
	return db
}

func TestRefreshModelMonitorHourlyAggregatesPersistsAvailabilityLatencyFailuresAndCost(t *testing.T) {
	db := setupModelMonitorAggregateTestDB(t)
	hourStart := int64(1_800_000_000)
	first100 := int64(100)
	first300 := int64(300)
	require.NoError(t, db.Create(&[]model.ModelMonitorObservation{
		{
			SiteID: 1, TargetID: 2, ChannelID: 3, ModelName: "gpt-5",
			Status: model.ModelMonitorStatusAvailable, FailureType: model.ModelMonitorFailureTypeNone,
			FirstResponseMS: &first100, TotalDurationMS: 500,
			CostMicrousd: 1000, CostKind: model.ModelMonitorCostKindActualUpstream,
			ObservedAt: hourStart + 10,
		},
		{
			SiteID: 1, TargetID: 2, ChannelID: 3, ModelName: "gpt-5",
			Status: model.ModelMonitorStatusUnavailable, FailureType: model.ModelMonitorFailureTypeTimeout,
			FirstResponseMS: &first300, TotalDurationMS: 900,
			CostMicrousd: 500, CostKind: model.ModelMonitorCostKindEstimatedUpstreamPricing,
			ObservedAt: hourStart + 20,
		},
	}).Error)

	require.NoError(t, RefreshModelMonitorHourlyAggregates(hourStart, hourStart+3600))

	var aggregate model.ModelMonitorAggregateHourly
	require.NoError(t, db.First(&aggregate).Error)
	assert.Equal(t, 2, aggregate.ObservationCount)
	assert.Equal(t, 1, aggregate.AvailableCount)
	assert.Equal(t, 1, aggregate.UnavailableCount)
	assert.Equal(t, 5000, aggregate.AvailabilityBasisPoints)
	require.NotNil(t, aggregate.FirstResponseP95MS)
	assert.EqualValues(t, 300, *aggregate.FirstResponseP95MS)
	assert.EqualValues(t, 900, aggregate.TotalDurationP95MS)
	assert.EqualValues(t, 1500, aggregate.CostMicrousd)
	assert.Equal(t, 1, aggregate.ActualCostCount)
	assert.Equal(t, 1, aggregate.EstimatedCostCount)
	assert.Contains(t, aggregate.FailureCounts, `"timeout":1`)

	require.NoError(t, db.Create(&model.ModelMonitorObservation{
		SiteID: 1, TargetID: 2, ChannelID: 3, ModelName: "gpt-5",
		Status: model.ModelMonitorStatusLimited, FailureType: model.ModelMonitorFailureTypeRateLimited,
		TotalDurationMS: 700, CostKind: model.ModelMonitorCostKindUnknown,
		ObservedAt: hourStart + 30,
	}).Error)
	require.NoError(t, RefreshModelMonitorHourlyAggregates(hourStart, hourStart+3600))
	require.NoError(t, db.First(&aggregate).Error)
	assert.Equal(t, 3, aggregate.ObservationCount)
	assert.Equal(t, 1, aggregate.LimitedCount)
	assert.Equal(t, 1, aggregate.UnknownCostCount)
	assert.Contains(t, aggregate.FailureCounts, `"rate_limited":1`)
}

func TestMaintainModelMonitorAggregatesRetainsHourlyDataAndDeletesRawAfterThirtyDays(t *testing.T) {
	db := setupModelMonitorAggregateTestDB(t)
	now := int64(1_800_000_000)
	oldObservedAt := now - modelMonitorObservationRetentionSeconds - 1
	recentObservedAt := now - 60
	require.NoError(t, db.Create(&[]model.ModelMonitorObservation{
		{
			SiteID: 1, TargetID: 2, ChannelID: 3, ModelName: "gpt-5",
			Status: model.ModelMonitorStatusAvailable, FailureType: model.ModelMonitorFailureTypeNone,
			CostKind: model.ModelMonitorCostKindUnknown, ObservedAt: oldObservedAt,
		},
		{
			SiteID: 1, TargetID: 2, ChannelID: 3, ModelName: "gpt-5",
			Status: model.ModelMonitorStatusAvailable, FailureType: model.ModelMonitorFailureTypeNone,
			CostKind: model.ModelMonitorCostKindUnknown, ObservedAt: recentObservedAt,
		},
	}).Error)

	require.NoError(t, MaintainModelMonitorAggregates(now))

	var observations []model.ModelMonitorObservation
	require.NoError(t, db.Order("observed_at ASC").Find(&observations).Error)
	require.Len(t, observations, 1)
	assert.Equal(t, recentObservedAt, observations[0].ObservedAt)

	var aggregateCount int64
	require.NoError(t, db.Model(&model.ModelMonitorAggregateHourly{}).Count(&aggregateCount).Error)
	assert.Positive(t, aggregateCount)
}
