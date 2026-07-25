package service

import (
	"errors"
	"math"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	modelMonitorObservationRetentionSeconds = int64(30 * 24 * 60 * 60)
	modelMonitorAggregateHourSeconds        = int64(60 * 60)
	modelMonitorObservationCleanupBatchSize = 1000
)

type modelMonitorAggregateKey struct {
	SiteID    int64
	TargetID  int64
	ChannelID int
	HourStart int64
}

type modelMonitorAggregateAccumulator struct {
	modelName          string
	observations       int
	available          int
	limited            int
	unavailable        int
	unknown            int
	firstResponseMS    []int64
	totalDurationMS    []int64
	costMicrousd       decimal.Decimal
	actualCostCount    int
	estimatedCostCount int
	unknownCostCount   int
	failureCounts      map[string]int
}

func RefreshModelMonitorHourlyAggregates(startTimestamp, endTimestamp int64) error {
	if endTimestamp <= startTimestamp {
		return errors.New("model monitor aggregate range is invalid")
	}
	startTimestamp = modelMonitorHourStart(startTimestamp)
	endTimestamp = modelMonitorHourStart(endTimestamp-1) + modelMonitorAggregateHourSeconds

	observations := make([]model.ModelMonitorObservation, 0)
	if err := model.DB.Where("observed_at >= ? AND observed_at < ?", startTimestamp, endTimestamp).
		Order("observed_at ASC, id ASC").
		Find(&observations).Error; err != nil {
		return err
	}
	if len(observations) == 0 {
		return nil
	}

	accumulators := make(map[modelMonitorAggregateKey]*modelMonitorAggregateAccumulator)
	for _, observation := range observations {
		key := modelMonitorAggregateKey{
			SiteID:    observation.SiteID,
			TargetID:  observation.TargetID,
			ChannelID: observation.ChannelID,
			HourStart: modelMonitorHourStart(observation.ObservedAt),
		}
		accumulator := accumulators[key]
		if accumulator == nil {
			accumulator = &modelMonitorAggregateAccumulator{
				modelName:     observation.ModelName,
				costMicrousd:  decimal.Zero,
				failureCounts: make(map[string]int),
			}
			accumulators[key] = accumulator
		}
		accumulator.observations++
		switch observation.Status {
		case model.ModelMonitorStatusAvailable:
			accumulator.available++
		case model.ModelMonitorStatusLimited:
			accumulator.limited++
		case model.ModelMonitorStatusUnavailable:
			accumulator.unavailable++
		default:
			accumulator.unknown++
		}
		if observation.FirstResponseMS != nil {
			accumulator.firstResponseMS = append(accumulator.firstResponseMS, *observation.FirstResponseMS)
		}
		accumulator.totalDurationMS = append(accumulator.totalDurationMS, observation.TotalDurationMS)
		accumulator.costMicrousd = accumulator.costMicrousd.Add(decimal.NewFromInt(observation.CostMicrousd))
		switch observation.CostKind {
		case model.ModelMonitorCostKindActualUpstream:
			accumulator.actualCostCount++
		case model.ModelMonitorCostKindEstimatedUpstreamPricing:
			accumulator.estimatedCostCount++
		default:
			accumulator.unknownCostCount++
		}
		if observation.FailureType != "" && observation.FailureType != model.ModelMonitorFailureTypeNone {
			accumulator.failureCounts[observation.FailureType]++
		}
	}

	now := common.GetTimestamp()
	aggregates := make([]model.ModelMonitorAggregateHourly, 0, len(accumulators))
	for key, accumulator := range accumulators {
		if accumulator.costMicrousd.IsNegative() || accumulator.costMicrousd.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
			return errors.New("model monitor aggregate cost is invalid")
		}
		failureCounts, err := common.Marshal(accumulator.failureCounts)
		if err != nil {
			return err
		}
		aggregate := model.ModelMonitorAggregateHourly{
			SiteID:                  key.SiteID,
			TargetID:                key.TargetID,
			ChannelID:               key.ChannelID,
			ModelName:               accumulator.modelName,
			HourStart:               key.HourStart,
			ObservationCount:        accumulator.observations,
			AvailableCount:          accumulator.available,
			LimitedCount:            accumulator.limited,
			UnavailableCount:        accumulator.unavailable,
			UnknownCount:            accumulator.unknown,
			AvailabilityBasisPoints: accumulator.available * 10_000 / accumulator.observations,
			TotalDurationP95MS:      modelMonitorP95(accumulator.totalDurationMS),
			CostMicrousd:            accumulator.costMicrousd.IntPart(),
			ActualCostCount:         accumulator.actualCostCount,
			EstimatedCostCount:      accumulator.estimatedCostCount,
			UnknownCostCount:        accumulator.unknownCostCount,
			FailureCounts:           string(failureCounts),
			UpdatedAt:               now,
		}
		if len(accumulator.firstResponseMS) > 0 {
			firstResponseP95 := modelMonitorP95(accumulator.firstResponseMS)
			aggregate.FirstResponseP95MS = &firstResponseP95
		}
		aggregates = append(aggregates, aggregate)
	}

	return model.DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "site_id"},
			{Name: "target_id"},
			{Name: "channel_id"},
			{Name: "hour_start"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"model_name",
			"observation_count",
			"available_count",
			"limited_count",
			"unavailable_count",
			"unknown_count",
			"availability_basis_points",
			"first_response_p95_ms",
			"total_duration_p95_ms",
			"cost_microusd",
			"actual_cost_count",
			"estimated_cost_count",
			"unknown_cost_count",
			"failure_counts",
			"updated_at",
		}),
	}).Create(&aggregates).Error
}

func MaintainModelMonitorAggregates(now int64) error {
	if now <= 0 {
		return errors.New("model monitor maintenance timestamp is invalid")
	}
	var latestAggregateHour int64
	if err := model.DB.Model(&model.ModelMonitorAggregateHourly{}).
		Select("COALESCE(MAX(hour_start), 0)").
		Scan(&latestAggregateHour).Error; err != nil {
		return err
	}

	startTimestamp := latestAggregateHour
	if startTimestamp == 0 {
		var oldestObservation model.ModelMonitorObservation
		err := model.DB.Order("observed_at ASC, id ASC").First(&oldestObservation).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if oldestObservation.ID != 0 {
			startTimestamp = modelMonitorHourStart(oldestObservation.ObservedAt)
		}
	}
	if startTimestamp > 0 {
		endTimestamp := modelMonitorHourStart(now) + modelMonitorAggregateHourSeconds
		if err := RefreshModelMonitorHourlyAggregates(startTimestamp, endTimestamp); err != nil {
			return err
		}
	}

	cutoff := now - modelMonitorObservationRetentionSeconds
	for {
		ids := make([]int64, 0, modelMonitorObservationCleanupBatchSize)
		if err := model.DB.Model(&model.ModelMonitorObservation{}).
			Where("observed_at < ?", cutoff).
			Order("observed_at ASC, id ASC").
			Limit(modelMonitorObservationCleanupBatchSize).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		if err := model.DB.Where("id IN ?", ids).Delete(&model.ModelMonitorObservation{}).Error; err != nil {
			return err
		}
		if len(ids) < modelMonitorObservationCleanupBatchSize {
			return nil
		}
	}
}

func modelMonitorHourStart(timestamp int64) int64 {
	return timestamp - timestamp%modelMonitorAggregateHourSeconds
}

func modelMonitorP95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	ordered := append([]int64(nil), values...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i] < ordered[j]
	})
	index := (95*len(ordered)+99)/100 - 1
	return ordered[index]
}
