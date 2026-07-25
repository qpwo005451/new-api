package service

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

type ModelMonitorPricingImportResult struct {
	SiteID   int64 `json:"site_id"`
	Imported int   `json:"imported"`
	Reused   int   `json:"reused"`
}

type importedModelMonitorPricingData struct {
	AdapterType          string             `json:"adapter_type"`
	QuotaType            int                `json:"quota_type"`
	ModelRatio           *float64           `json:"model_ratio,omitempty"`
	ModelPrice           *float64           `json:"model_price,omitempty"`
	CompletionRatio      *float64           `json:"completion_ratio,omitempty"`
	CacheRatio           *float64           `json:"cache_ratio,omitempty"`
	CreateCacheRatio     *float64           `json:"create_cache_ratio,omitempty"`
	ImageRatio           *float64           `json:"image_ratio,omitempty"`
	AudioRatio           *float64           `json:"audio_ratio,omitempty"`
	AudioCompletionRatio *float64           `json:"audio_completion_ratio,omitempty"`
	GroupRatio           map[string]float64 `json:"group_ratio,omitempty"`
	EnableGroups         []string           `json:"enable_groups,omitempty"`
}

func ImportNewAPIModelMonitorPricing(request dto.ModelMonitorPricingImportRequest) (ModelMonitorPricingImportResult, error) {
	if strings.TrimSpace(request.SiteName) == "" {
		return ModelMonitorPricingImportResult{}, errors.New("model monitor pricing site name is required")
	}
	if request.SiteType != model.ModelMonitorSiteTypeNewAPI {
		return ModelMonitorPricingImportResult{}, errors.New("model monitor pricing import requires the newapi site type")
	}
	if strings.TrimSpace(request.PricingVersion) == "" {
		return ModelMonitorPricingImportResult{}, errors.New("model monitor pricing version is required")
	}
	if len(request.Models) == 0 {
		return ModelMonitorPricingImportResult{}, errors.New("model monitor pricing models are required")
	}
	for group, ratio := range request.GroupRatio {
		if strings.TrimSpace(group) == "" || !validModelMonitorPricingNumber(ratio) {
			return ModelMonitorPricingImportResult{}, errors.New("model monitor pricing group ratio is invalid")
		}
	}

	result := ModelMonitorPricingImportResult{}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var site model.ModelMonitorSite
		err := tx.Where("name = ?", strings.TrimSpace(request.SiteName)).First(&site).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			site = model.ModelMonitorSite{
				Name:     strings.TrimSpace(request.SiteName),
				SiteType: model.ModelMonitorSiteTypeNewAPI,
				Enabled:  true,
			}
			if err = tx.Create(&site).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if site.SiteType != model.ModelMonitorSiteTypeNewAPI {
			return errors.New("model monitor pricing site type does not match imported catalog")
		}
		result.SiteID = site.ID

		for _, imported := range request.Models {
			data, err := newImportedModelMonitorPricingData(imported, request.GroupRatio)
			if err != nil {
				return err
			}
			dataBytes, err := common.Marshal(data)
			if err != nil {
				return fmt.Errorf("marshal model monitor pricing data: %w", err)
			}

			var latest model.ModelMonitorPriceSnapshot
			err = tx.Where(
				"site_id = ? AND model_name = ? AND source = ?",
				site.ID,
				imported.ModelName,
				model.ModelMonitorPriceSourceUpstreamCatalog,
			).Order("captured_at DESC, id DESC").First(&latest).Error
			if err == nil && latest.Version == request.PricingVersion && latest.PricingData == string(dataBytes) {
				result.Reused++
				continue
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			snapshot := model.ModelMonitorPriceSnapshot{
				SiteID:      site.ID,
				ModelName:   imported.ModelName,
				Source:      model.ModelMonitorPriceSourceUpstreamCatalog,
				Version:     request.PricingVersion,
				PricingData: string(dataBytes),
				CapturedAt:  common.GetTimestamp(),
			}
			if err = tx.Create(&snapshot).Error; err != nil {
				return err
			}
			result.Imported++
		}
		return nil
	})
	if err != nil {
		return ModelMonitorPricingImportResult{}, err
	}
	return result, nil
}

func newImportedModelMonitorPricingData(
	imported dto.ModelMonitorPricingImportModel,
	groupRatio map[string]float64,
) (importedModelMonitorPricingData, error) {
	if strings.TrimSpace(imported.ModelName) == "" {
		return importedModelMonitorPricingData{}, errors.New("model monitor pricing model name is required")
	}
	if imported.QuotaType != 0 && imported.QuotaType != 1 {
		return importedModelMonitorPricingData{}, errors.New("model monitor pricing quota type is unsupported")
	}
	if err := validateOptionalModelMonitorPricingNumber(
		imported.ModelRatio,
		imported.ModelPrice,
		imported.CompletionRatio,
		imported.CacheRatio,
		imported.CreateCacheRatio,
		imported.ImageRatio,
		imported.AudioRatio,
		imported.AudioCompletionRatio,
	); err != nil {
		return importedModelMonitorPricingData{}, err
	}
	if imported.QuotaType == 0 && (imported.ModelRatio == nil || imported.CompletionRatio == nil) {
		return importedModelMonitorPricingData{}, errors.New("ratio-priced model is missing model or completion ratio")
	}
	if imported.QuotaType == 1 && imported.ModelPrice == nil {
		return importedModelMonitorPricingData{}, errors.New("fixed-price model is missing model price")
	}

	return importedModelMonitorPricingData{
		AdapterType:          model.ModelMonitorSiteTypeNewAPI,
		QuotaType:            imported.QuotaType,
		ModelRatio:           imported.ModelRatio,
		ModelPrice:           imported.ModelPrice,
		CompletionRatio:      imported.CompletionRatio,
		CacheRatio:           imported.CacheRatio,
		CreateCacheRatio:     imported.CreateCacheRatio,
		ImageRatio:           imported.ImageRatio,
		AudioRatio:           imported.AudioRatio,
		AudioCompletionRatio: imported.AudioCompletionRatio,
		GroupRatio:           groupRatio,
		EnableGroups:         imported.EnableGroups,
	}, nil
}

func validateOptionalModelMonitorPricingNumber(values ...*float64) error {
	for _, value := range values {
		if value != nil && !validModelMonitorPricingNumber(*value) {
			return errors.New("model monitor pricing value is invalid")
		}
	}
	return nil
}

func validModelMonitorPricingNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}
