package model

import (
	"errors"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	ModelMonitorSiteTypeNewAPI  = "newapi"
	ModelMonitorSiteTypeSub2API = "sub2api"

	ModelMonitorStatusAvailable   = "available"
	ModelMonitorStatusLimited     = "limited"
	ModelMonitorStatusUnavailable = "unavailable"
	ModelMonitorStatusUnknown     = "unknown"

	ModelMonitorObservationSourceActive  = "active"
	ModelMonitorObservationSourcePassive = "passive"

	ModelMonitorFailureTypeNone           = "none"
	ModelMonitorFailureTypeUnauthorized   = "unauthorized"
	ModelMonitorFailureTypeRateLimited    = "rate_limited"
	ModelMonitorFailureTypeUpstreamServer = "upstream_server"
	ModelMonitorFailureTypeModelNotFound  = "model_not_found"
	ModelMonitorFailureTypeTimeout        = "timeout"
	ModelMonitorFailureTypeStreamBreak    = "stream_break"
	ModelMonitorFailureTypeConnection     = "connection"
	ModelMonitorFailureTypeInvalidStream  = "invalid_stream"
	ModelMonitorFailureTypeCancelled      = "cancelled"
	ModelMonitorFailureTypeConfiguration  = "configuration"

	ModelMonitorPriceSourceUpstreamCatalog = "upstream_catalog"

	ModelMonitorPricingSyncStatusUnknown = "unknown"
	ModelMonitorPricingSyncStatusOK      = "ok"
	ModelMonitorPricingSyncStatusError   = "error"

	ModelMonitorCostKindUnknown                  = "unknown"
	ModelMonitorCostKindEstimatedUpstreamPricing = "estimated_upstream_pricing"
	ModelMonitorCostKindActualUpstream           = "actual_upstream"

	ModelMonitorModelFamilyGPT      = "gpt"
	ModelMonitorModelFamilyGrok     = "grok"
	ModelMonitorModelFamilyClaude   = "claude"
	ModelMonitorModelFamilyGemini   = "gemini"
	ModelMonitorModelFamilyDeepSeek = "deepseek"
	ModelMonitorModelFamilyOther    = "other"

	ModelMonitorModalityText  = "text"
	ModelMonitorModalityImage = "image"

	ModelMonitorBillingClassPaid    = "paid"
	ModelMonitorBillingClassFree    = "free"
	ModelMonitorBillingClassUnknown = "unknown"

	ModelMonitorSiteHealthNormal      = "normal"
	ModelMonitorSiteHealthDegraded    = "degraded"
	ModelMonitorSiteHealthUnavailable = "unavailable"
	ModelMonitorSiteHealthUnknown     = "unknown"
)

type ModelMonitorSite struct {
	ID                int64  `json:"id" gorm:"primaryKey"`
	Name              string `json:"name" gorm:"type:varchar(128);not null;uniqueIndex:uk_model_monitor_site_name"`
	SiteType          string `json:"site_type" gorm:"type:varchar(32);not null;index"`
	PricingGroup      string `json:"pricing_group" gorm:"type:varchar(64)"`
	PricingSyncStatus string `json:"pricing_sync_status" gorm:"type:varchar(32)"`
	PricingSyncError  string `json:"pricing_sync_error,omitempty" gorm:"type:text"`
	PricingSyncedAt   int64  `json:"pricing_synced_at" gorm:"bigint"`
	Enabled           bool   `json:"enabled"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt         int64  `json:"updated_at" gorm:"bigint"`
}

type ModelMonitorSiteChannel struct {
	ID        int64 `json:"id" gorm:"primaryKey"`
	SiteID    int64 `json:"site_id" gorm:"not null;uniqueIndex:uk_model_monitor_site_channel,priority:1;index"`
	ChannelID int   `json:"channel_id" gorm:"not null;uniqueIndex:uk_model_monitor_site_channel,priority:2;index"`
	CreatedAt int64 `json:"created_at" gorm:"bigint;index"`
}

type ModelMonitorTarget struct {
	ID           int64  `json:"id" gorm:"primaryKey"`
	SiteID       int64  `json:"site_id" gorm:"not null;uniqueIndex:uk_model_monitor_target,priority:1;index"`
	ModelName    string `json:"model_name" gorm:"type:varchar(255);not null;uniqueIndex:uk_model_monitor_target,priority:2;index"`
	EndpointType string `json:"endpoint_type" gorm:"type:varchar(64)"`
	Weight       int    `json:"weight" gorm:"not null"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint"`
}

type ModelMonitorPriceSnapshot struct {
	ID           int64  `json:"id" gorm:"primaryKey"`
	SiteID       int64  `json:"site_id" gorm:"not null;index:idx_model_monitor_price_site_model_time,priority:1"`
	ModelName    string `json:"model_name" gorm:"type:varchar(255);not null;index:idx_model_monitor_price_site_model_time,priority:2"`
	Source       string `json:"source" gorm:"type:varchar(64);not null"`
	Version      string `json:"version" gorm:"type:varchar(128)"`
	ModelFamily  string `json:"model_family" gorm:"type:varchar(32);index"`
	Modality     string `json:"modality" gorm:"type:varchar(32);index"`
	BillingClass string `json:"billing_class" gorm:"type:varchar(32);index"`
	PricingData  string `json:"pricing_data" gorm:"type:text;not null"`
	CapturedAt   int64  `json:"captured_at" gorm:"bigint;index:idx_model_monitor_price_site_model_time,priority:3"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
}

type ModelMonitorObservation struct {
	ID                  int64  `json:"id" gorm:"primaryKey"`
	SiteID              int64  `json:"site_id" gorm:"not null;index:idx_model_monitor_observation_site_model_time,priority:1"`
	ChannelID           int    `json:"channel_id" gorm:"not null;index:idx_model_monitor_observation_channel_time,priority:1"`
	TargetID            int64  `json:"target_id" gorm:"index"`
	ModelName           string `json:"model_name" gorm:"type:varchar(255);not null;index:idx_model_monitor_observation_site_model_time,priority:2"`
	UpstreamModelName   string `json:"upstream_model_name" gorm:"type:varchar(255)"`
	UpstreamRequestID   string `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_model_monitor_observation_upstream_request"`
	Status              string `json:"status" gorm:"type:varchar(32);not null;index"`
	Source              string `json:"source" gorm:"type:varchar(32);not null;index"`
	FailureType         string `json:"failure_type" gorm:"type:varchar(64)"`
	ErrorSummary        string `json:"error_summary" gorm:"type:text"`
	FirstResponseMS     *int64 `json:"first_response_ms"`
	TotalDurationMS     int64  `json:"total_duration_ms" gorm:"bigint"`
	PromptTokens        int    `json:"prompt_tokens"`
	CompletionTokens    int    `json:"completion_tokens"`
	CacheReadTokens     int    `json:"cache_read_tokens"`
	CacheCreationTokens int    `json:"cache_creation_tokens"`
	PriceSnapshotID     int64  `json:"price_snapshot_id" gorm:"index"`
	CostMicrousd        int64  `json:"cost_microusd" gorm:"bigint"`
	CostKind            string `json:"cost_kind" gorm:"type:varchar(32)"`
	ObservedAt          int64  `json:"observed_at" gorm:"bigint;not null;index:idx_model_monitor_observation_site_model_time,priority:3;index:idx_model_monitor_observation_channel_time,priority:2"`
	CreatedAt           int64  `json:"created_at" gorm:"bigint;index"`
}

type ModelMonitorAggregateHourly struct {
	ID                      int64  `json:"id" gorm:"primaryKey"`
	SiteID                  int64  `json:"site_id" gorm:"not null;uniqueIndex:uk_model_monitor_aggregate_path_hour,priority:1;index"`
	TargetID                int64  `json:"target_id" gorm:"not null;uniqueIndex:uk_model_monitor_aggregate_path_hour,priority:2;index"`
	ChannelID               int    `json:"channel_id" gorm:"not null;uniqueIndex:uk_model_monitor_aggregate_path_hour,priority:3;index"`
	ModelName               string `json:"model_name" gorm:"type:varchar(255);not null;index"`
	HourStart               int64  `json:"hour_start" gorm:"bigint;not null;uniqueIndex:uk_model_monitor_aggregate_path_hour,priority:4;index"`
	ObservationCount        int    `json:"observation_count"`
	AvailableCount          int    `json:"available_count"`
	LimitedCount            int    `json:"limited_count"`
	UnavailableCount        int    `json:"unavailable_count"`
	UnknownCount            int    `json:"unknown_count"`
	AvailabilityBasisPoints int    `json:"availability_basis_points"`
	FirstResponseP95MS      *int64 `json:"first_response_p95_ms"`
	TotalDurationP95MS      int64  `json:"total_duration_p95_ms" gorm:"bigint"`
	CostMicrousd            int64  `json:"cost_microusd" gorm:"bigint"`
	ActualCostCount         int    `json:"actual_cost_count"`
	EstimatedCostCount      int    `json:"estimated_cost_count"`
	UnknownCostCount        int    `json:"unknown_cost_count"`
	FailureCounts           string `json:"failure_counts" gorm:"type:text;not null"`
	UpdatedAt               int64  `json:"updated_at" gorm:"bigint"`
}

type ModelMonitorEffectiveModel struct {
	ModelName          string `json:"model_name"`
	Status             string `json:"status"`
	LatestStatus       string `json:"latest_status"`
	LatestFailureType  string `json:"latest_failure_type,omitempty"`
	LatestErrorSummary string `json:"latest_error_summary,omitempty"`
	Weight             int    `json:"weight"`
	Stale              bool   `json:"stale"`
}

type ModelMonitorSiteSummary struct {
	Score  int                          `json:"score"`
	Health string                       `json:"health"`
	Models []ModelMonitorEffectiveModel `json:"models"`
}

type ModelMonitorProbePath struct {
	SiteID       int64
	TargetID     int64
	ChannelID    int
	ModelName    string
	EndpointType string
	Weight       int
}

type ModelMonitorProbeScheduleState struct {
	LastSiteProbeAt         int64
	LastFailureAt           int64
	ConsecutiveFailureCount int
	LastFailureType         string
}

func (site *ModelMonitorSite) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if site.CreatedAt == 0 {
		site.CreatedAt = now
	}
	if site.UpdatedAt == 0 {
		site.UpdatedAt = now
	}
	return nil
}

func (target *ModelMonitorTarget) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if target.CreatedAt == 0 {
		target.CreatedAt = now
	}
	if target.UpdatedAt == 0 {
		target.UpdatedAt = now
	}
	return nil
}

func (snapshot *ModelMonitorPriceSnapshot) BeforeCreate(_ *gorm.DB) error {
	if snapshot.CreatedAt == 0 {
		snapshot.CreatedAt = common.GetTimestamp()
	}
	return nil
}

func (observation *ModelMonitorObservation) BeforeCreate(_ *gorm.DB) error {
	if observation.CreatedAt == 0 {
		observation.CreatedAt = common.GetTimestamp()
	}
	if observation.ObservedAt == 0 {
		observation.ObservedAt = observation.CreatedAt
	}
	return nil
}

func (siteChannel *ModelMonitorSiteChannel) BeforeCreate(_ *gorm.DB) error {
	if siteChannel.CreatedAt == 0 {
		siteChannel.CreatedAt = common.GetTimestamp()
	}
	return nil
}

func ValidateModelMonitorTarget(target ModelMonitorTarget) error {
	if target.SiteID <= 0 {
		return errors.New("model monitor target site is required")
	}
	if strings.TrimSpace(target.ModelName) == "" {
		return errors.New("model monitor target model is required")
	}
	if target.Weight < 1 || target.Weight > 5 {
		return errors.New("model monitor target weight must be between 1 and 5")
	}
	return nil
}

func CountEnabledModelMonitorTargets() (int64, error) {
	var count int64
	err := DB.Model(&ModelMonitorTarget{}).Where("enabled = ?", true).Count(&count).Error
	return count, err
}

func ListEnabledModelMonitorProbePaths() ([]ModelMonitorProbePath, error) {
	paths := make([]ModelMonitorProbePath, 0)
	err := DB.Table("model_monitor_targets AS target").
		Select("target.site_id, target.id AS target_id, site_channel.channel_id, target.model_name, target.endpoint_type, target.weight").
		Joins("JOIN model_monitor_sites AS site ON site.id = target.site_id").
		Joins("JOIN model_monitor_site_channels AS site_channel ON site_channel.site_id = target.site_id").
		Where("target.enabled = ? AND site.enabled = ?", true, true).
		Order("target.weight DESC, target.id ASC, site_channel.channel_id ASC").
		Scan(&paths).Error
	return paths, err
}

func ListEnabledModelMonitorPassivePaths(channelID int, modelName string) ([]ModelMonitorProbePath, error) {
	paths := make([]ModelMonitorProbePath, 0)
	err := DB.Table("model_monitor_targets AS target").
		Select("target.site_id, target.id AS target_id, site_channel.channel_id, target.model_name, target.endpoint_type, target.weight").
		Joins("JOIN model_monitor_sites AS site ON site.id = target.site_id").
		Joins("JOIN model_monitor_site_channels AS site_channel ON site_channel.site_id = target.site_id").
		Where("target.enabled = ? AND site.enabled = ? AND site_channel.channel_id = ? AND target.model_name = ?", true, true, channelID, modelName).
		Order("target.id ASC").
		Scan(&paths).Error
	return paths, err
}

func GetModelMonitorProbeScheduleState(siteID int64, targetID int64, channelID int) (ModelMonitorProbeScheduleState, error) {
	state := ModelMonitorProbeScheduleState{}

	var latestSiteObservation ModelMonitorObservation
	err := DB.Where("site_id = ? AND source = ?", siteID, ModelMonitorObservationSourceActive).
		Order("observed_at DESC, id DESC").
		Limit(1).
		Find(&latestSiteObservation).Error
	if err != nil {
		return state, err
	}
	if latestSiteObservation.ID != 0 {
		state.LastSiteProbeAt = latestSiteObservation.ObservedAt
	}

	observations := make([]ModelMonitorObservation, 0, 8)
	err = DB.Where("site_id = ? AND target_id = ? AND channel_id = ? AND source = ?", siteID, targetID, channelID, ModelMonitorObservationSourceActive).
		Order("observed_at DESC, id DESC").
		Limit(8).
		Find(&observations).Error
	if err != nil {
		return state, err
	}
	for _, observation := range observations {
		if observation.Status == ModelMonitorStatusAvailable || observation.FailureType == ModelMonitorFailureTypeNone {
			break
		}
		state.ConsecutiveFailureCount++
		if state.LastFailureAt == 0 {
			state.LastFailureAt = observation.ObservedAt
			state.LastFailureType = observation.FailureType
		}
	}
	return state, nil
}

func BuildModelMonitorSiteSummary(targets []ModelMonitorTarget, observations []ModelMonitorObservation, now int64, unknownGraceSeconds int64) ModelMonitorSiteSummary {
	targetsByModel := make(map[string]ModelMonitorTarget, len(targets))
	for _, target := range targets {
		if !target.Enabled {
			continue
		}
		modelName := strings.TrimSpace(target.ModelName)
		if modelName == "" {
			continue
		}
		targetsByModel[modelName] = target
	}

	modelNames := make([]string, 0, len(targetsByModel))
	for modelName := range targetsByModel {
		modelNames = append(modelNames, modelName)
	}
	sort.Strings(modelNames)

	observationsByModelPath := make(map[string]map[[2]int64][]ModelMonitorObservation, len(modelNames))
	latestObservationByModel := make(map[string]ModelMonitorObservation, len(modelNames))
	for _, observation := range observations {
		if _, ok := targetsByModel[observation.ModelName]; !ok {
			continue
		}
		latestObservation, exists := latestObservationByModel[observation.ModelName]
		if !exists ||
			observation.ObservedAt > latestObservation.ObservedAt ||
			(observation.ObservedAt == latestObservation.ObservedAt && observation.ID > latestObservation.ID) {
			latestObservationByModel[observation.ModelName] = observation
		}
		paths := observationsByModelPath[observation.ModelName]
		if paths == nil {
			paths = make(map[[2]int64][]ModelMonitorObservation)
			observationsByModelPath[observation.ModelName] = paths
		}
		key := [2]int64{observation.TargetID, int64(observation.ChannelID)}
		paths[key] = append(paths[key], observation)
	}

	summary := ModelMonitorSiteSummary{
		Health: ModelMonitorSiteHealthUnknown,
		Models: make([]ModelMonitorEffectiveModel, 0, len(modelNames)),
	}
	totalWeight := 0
	totalScore := 0
	allUnavailable := len(modelNames) > 0
	for _, modelName := range modelNames {
		target := targetsByModel[modelName]
		pathStates := make([]ModelMonitorObservation, 0, len(observationsByModelPath[modelName]))
		for _, pathObservations := range observationsByModelPath[modelName] {
			status, observedAt := deriveModelMonitorPathStatus(pathObservations)
			pathStates = append(pathStates, ModelMonitorObservation{
				Status:     status,
				ObservedAt: observedAt,
			})
		}
		status, observedAt := effectiveModelMonitorStatus(pathStates)
		stale := status == ModelMonitorStatusUnknown && now-observedAt > unknownGraceSeconds
		effective := ModelMonitorEffectiveModel{
			ModelName: modelName,
			Status:    status,
			Weight:    target.Weight,
			Stale:     stale,
		}
		if latestObservation, ok := latestObservationByModel[modelName]; ok {
			effective.LatestStatus = latestObservation.Status
			effective.LatestFailureType = latestObservation.FailureType
			effective.LatestErrorSummary = latestObservation.ErrorSummary
		} else {
			effective.LatestStatus = ModelMonitorStatusUnknown
		}
		summary.Models = append(summary.Models, effective)

		if status != ModelMonitorStatusUnavailable {
			allUnavailable = false
		}
		score, counted := modelMonitorStatusScore(status, stale)
		if !counted {
			continue
		}
		totalWeight += target.Weight
		totalScore += score * target.Weight
	}

	if totalWeight == 0 {
		return summary
	}
	summary.Score = totalScore / totalWeight
	switch {
	case allUnavailable || summary.Score < 50:
		summary.Health = ModelMonitorSiteHealthUnavailable
	case summary.Score == 100:
		summary.Health = ModelMonitorSiteHealthNormal
	default:
		summary.Health = ModelMonitorSiteHealthDegraded
	}
	return summary
}

func deriveModelMonitorPathStatus(observations []ModelMonitorObservation) (string, int64) {
	if len(observations) == 0 {
		return ModelMonitorStatusUnknown, 0
	}
	ordered := append([]ModelMonitorObservation(nil), observations...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].ObservedAt == ordered[j].ObservedAt {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].ObservedAt < ordered[j].ObservedAt
	})

	status := ModelMonitorStatusUnknown
	consecutiveFailures := 0
	consecutiveSuccesses := 0
	latestObservedAt := int64(0)
	for _, observation := range ordered {
		if observation.ObservedAt > latestObservedAt {
			latestObservedAt = observation.ObservedAt
		}
		switch observation.Status {
		case ModelMonitorStatusLimited:
			status = ModelMonitorStatusLimited
			consecutiveFailures = 0
			consecutiveSuccesses = 0
		case ModelMonitorStatusAvailable:
			consecutiveFailures = 0
			consecutiveSuccesses++
			if status == ModelMonitorStatusUnknown || status == ModelMonitorStatusAvailable || consecutiveSuccesses >= 2 {
				status = ModelMonitorStatusAvailable
			}
		case ModelMonitorStatusUnavailable:
			consecutiveSuccesses = 0
			consecutiveFailures++
			if consecutiveFailures >= 3 {
				status = ModelMonitorStatusUnavailable
			}
		}
	}
	return status, latestObservedAt
}

func effectiveModelMonitorStatus(observations []ModelMonitorObservation) (string, int64) {
	if len(observations) == 0 {
		return ModelMonitorStatusUnknown, 0
	}

	hasLimited := false
	hasUnknown := false
	latestObservedAt := int64(0)
	for _, observation := range observations {
		if observation.ObservedAt > latestObservedAt {
			latestObservedAt = observation.ObservedAt
		}
		switch observation.Status {
		case ModelMonitorStatusAvailable:
			return ModelMonitorStatusAvailable, latestObservedAt
		case ModelMonitorStatusLimited:
			hasLimited = true
		case ModelMonitorStatusUnknown:
			hasUnknown = true
		}
	}
	if hasLimited {
		return ModelMonitorStatusLimited, latestObservedAt
	}
	if hasUnknown {
		return ModelMonitorStatusUnknown, latestObservedAt
	}
	return ModelMonitorStatusUnavailable, latestObservedAt
}

func modelMonitorStatusScore(status string, stale bool) (int, bool) {
	switch status {
	case ModelMonitorStatusAvailable:
		return 100, true
	case ModelMonitorStatusLimited:
		return 50, true
	case ModelMonitorStatusUnavailable:
		return 0, true
	case ModelMonitorStatusUnknown:
		if stale {
			return 50, true
		}
	}
	return 0, false
}
