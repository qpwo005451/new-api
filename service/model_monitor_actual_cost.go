package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

type ModelMonitorActualCostImportResult struct {
	Matched   int `json:"matched"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Unmatched int `json:"unmatched"`
}

func ImportModelMonitorActualCosts(request dto.ModelMonitorActualCostImportRequest) (ModelMonitorActualCostImportResult, error) {
	result := ModelMonitorActualCostImportResult{}
	siteName := strings.TrimSpace(request.SiteName)
	if siteName == "" {
		return result, errors.New("model monitor actual cost site is required")
	}
	if len(request.Records) == 0 {
		return result, errors.New("model monitor actual cost records are required")
	}

	var site model.ModelMonitorSite
	if err := model.DB.Where("name = ?", siteName).First(&site).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return result, errors.New("model monitor actual cost site was not found")
		}
		return result, err
	}

	records := make([]dto.ModelMonitorActualCostRecord, 0, len(request.Records))
	seen := make(map[string]struct{}, len(request.Records))
	for _, record := range request.Records {
		requestID := strings.TrimSpace(record.RequestID)
		if requestID == "" {
			return result, errors.New("model monitor actual cost request id is required")
		}
		if _, ok := seen[requestID]; ok {
			return result, errors.New("model monitor actual cost request id must be unique")
		}
		seen[requestID] = struct{}{}
		probe := model.ModelMonitorObservation{}
		if err := ApplyModelMonitorActualCost(&probe, record.ActualCost); err != nil {
			return result, err
		}
		record.RequestID = requestID
		records = append(records, record)
	}

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		for _, record := range records {
			observations := make([]model.ModelMonitorObservation, 0, 2)
			if err := tx.Where("site_id = ? AND upstream_request_id = ?", site.ID, record.RequestID).
				Order("id ASC").
				Limit(2).
				Find(&observations).Error; err != nil {
				return err
			}
			if len(observations) == 0 {
				result.Unmatched++
				continue
			}
			if len(observations) > 1 {
				return fmt.Errorf("model monitor actual cost request id is ambiguous: %s", record.RequestID)
			}

			observation := observations[0]
			previousCost := observation.CostMicrousd
			previousKind := observation.CostKind
			previousSnapshotID := observation.PriceSnapshotID
			if err := ApplyModelMonitorActualCost(&observation, record.ActualCost); err != nil {
				return err
			}
			result.Matched++
			if previousCost == observation.CostMicrousd &&
				previousKind == observation.CostKind &&
				previousSnapshotID == observation.PriceSnapshotID {
				result.Unchanged++
				continue
			}
			if err := tx.Model(&model.ModelMonitorObservation{}).
				Where("id = ?", observation.ID).
				Updates(map[string]any{
					"price_snapshot_id": observation.PriceSnapshotID,
					"cost_microusd":     observation.CostMicrousd,
					"cost_kind":         observation.CostKind,
				}).Error; err != nil {
				return err
			}
			result.Updated++
		}
		return nil
	})
	return result, err
}
