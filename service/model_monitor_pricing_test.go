package service

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelMonitorPricingTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.ModelMonitorSite{},
		&model.ModelMonitorPriceSnapshot{},
		&model.ModelMonitorObservation{},
	))

	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
	})
	return db
}

func TestNewAPIModelMonitorPricingEstimatesCostFromReferencedSnapshot(t *testing.T) {
	db := setupModelMonitorPricingTestDB(t)
	site := model.ModelMonitorSite{
		Name:         "input",
		SiteType:     model.ModelMonitorSiteTypeNewAPI,
		PricingGroup: "plus",
		Enabled:      true,
	}
	require.NoError(t, db.Create(&site).Error)

	snapshot := model.ModelMonitorPriceSnapshot{
		SiteID:    site.ID,
		ModelName: "gpt-5",
		Source:    model.ModelMonitorPriceSourceUpstreamCatalog,
		Version:   "catalog-v1",
		PricingData: `{
			"adapter_type":"newapi",
			"quota_type":0,
			"quota_per_unit":500000,
			"model_ratio":2,
			"completion_ratio":4,
			"cache_ratio":0.1,
			"create_cache_ratio":1.25,
			"group_ratio":{"plus":0.5}
		}`,
		CapturedAt: common.GetTimestamp(),
	}
	require.NoError(t, db.Create(&snapshot).Error)

	observation := model.ModelMonitorObservation{
		SiteID:              site.ID,
		ModelName:           "gpt-5",
		PromptTokens:        1000,
		CompletionTokens:    100,
		CacheReadTokens:     200,
		CacheCreationTokens: 100,
		CostKind:            model.ModelMonitorCostKindUnknown,
	}
	require.NoError(t, ApplyModelMonitorEstimatedCost(&observation))
	assert.Equal(t, snapshot.ID, observation.PriceSnapshotID)
	assert.EqualValues(t, 2490, observation.CostMicrousd)
	assert.Equal(t, model.ModelMonitorCostKindEstimatedUpstreamPricing, observation.CostKind)
}

func TestNewAPIModelMonitorPricingKeepsHistoricalSnapshotCost(t *testing.T) {
	db := setupModelMonitorPricingTestDB(t)
	site := model.ModelMonitorSite{
		Name:         "input",
		SiteType:     model.ModelMonitorSiteTypeNewAPI,
		PricingGroup: "default",
		Enabled:      true,
	}
	require.NoError(t, db.Create(&site).Error)

	first := model.ModelMonitorPriceSnapshot{
		SiteID:      site.ID,
		ModelName:   "gpt-5",
		Source:      model.ModelMonitorPriceSourceUpstreamCatalog,
		Version:     "catalog-v1",
		PricingData: `{"adapter_type":"newapi","quota_type":0,"quota_per_unit":500000,"model_ratio":1,"completion_ratio":1,"group_ratio":{"default":1}}`,
		CapturedAt:  100,
	}
	require.NoError(t, db.Create(&first).Error)

	observation := model.ModelMonitorObservation{
		SiteID:          site.ID,
		ModelName:       "gpt-5",
		PromptTokens:    1000,
		PriceSnapshotID: first.ID,
		CostKind:        model.ModelMonitorCostKindUnknown,
	}
	require.NoError(t, ApplyModelMonitorEstimatedCost(&observation))
	assert.EqualValues(t, 2000, observation.CostMicrousd)

	second := model.ModelMonitorPriceSnapshot{
		SiteID:      site.ID,
		ModelName:   "gpt-5",
		Source:      model.ModelMonitorPriceSourceUpstreamCatalog,
		Version:     "catalog-v2",
		PricingData: `{"adapter_type":"newapi","quota_type":0,"quota_per_unit":500000,"model_ratio":10,"completion_ratio":1,"group_ratio":{"default":1}}`,
		CapturedAt:  200,
	}
	require.NoError(t, db.Create(&second).Error)

	require.NoError(t, ApplyModelMonitorEstimatedCost(&observation))
	assert.Equal(t, first.ID, observation.PriceSnapshotID)
	assert.EqualValues(t, 2000, observation.CostMicrousd)
}

func TestNewAPIModelMonitorPricingLeavesCostUnknownWithoutExplicitGroup(t *testing.T) {
	db := setupModelMonitorPricingTestDB(t)
	site := model.ModelMonitorSite{
		Name:     "input",
		SiteType: model.ModelMonitorSiteTypeNewAPI,
		Enabled:  true,
	}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Create(&model.ModelMonitorPriceSnapshot{
		SiteID:      site.ID,
		ModelName:   "gpt-5",
		Source:      model.ModelMonitorPriceSourceUpstreamCatalog,
		Version:     "catalog-v1",
		PricingData: `{"adapter_type":"newapi","quota_type":0,"quota_per_unit":500000,"model_ratio":1,"completion_ratio":1,"group_ratio":{"default":1}}`,
		CapturedAt:  100,
	}).Error)

	observation := model.ModelMonitorObservation{
		SiteID:       site.ID,
		ModelName:    "gpt-5",
		PromptTokens: 1000,
		CostKind:     model.ModelMonitorCostKindUnknown,
	}
	require.NoError(t, ApplyModelMonitorEstimatedCost(&observation))
	assert.Zero(t, observation.PriceSnapshotID)
	assert.Zero(t, observation.CostMicrousd)
	assert.Equal(t, model.ModelMonitorCostKindUnknown, observation.CostKind)
}

func TestModelMonitorActualCostOverridesEstimate(t *testing.T) {
	observation := model.ModelMonitorObservation{
		PriceSnapshotID: 7,
		CostMicrousd:    1200,
		CostKind:        model.ModelMonitorCostKindEstimatedUpstreamPricing,
	}

	require.NoError(t, ApplyModelMonitorActualCost(&observation, 0.00321))
	assert.Zero(t, observation.PriceSnapshotID)
	assert.EqualValues(t, 3210, observation.CostMicrousd)
	assert.Equal(t, model.ModelMonitorCostKindActualUpstream, observation.CostKind)
	assert.Error(t, ApplyModelMonitorActualCost(&observation, -1))
}

func TestImportModelMonitorActualCostsMatchesExactUpstreamRequestID(t *testing.T) {
	db := setupModelMonitorPricingTestDB(t)
	site := model.ModelMonitorSite{Name: "sub2api", SiteType: model.ModelMonitorSiteTypeSub2API, Enabled: true}
	require.NoError(t, db.Create(&site).Error)
	observations := []model.ModelMonitorObservation{
		{
			SiteID: site.ID, ChannelID: 10, ModelName: "gpt-5.4",
			UpstreamRequestID: "req_exact_1", Status: model.ModelMonitorStatusAvailable,
			Source: model.ModelMonitorObservationSourceActive, FailureType: model.ModelMonitorFailureTypeNone,
			CostMicrousd: 1200, CostKind: model.ModelMonitorCostKindEstimatedUpstreamPricing,
		},
		{
			SiteID: site.ID, ChannelID: 11, ModelName: "gpt-5.4",
			UpstreamRequestID: "req_other", Status: model.ModelMonitorStatusAvailable,
			Source: model.ModelMonitorObservationSourceActive, FailureType: model.ModelMonitorFailureTypeNone,
			CostKind: model.ModelMonitorCostKindUnknown,
		},
	}
	require.NoError(t, db.Create(&observations).Error)

	result, err := ImportModelMonitorActualCosts(dto.ModelMonitorActualCostImportRequest{
		SiteName: site.Name,
		Records: []dto.ModelMonitorActualCostRecord{
			{RequestID: "req_exact_1", ActualCost: 0.00321},
			{RequestID: "req_missing", ActualCost: 0.5},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Matched)
	assert.Equal(t, 1, result.Unmatched)

	var matched model.ModelMonitorObservation
	require.NoError(t, db.First(&matched, observations[0].ID).Error)
	assert.EqualValues(t, 3210, matched.CostMicrousd)
	assert.Equal(t, model.ModelMonitorCostKindActualUpstream, matched.CostKind)
	assert.Zero(t, matched.PriceSnapshotID)

	var untouched model.ModelMonitorObservation
	require.NoError(t, db.First(&untouched, observations[1].ID).Error)
	assert.Equal(t, model.ModelMonitorCostKindUnknown, untouched.CostKind)

	repeated, err := ImportModelMonitorActualCosts(dto.ModelMonitorActualCostImportRequest{
		SiteName: site.Name,
		Records:  []dto.ModelMonitorActualCostRecord{{RequestID: "req_exact_1", ActualCost: 0.00321}},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, repeated.Matched)
	assert.Equal(t, 1, repeated.Unchanged)
}

func TestImportModelMonitorActualCostsRejectsAmbiguousOrInvalidRecords(t *testing.T) {
	db := setupModelMonitorPricingTestDB(t)
	site := model.ModelMonitorSite{Name: "sub2api", SiteType: model.ModelMonitorSiteTypeSub2API, Enabled: true}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Create(&[]model.ModelMonitorObservation{
		{
			SiteID: site.ID, ChannelID: 10, ModelName: "gpt-5.4",
			UpstreamRequestID: "req_duplicate", Status: model.ModelMonitorStatusAvailable,
			Source: model.ModelMonitorObservationSourceActive, FailureType: model.ModelMonitorFailureTypeNone,
		},
		{
			SiteID: site.ID, ChannelID: 11, ModelName: "gpt-5.4",
			UpstreamRequestID: "req_duplicate", Status: model.ModelMonitorStatusAvailable,
			Source: model.ModelMonitorObservationSourceActive, FailureType: model.ModelMonitorFailureTypeNone,
		},
	}).Error)

	_, err := ImportModelMonitorActualCosts(dto.ModelMonitorActualCostImportRequest{
		SiteName: site.Name,
		Records:  []dto.ModelMonitorActualCostRecord{{RequestID: "req_duplicate", ActualCost: 0.1}},
	})
	assert.ErrorContains(t, err, "ambiguous")

	_, err = ImportModelMonitorActualCosts(dto.ModelMonitorActualCostImportRequest{
		SiteName: site.Name,
		Records:  []dto.ModelMonitorActualCostRecord{{RequestID: "", ActualCost: 0.1}},
	})
	assert.Error(t, err)

	_, err = ImportModelMonitorActualCosts(dto.ModelMonitorActualCostImportRequest{
		SiteName: site.Name,
		Records:  []dto.ModelMonitorActualCostRecord{{RequestID: "req_negative", ActualCost: -1}},
	})
	assert.Error(t, err)
}

func TestSub2APIModelMonitorPricingImportsRealCatalogShapeAndEstimatesCost(t *testing.T) {
	db := setupModelMonitorPricingTestDB(t)
	fixture, err := os.ReadFile("testdata/sub2api_channels_available.json")
	require.NoError(t, err)
	envelope := struct {
		Code int                              `json:"code"`
		Data []dto.ModelMonitorSub2APIChannel `json:"data"`
	}{}
	require.NoError(t, common.Unmarshal(fixture, &envelope))
	require.Zero(t, envelope.Code)

	result, err := ImportModelMonitorPricing(dto.ModelMonitorPricingImportRequest{
		SiteName:        "https://sub2api.example",
		SiteType:        model.ModelMonitorSiteTypeSub2API,
		PricingVersion:  "catalog-sha256-v1",
		PricingGroup:    "CodeX",
		Sub2APIChannels: envelope.Data,
		Sub2APICustomGroupRates: map[string]float64{
			"12": 0.5,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Imported)

	observation := model.ModelMonitorObservation{
		SiteID:              result.SiteID,
		ModelName:           "gpt-5.4",
		PromptTokens:        1000,
		CompletionTokens:    100,
		CacheReadTokens:     200,
		CacheCreationTokens: 100,
		CostKind:            model.ModelMonitorCostKindUnknown,
	}
	require.NoError(t, ApplyModelMonitorEstimatedCost(&observation))
	assert.Equal(t, model.ModelMonitorCostKindEstimatedUpstreamPricing, observation.CostKind)
	assert.EqualValues(t, 1418, observation.CostMicrousd)

	var snapshot model.ModelMonitorPriceSnapshot
	require.NoError(t, db.First(&snapshot, observation.PriceSnapshotID).Error)
	assert.Equal(t, model.ModelMonitorModelFamilyGPT, snapshot.ModelFamily)
	assert.Equal(t, model.ModelMonitorModalityText, snapshot.Modality)
	assert.Equal(t, model.ModelMonitorBillingClassPaid, snapshot.BillingClass)
	assert.Contains(t, snapshot.PricingData, `"adapter_type":"sub2api"`)
	assert.Contains(t, snapshot.PricingData, `"rate_multiplier":0.5`)
	assert.NotContains(t, snapshot.PricingData, "description")
}

func TestSub2APIModelMonitorPricingDoesNotEstimatePeakRateFromCatalogSnapshot(t *testing.T) {
	db := setupModelMonitorPricingTestDB(t)
	site := model.ModelMonitorSite{
		Name:         "sub2api",
		SiteType:     model.ModelMonitorSiteTypeSub2API,
		PricingGroup: "Peak",
		Enabled:      true,
	}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Create(&model.ModelMonitorPriceSnapshot{
		SiteID:      site.ID,
		ModelName:   "gpt-5.4",
		Source:      model.ModelMonitorPriceSourceUpstreamCatalog,
		Version:     "catalog-v1",
		PricingData: `{"adapter_type":"sub2api","scopes":{"Peak":{"channel_name":"Primary","platform":"openai","rate_multiplier":1,"peak_rate_enabled":true,"peak_start":"09:00","peak_end":"18:00","peak_rate_multiplier":2,"billing_mode":"token","input_price":0.000001,"output_price":0.000002}}}`,
		CapturedAt:  100,
	}).Error)

	observation := model.ModelMonitorObservation{
		SiteID:       site.ID,
		ModelName:    "gpt-5.4",
		PromptTokens: 1000,
		CostKind:     model.ModelMonitorCostKindUnknown,
	}
	require.NoError(t, ApplyModelMonitorEstimatedCost(&observation))
	assert.Equal(t, model.ModelMonitorCostKindUnknown, observation.CostKind)
	assert.Zero(t, observation.PriceSnapshotID)
}
