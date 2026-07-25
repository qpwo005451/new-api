package service

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
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
	QuotaPerUnit         *float64           `json:"quota_per_unit,omitempty"`
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

type importedSub2APIModelMonitorPricingData struct {
	AdapterType string                                           `json:"adapter_type"`
	Scopes      map[string]importedSub2APIModelMonitorPriceScope `json:"scopes"`
}

type importedSub2APIModelMonitorPriceScope struct {
	ChannelName        string                                    `json:"channel_name"`
	Platform           string                                    `json:"platform"`
	RateMultiplier     float64                                   `json:"rate_multiplier"`
	PeakRateEnabled    bool                                      `json:"peak_rate_enabled"`
	PeakStart          string                                    `json:"peak_start,omitempty"`
	PeakEnd            string                                    `json:"peak_end,omitempty"`
	PeakRateMultiplier float64                                   `json:"peak_rate_multiplier,omitempty"`
	BillingMode        string                                    `json:"billing_mode"`
	InputPrice         *float64                                  `json:"input_price,omitempty"`
	OutputPrice        *float64                                  `json:"output_price,omitempty"`
	CacheWritePrice    *float64                                  `json:"cache_write_price,omitempty"`
	CacheReadPrice     *float64                                  `json:"cache_read_price,omitempty"`
	PerRequestPrice    *float64                                  `json:"per_request_price,omitempty"`
	Intervals          []ModelMonitorSub2APIImportedPricingRange `json:"intervals,omitempty"`
}

type ModelMonitorSub2APIImportedPricingRange struct {
	MinTokens       int      `json:"min_tokens"`
	MaxTokens       *int     `json:"max_tokens,omitempty"`
	InputPrice      *float64 `json:"input_price,omitempty"`
	OutputPrice     *float64 `json:"output_price,omitempty"`
	CacheWritePrice *float64 `json:"cache_write_price,omitempty"`
	CacheReadPrice  *float64 `json:"cache_read_price,omitempty"`
	PerRequestPrice *float64 `json:"per_request_price,omitempty"`
}

func ImportModelMonitorPricing(request dto.ModelMonitorPricingImportRequest) (ModelMonitorPricingImportResult, error) {
	switch request.SiteType {
	case model.ModelMonitorSiteTypeNewAPI:
		return ImportNewAPIModelMonitorPricing(request)
	case model.ModelMonitorSiteTypeSub2API:
		return ImportSub2APIModelMonitorPricing(request)
	default:
		return ModelMonitorPricingImportResult{}, errors.New("model monitor pricing site type is unsupported")
	}
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
	if request.QuotaPerUnit != nil && (!validModelMonitorPricingNumber(*request.QuotaPerUnit) || *request.QuotaPerUnit == 0) {
		return ModelMonitorPricingImportResult{}, errors.New("model monitor pricing quota per unit is invalid")
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
				Name:         strings.TrimSpace(request.SiteName),
				SiteType:     model.ModelMonitorSiteTypeNewAPI,
				PricingGroup: strings.TrimSpace(request.PricingGroup),
				Enabled:      true,
			}
			if err = tx.Create(&site).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if site.SiteType != model.ModelMonitorSiteTypeNewAPI {
			return errors.New("model monitor pricing site type does not match imported catalog")
		}
		pricingGroup := strings.TrimSpace(request.PricingGroup)
		if pricingGroup != "" && site.PricingGroup != pricingGroup {
			if err = tx.Model(&site).Update("pricing_group", pricingGroup).Error; err != nil {
				return err
			}
			site.PricingGroup = pricingGroup
		}
		result.SiteID = site.ID

		for _, imported := range request.Models {
			data, err := newImportedModelMonitorPricingData(imported, request.QuotaPerUnit, request.GroupRatio)
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
				SiteID:       site.ID,
				ModelName:    imported.ModelName,
				Source:       model.ModelMonitorPriceSourceUpstreamCatalog,
				Version:      request.PricingVersion,
				ModelFamily:  classifyModelMonitorFamily(imported.ModelName),
				Modality:     classifyNewAPIModelMonitorModality(imported),
				BillingClass: classifyNewAPIModelMonitorBilling(imported, request.PricingGroup, request.GroupRatio),
				PricingData:  string(dataBytes),
				CapturedAt:   common.GetTimestamp(),
			}
			if err = tx.Create(&snapshot).Error; err != nil {
				return err
			}
			result.Imported++
		}
		if err = tx.Model(&site).Updates(map[string]any{
			"pricing_sync_status": model.ModelMonitorPricingSyncStatusOK,
			"pricing_sync_error":  "",
			"pricing_synced_at":   common.GetTimestamp(),
		}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return ModelMonitorPricingImportResult{}, err
	}
	return result, nil
}

func ImportSub2APIModelMonitorPricing(request dto.ModelMonitorPricingImportRequest) (ModelMonitorPricingImportResult, error) {
	if strings.TrimSpace(request.SiteName) == "" {
		return ModelMonitorPricingImportResult{}, errors.New("model monitor pricing site name is required")
	}
	if request.SiteType != model.ModelMonitorSiteTypeSub2API {
		return ModelMonitorPricingImportResult{}, errors.New("model monitor pricing import requires the sub2api site type")
	}
	if strings.TrimSpace(request.PricingVersion) == "" {
		return ModelMonitorPricingImportResult{}, errors.New("model monitor pricing version is required")
	}
	if len(request.Sub2APIChannels) == 0 {
		return ModelMonitorPricingImportResult{}, errors.New("model monitor sub2api channels are required")
	}
	for groupID, ratio := range request.Sub2APICustomGroupRates {
		if strings.TrimSpace(groupID) == "" || !validModelMonitorPricingNumber(ratio) {
			return ModelMonitorPricingImportResult{}, errors.New("model monitor sub2api custom group rate is invalid")
		}
	}

	modelPricing, err := buildImportedSub2APIModelMonitorPricing(request)
	if err != nil {
		return ModelMonitorPricingImportResult{}, err
	}
	if len(modelPricing) == 0 {
		return ModelMonitorPricingImportResult{}, errors.New("model monitor sub2api pricing models are required")
	}

	result := ModelMonitorPricingImportResult{}
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		var site model.ModelMonitorSite
		err := tx.Where("name = ?", strings.TrimSpace(request.SiteName)).First(&site).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			site = model.ModelMonitorSite{
				Name:         strings.TrimSpace(request.SiteName),
				SiteType:     model.ModelMonitorSiteTypeSub2API,
				PricingGroup: strings.TrimSpace(request.PricingGroup),
				Enabled:      true,
			}
			if err = tx.Create(&site).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if site.SiteType != model.ModelMonitorSiteTypeSub2API {
			return errors.New("model monitor pricing site type does not match imported catalog")
		}
		pricingGroup := strings.TrimSpace(request.PricingGroup)
		if pricingGroup != "" && site.PricingGroup != pricingGroup {
			if err = tx.Model(&site).Update("pricing_group", pricingGroup).Error; err != nil {
				return err
			}
			site.PricingGroup = pricingGroup
		}
		result.SiteID = site.ID

		for modelName, pricing := range modelPricing {
			dataBytes, err := common.Marshal(pricing)
			if err != nil {
				return fmt.Errorf("marshal model monitor sub2api pricing data: %w", err)
			}
			var latest model.ModelMonitorPriceSnapshot
			err = tx.Where(
				"site_id = ? AND model_name = ? AND source = ?",
				site.ID,
				modelName,
				model.ModelMonitorPriceSourceUpstreamCatalog,
			).Order("captured_at DESC, id DESC").First(&latest).Error
			if err == nil && latest.Version == request.PricingVersion && latest.PricingData == string(dataBytes) {
				result.Reused++
				continue
			}
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err = tx.Create(&model.ModelMonitorPriceSnapshot{
				SiteID:       site.ID,
				ModelName:    modelName,
				Source:       model.ModelMonitorPriceSourceUpstreamCatalog,
				Version:      request.PricingVersion,
				ModelFamily:  classifyModelMonitorFamily(modelName),
				Modality:     classifySub2APIModelMonitorModality(pricing),
				BillingClass: classifySub2APIModelMonitorBilling(pricing, request.PricingGroup),
				PricingData:  string(dataBytes),
				CapturedAt:   common.GetTimestamp(),
			}).Error; err != nil {
				return err
			}
			result.Imported++
		}
		if err = tx.Model(&site).Updates(map[string]any{
			"pricing_sync_status": model.ModelMonitorPricingSyncStatusOK,
			"pricing_sync_error":  "",
			"pricing_synced_at":   common.GetTimestamp(),
		}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return ModelMonitorPricingImportResult{}, err
	}
	return result, nil
}

func RecordModelMonitorPricingSyncFailure(siteName string, syncErr error) {
	siteName = strings.TrimSpace(siteName)
	if siteName == "" || syncErr == nil {
		return
	}
	_ = model.DB.Model(&model.ModelMonitorSite{}).
		Where("name = ?", siteName).
		Updates(map[string]any{
			"pricing_sync_status": model.ModelMonitorPricingSyncStatusError,
			"pricing_sync_error":  syncErr.Error(),
		}).Error
}

func classifyModelMonitorFamily(modelName string) string {
	name := strings.ToLower(modelName)
	switch {
	case strings.Contains(name, "grok"):
		return model.ModelMonitorModelFamilyGrok
	case strings.Contains(name, "claude"):
		return model.ModelMonitorModelFamilyClaude
	case strings.Contains(name, "gemini"):
		return model.ModelMonitorModelFamilyGemini
	case strings.Contains(name, "deepseek"):
		return model.ModelMonitorModelFamilyDeepSeek
	case strings.Contains(name, "gpt") || strings.HasPrefix(name, "o1") || strings.HasPrefix(name, "o3") || strings.HasPrefix(name, "o4"):
		return model.ModelMonitorModelFamilyGPT
	default:
		return model.ModelMonitorModelFamilyOther
	}
}

func classifyNewAPIModelMonitorModality(imported dto.ModelMonitorPricingImportModel) string {
	for _, endpoint := range imported.SupportedEndpointTypes {
		if strings.Contains(strings.ToLower(endpoint), "image") {
			return model.ModelMonitorModalityImage
		}
	}
	name := strings.ToLower(imported.ModelName)
	if strings.Contains(name, "image") || strings.Contains(name, "dall-e") {
		return model.ModelMonitorModalityImage
	}
	return model.ModelMonitorModalityText
}

func classifyNewAPIModelMonitorBilling(
	imported dto.ModelMonitorPricingImportModel,
	pricingGroup string,
	groupRatios map[string]float64,
) string {
	if ratio, ok := groupRatios[strings.TrimSpace(pricingGroup)]; ok && ratio == 0 {
		return model.ModelMonitorBillingClassFree
	}
	if imported.QuotaType == 0 && imported.ModelRatio != nil {
		if *imported.ModelRatio == 0 {
			return model.ModelMonitorBillingClassFree
		}
		return model.ModelMonitorBillingClassPaid
	}
	if imported.QuotaType == 1 && imported.ModelPrice != nil {
		if *imported.ModelPrice == 0 {
			return model.ModelMonitorBillingClassFree
		}
		return model.ModelMonitorBillingClassPaid
	}
	return model.ModelMonitorBillingClassUnknown
}

func classifySub2APIModelMonitorModality(pricing importedSub2APIModelMonitorPricingData) string {
	for _, scope := range pricing.Scopes {
		if scope.BillingMode == "image" {
			return model.ModelMonitorModalityImage
		}
	}
	return model.ModelMonitorModalityText
}

func classifySub2APIModelMonitorBilling(
	pricing importedSub2APIModelMonitorPricingData,
	pricingGroup string,
) string {
	scope, ok := pricing.Scopes[strings.TrimSpace(pricingGroup)]
	if !ok {
		return model.ModelMonitorBillingClassUnknown
	}
	if scope.RateMultiplier == 0 {
		return model.ModelMonitorBillingClassFree
	}
	hasPrice := false
	for _, price := range []*float64{
		scope.InputPrice,
		scope.OutputPrice,
		scope.CacheWritePrice,
		scope.CacheReadPrice,
		scope.PerRequestPrice,
	} {
		if price == nil {
			continue
		}
		hasPrice = true
		if *price > 0 {
			return model.ModelMonitorBillingClassPaid
		}
	}
	if hasPrice {
		return model.ModelMonitorBillingClassFree
	}
	return model.ModelMonitorBillingClassUnknown
}

func buildImportedSub2APIModelMonitorPricing(
	request dto.ModelMonitorPricingImportRequest,
) (map[string]importedSub2APIModelMonitorPricingData, error) {
	result := make(map[string]importedSub2APIModelMonitorPricingData)
	for _, channel := range request.Sub2APIChannels {
		if strings.TrimSpace(channel.Name) == "" {
			return nil, errors.New("model monitor sub2api channel name is required")
		}
		for _, platform := range channel.Platforms {
			if strings.TrimSpace(platform.Platform) == "" {
				return nil, errors.New("model monitor sub2api platform is required")
			}
			for _, group := range platform.Groups {
				if group.ID <= 0 || strings.TrimSpace(group.Name) == "" ||
					!validModelMonitorPricingNumber(group.RateMultiplier) ||
					!validModelMonitorPricingNumber(group.PeakRateMultiplier) {
					return nil, errors.New("model monitor sub2api group is invalid")
				}
			}
			for _, importedModel := range platform.SupportedModels {
				if strings.TrimSpace(importedModel.Name) == "" {
					return nil, errors.New("model monitor sub2api model name is required")
				}
				if importedModel.Pricing == nil {
					continue
				}
				if err := validateSub2APIModelMonitorPricing(importedModel.Pricing); err != nil {
					return nil, err
				}
				pricing := result[importedModel.Name]
				if pricing.AdapterType == "" {
					pricing = importedSub2APIModelMonitorPricingData{
						AdapterType: model.ModelMonitorSiteTypeSub2API,
						Scopes:      make(map[string]importedSub2APIModelMonitorPriceScope),
					}
				}
				for _, group := range platform.Groups {
					rateMultiplier := group.RateMultiplier
					if custom, ok := request.Sub2APICustomGroupRates[strconv.FormatInt(group.ID, 10)]; ok {
						rateMultiplier = custom
					}
					if _, exists := pricing.Scopes[group.Name]; exists {
						return nil, errors.New("model monitor sub2api group pricing is ambiguous")
					}
					pricing.Scopes[group.Name] = importedSub2APIModelMonitorPriceScope{
						ChannelName:        channel.Name,
						Platform:           platform.Platform,
						RateMultiplier:     rateMultiplier,
						PeakRateEnabled:    group.PeakRateEnabled,
						PeakStart:          group.PeakStart,
						PeakEnd:            group.PeakEnd,
						PeakRateMultiplier: group.PeakRateMultiplier,
						BillingMode:        importedModel.Pricing.BillingMode,
						InputPrice:         importedModel.Pricing.InputPrice,
						OutputPrice:        importedModel.Pricing.OutputPrice,
						CacheWritePrice:    importedModel.Pricing.CacheWritePrice,
						CacheReadPrice:     importedModel.Pricing.CacheReadPrice,
						PerRequestPrice:    importedModel.Pricing.PerRequestPrice,
						Intervals:          importedSub2APIPricingRanges(importedModel.Pricing.Intervals),
					}
				}
				result[importedModel.Name] = pricing
			}
		}
	}
	return result, nil
}

func importedSub2APIPricingRanges(ranges []dto.ModelMonitorSub2APIPricingRange) []ModelMonitorSub2APIImportedPricingRange {
	result := make([]ModelMonitorSub2APIImportedPricingRange, 0, len(ranges))
	for _, pricingRange := range ranges {
		result = append(result, ModelMonitorSub2APIImportedPricingRange{
			MinTokens:       pricingRange.MinTokens,
			MaxTokens:       pricingRange.MaxTokens,
			InputPrice:      pricingRange.InputPrice,
			OutputPrice:     pricingRange.OutputPrice,
			CacheWritePrice: pricingRange.CacheWritePrice,
			CacheReadPrice:  pricingRange.CacheReadPrice,
			PerRequestPrice: pricingRange.PerRequestPrice,
		})
	}
	return result
}

func validateSub2APIModelMonitorPricing(pricing *dto.ModelMonitorSub2APIModelPricing) error {
	switch pricing.BillingMode {
	case "", "token", "per_request", "image":
	default:
		return errors.New("model monitor sub2api billing mode is unsupported")
	}
	if err := validateOptionalModelMonitorPricingNumber(
		pricing.InputPrice,
		pricing.OutputPrice,
		pricing.CacheWritePrice,
		pricing.CacheReadPrice,
		pricing.ImageOutputPrice,
		pricing.PerRequestPrice,
	); err != nil {
		return err
	}
	for _, pricingRange := range pricing.Intervals {
		if pricingRange.MinTokens < 0 || pricingRange.MaxTokens != nil && *pricingRange.MaxTokens <= pricingRange.MinTokens {
			return errors.New("model monitor sub2api pricing interval is invalid")
		}
		if err := validateOptionalModelMonitorPricingNumber(
			pricingRange.InputPrice,
			pricingRange.OutputPrice,
			pricingRange.CacheWritePrice,
			pricingRange.CacheReadPrice,
			pricingRange.PerRequestPrice,
		); err != nil {
			return err
		}
	}
	return nil
}

func newImportedModelMonitorPricingData(
	imported dto.ModelMonitorPricingImportModel,
	quotaPerUnit *float64,
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
		QuotaPerUnit:         quotaPerUnit,
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

func ApplyModelMonitorEstimatedCost(observation *model.ModelMonitorObservation) error {
	if observation == nil {
		return errors.New("model monitor observation is required")
	}
	if observation.CostKind == model.ModelMonitorCostKindActualUpstream {
		return nil
	}
	if observation.PromptTokens < 0 || observation.CompletionTokens < 0 ||
		observation.CacheReadTokens < 0 || observation.CacheCreationTokens < 0 {
		return errors.New("model monitor observation token usage is invalid")
	}
	var site model.ModelMonitorSite
	if err := model.DB.First(&site, observation.SiteID).Error; err != nil {
		return err
	}
	if strings.TrimSpace(site.PricingGroup) == "" {
		return nil
	}

	var snapshot model.ModelMonitorPriceSnapshot
	query := model.DB
	if observation.PriceSnapshotID > 0 {
		query = query.Where("id = ? AND site_id = ? AND model_name = ?", observation.PriceSnapshotID, observation.SiteID, observation.ModelName)
	} else {
		query = query.Where(
			"site_id = ? AND model_name = ? AND source = ?",
			observation.SiteID,
			observation.ModelName,
			model.ModelMonitorPriceSourceUpstreamCatalog,
		).Order("captured_at DESC, id DESC")
	}
	if err := query.First(&snapshot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	adapter := struct {
		AdapterType string `json:"adapter_type"`
	}{}
	if err := common.UnmarshalJsonStr(snapshot.PricingData, &adapter); err != nil {
		return fmt.Errorf("decode model monitor pricing snapshot: %w", err)
	}

	var costMicrousd decimal.Decimal
	switch adapter.AdapterType {
	case model.ModelMonitorSiteTypeNewAPI:
		if site.SiteType != model.ModelMonitorSiteTypeNewAPI {
			return nil
		}
		pricing := importedModelMonitorPricingData{}
		if err := common.UnmarshalJsonStr(snapshot.PricingData, &pricing); err != nil {
			return fmt.Errorf("decode model monitor newapi pricing snapshot: %w", err)
		}
		estimated, ok := estimateNewAPIModelMonitorCostMicrousd(observation, site.PricingGroup, pricing)
		if !ok {
			return nil
		}
		costMicrousd = estimated
	case model.ModelMonitorSiteTypeSub2API:
		if site.SiteType != model.ModelMonitorSiteTypeSub2API {
			return nil
		}
		pricing := importedSub2APIModelMonitorPricingData{}
		if err := common.UnmarshalJsonStr(snapshot.PricingData, &pricing); err != nil {
			return fmt.Errorf("decode model monitor sub2api pricing snapshot: %w", err)
		}
		estimated, ok := estimateSub2APIModelMonitorCostMicrousd(observation, site.PricingGroup, pricing)
		if !ok {
			return nil
		}
		costMicrousd = estimated
	default:
		return nil
	}

	if costMicrousd.IsNegative() || costMicrousd.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return errors.New("model monitor estimated cost is invalid")
	}
	observation.PriceSnapshotID = snapshot.ID
	observation.CostMicrousd = costMicrousd.Round(0).IntPart()
	observation.CostKind = model.ModelMonitorCostKindEstimatedUpstreamPricing
	return nil
}

func estimateNewAPIModelMonitorCostMicrousd(
	observation *model.ModelMonitorObservation,
	pricingGroup string,
	pricing importedModelMonitorPricingData,
) (decimal.Decimal, bool) {
	if pricing.QuotaPerUnit == nil || *pricing.QuotaPerUnit <= 0 {
		return decimal.Zero, false
	}
	if observation.PromptTokens+observation.CompletionTokens == 0 {
		return decimal.Zero, false
	}
	groupRatio, ok := pricing.GroupRatio[pricingGroup]
	if !ok {
		return decimal.Zero, false
	}
	switch pricing.QuotaType {
	case 0:
		if pricing.ModelRatio == nil || pricing.CompletionRatio == nil {
			return decimal.Zero, false
		}
		promptTokens := observation.PromptTokens - observation.CacheReadTokens - observation.CacheCreationTokens
		if promptTokens < 0 {
			promptTokens = 0
		}
		cacheRatio := 1.0
		if pricing.CacheRatio != nil {
			cacheRatio = *pricing.CacheRatio
		}
		createCacheRatio := 1.0
		if pricing.CreateCacheRatio != nil {
			createCacheRatio = *pricing.CreateCacheRatio
		}
		weightedTokens := decimal.NewFromInt(int64(promptTokens)).
			Add(decimal.NewFromInt(int64(observation.CacheReadTokens)).Mul(decimal.NewFromFloat(cacheRatio))).
			Add(decimal.NewFromInt(int64(observation.CacheCreationTokens)).Mul(decimal.NewFromFloat(createCacheRatio))).
			Add(decimal.NewFromInt(int64(observation.CompletionTokens)).Mul(decimal.NewFromFloat(*pricing.CompletionRatio)))
		return weightedTokens.
			Mul(decimal.NewFromFloat(*pricing.ModelRatio)).
			Mul(decimal.NewFromFloat(groupRatio)).
			Div(decimal.NewFromFloat(*pricing.QuotaPerUnit)).
			Mul(decimal.NewFromInt(1_000_000)), true
	case 1:
		if pricing.ModelPrice == nil {
			return decimal.Zero, false
		}
		return decimal.NewFromFloat(*pricing.ModelPrice).
			Mul(decimal.NewFromFloat(groupRatio)).
			Mul(decimal.NewFromInt(1_000_000)), true
	default:
		return decimal.Zero, false
	}
}

func estimateSub2APIModelMonitorCostMicrousd(
	observation *model.ModelMonitorObservation,
	pricingGroup string,
	pricing importedSub2APIModelMonitorPricingData,
) (decimal.Decimal, bool) {
	scope, ok := pricing.Scopes[pricingGroup]
	if !ok || scope.PeakRateEnabled {
		return decimal.Zero, false
	}
	rateMultiplier := decimal.NewFromFloat(scope.RateMultiplier)
	if scope.BillingMode == "per_request" || scope.BillingMode == "image" {
		if scope.PerRequestPrice == nil {
			return decimal.Zero, false
		}
		return decimal.NewFromFloat(*scope.PerRequestPrice).
			Mul(rateMultiplier).
			Mul(decimal.NewFromInt(1_000_000)), true
	}
	if observation.PromptTokens+observation.CompletionTokens == 0 {
		return decimal.Zero, false
	}

	prices := scope
	totalContext := observation.PromptTokens + observation.CacheReadTokens
	for _, pricingRange := range scope.Intervals {
		if totalContext <= pricingRange.MinTokens {
			continue
		}
		if pricingRange.MaxTokens != nil && totalContext > *pricingRange.MaxTokens {
			continue
		}
		prices.InputPrice = pricingRange.InputPrice
		prices.OutputPrice = pricingRange.OutputPrice
		prices.CacheWritePrice = pricingRange.CacheWritePrice
		prices.CacheReadPrice = pricingRange.CacheReadPrice
		prices.PerRequestPrice = pricingRange.PerRequestPrice
		break
	}
	if prices.InputPrice == nil || prices.OutputPrice == nil {
		return decimal.Zero, false
	}
	cacheReadPrice := prices.InputPrice
	if prices.CacheReadPrice != nil {
		cacheReadPrice = prices.CacheReadPrice
	}
	cacheWritePrice := prices.InputPrice
	if prices.CacheWritePrice != nil {
		cacheWritePrice = prices.CacheWritePrice
	}
	promptTokens := observation.PromptTokens - observation.CacheReadTokens - observation.CacheCreationTokens
	if promptTokens < 0 {
		promptTokens = 0
	}
	costUSD := decimal.NewFromInt(int64(promptTokens)).
		Mul(decimal.NewFromFloat(*prices.InputPrice)).
		Add(decimal.NewFromInt(int64(observation.CompletionTokens)).Mul(decimal.NewFromFloat(*prices.OutputPrice))).
		Add(decimal.NewFromInt(int64(observation.CacheReadTokens)).Mul(decimal.NewFromFloat(*cacheReadPrice))).
		Add(decimal.NewFromInt(int64(observation.CacheCreationTokens)).Mul(decimal.NewFromFloat(*cacheWritePrice))).
		Mul(rateMultiplier)
	return costUSD.Mul(decimal.NewFromInt(1_000_000)), true
}

func ApplyModelMonitorActualCost(observation *model.ModelMonitorObservation, costUSD float64) error {
	if observation == nil {
		return errors.New("model monitor observation is required")
	}
	if !validModelMonitorPricingNumber(costUSD) {
		return errors.New("model monitor actual cost is invalid")
	}
	costMicrousd := decimal.NewFromFloat(costUSD).Mul(decimal.NewFromInt(1_000_000))
	if costMicrousd.GreaterThan(decimal.NewFromInt(math.MaxInt64)) {
		return errors.New("model monitor actual cost is invalid")
	}
	observation.PriceSnapshotID = 0
	observation.CostMicrousd = costMicrousd.Round(0).IntPart()
	observation.CostKind = model.ModelMonitorCostKindActualUpstream
	return nil
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
