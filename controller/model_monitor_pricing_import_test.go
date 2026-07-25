package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelMonitorPricingImportTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.ModelMonitorSite{},
		&model.ModelMonitorPriceSnapshot{},
	))

	previousDB := model.DB
	model.DB = db
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.RedisEnabled = previousRedisEnabled
	})
	return db
}

func performModelMonitorPricingImport(
	t *testing.T,
	userID int,
	payload string,
) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", userID)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/model-monitor/pricing-snapshots", bytes.NewBufferString(payload))
	context.Request.Header.Set("Content-Type", "application/json")

	ImportModelMonitorPricingSnapshot(context)
	return recorder
}

func performModelMonitorActualCostImport(
	t *testing.T,
	userID int,
	payload string,
) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", userID)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/model-monitor/actual-costs", bytes.NewBufferString(payload))
	context.Request.Header.Set("Content-Type", "application/json")

	ImportModelMonitorActualCosts(context)
	return recorder
}

func setModelMonitorPricingImportUserIDs(t *testing.T, userIDs []int) {
	t.Helper()

	setting := operation_setting.GetModelMonitorSetting()
	original := *setting
	t.Cleanup(func() {
		*setting = original
	})
	setting.PricingImportUserIDs = userIDs
}

func TestModelMonitorPricingImportCreatesSanitizedSnapshotsAndDeduplicates(t *testing.T) {
	db := setupModelMonitorPricingImportTestDB(t)
	admin := model.User{Username: "pricing-admin", AffCode: "pricing-admin-aff", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&admin).Error)

	payload := `{
		"site_name":"https://688.qzz.io",
		"site_type":"newapi",
		"pricing_version":"catalog-v1",
		"quota_per_unit":500000,
		"pricing_group":"plus",
		"group_ratio":{"plus":0.15,"standard":1},
		"models":[
			{
				"model_name":"claude-opus-4-6",
				"quota_type":0,
				"model_ratio":2.5,
				"completion_ratio":5,
				"cache_ratio":0.2,
				"supported_endpoint_types":["openai"],
				"enable_groups":["plus"]
			},
			{
				"model_name":"grok-4.20-fast",
				"quota_type":1,
				"model_price":0.08,
				"enable_groups":["standard"]
			}
		]
	}`

	first := performModelMonitorPricingImport(t, admin.Id, payload)
	require.Equal(t, http.StatusOK, first.Code)

	var site model.ModelMonitorSite
	require.NoError(t, db.Where("name = ?", "https://688.qzz.io").First(&site).Error)
	assert.Equal(t, model.ModelMonitorSiteTypeNewAPI, site.SiteType)
	assert.Equal(t, "plus", site.PricingGroup)
	assert.Equal(t, model.ModelMonitorPricingSyncStatusOK, site.PricingSyncStatus)
	assert.NotZero(t, site.PricingSyncedAt)

	var snapshots []model.ModelMonitorPriceSnapshot
	require.NoError(t, db.Where("site_id = ?", site.ID).Order("model_name ASC").Find(&snapshots).Error)
	require.Len(t, snapshots, 2)
	assert.Equal(t, "catalog-v1", snapshots[0].Version)
	assert.Equal(t, model.ModelMonitorModelFamilyClaude, snapshots[0].ModelFamily)
	assert.Equal(t, model.ModelMonitorModalityText, snapshots[0].Modality)
	assert.Equal(t, model.ModelMonitorBillingClassPaid, snapshots[0].BillingClass)
	assert.Contains(t, snapshots[0].PricingData, `"quota_per_unit":500000`)
	assert.Contains(t, snapshots[0].PricingData, `"group_ratio":{"plus":0.15,"standard":1}`)
	assert.NotContains(t, snapshots[0].PricingData, "authorization")
	assert.NotContains(t, snapshots[0].PricingData, "cookie")

	second := performModelMonitorPricingImport(t, admin.Id, payload)
	require.Equal(t, http.StatusOK, second.Code)

	var snapshotCount int64
	require.NoError(t, db.Model(&model.ModelMonitorPriceSnapshot{}).Count(&snapshotCount).Error)
	assert.EqualValues(t, 2, snapshotCount)
}

func TestModelMonitorPricingImportRejectsNonAdminAndIncompletePricing(t *testing.T) {
	db := setupModelMonitorPricingImportTestDB(t)
	setModelMonitorPricingImportUserIDs(t, nil)
	user := model.User{Username: "pricing-user", AffCode: "pricing-user-aff", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)

	nonAdmin := performModelMonitorPricingImport(t, user.Id, `{
		"site_name":"https://688.qzz.io",
		"site_type":"newapi",
		"pricing_version":"catalog-v1",
		"models":[{"model_name":"gpt-5","quota_type":0,"model_ratio":1,"completion_ratio":1}]
	}`)
	require.Equal(t, http.StatusForbidden, nonAdmin.Code)

	admin := model.User{Username: "pricing-admin", AffCode: "pricing-admin-aff", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&admin).Error)
	site := model.ModelMonitorSite{Name: "https://688.qzz.io", SiteType: model.ModelMonitorSiteTypeNewAPI, Enabled: true}
	require.NoError(t, db.Create(&site).Error)
	incomplete := performModelMonitorPricingImport(t, admin.Id, `{
		"site_name":"https://688.qzz.io",
		"site_type":"newapi",
		"pricing_version":"catalog-v1",
		"models":[{"model_name":"gpt-5","quota_type":0,"completion_ratio":1}]
	}`)
	require.Equal(t, http.StatusBadRequest, incomplete.Code)

	var snapshotCount int64
	require.NoError(t, db.Model(&model.ModelMonitorPriceSnapshot{}).Count(&snapshotCount).Error)
	assert.Zero(t, snapshotCount)
	require.NoError(t, db.First(&site, site.ID).Error)
	assert.Equal(t, model.ModelMonitorPricingSyncStatusError, site.PricingSyncStatus)
	assert.NotEmpty(t, site.PricingSyncError)
}

func TestModelMonitorPricingImportAllowsConfiguredOrdinaryUser(t *testing.T) {
	db := setupModelMonitorPricingImportTestDB(t)
	user := model.User{Username: "pricing-importer", AffCode: "pricing-importer-aff", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	setModelMonitorPricingImportUserIDs(t, []int{user.Id})

	response := performModelMonitorPricingImport(t, user.Id, `{
		"site_name":"https://688.qzz.io",
		"site_type":"newapi",
		"pricing_version":"catalog-v1",
		"models":[{"model_name":"gpt-5","quota_type":0,"model_ratio":1,"completion_ratio":1}]
	}`)
	require.Equal(t, http.StatusOK, response.Code)

	var snapshotCount int64
	require.NoError(t, db.Model(&model.ModelMonitorPriceSnapshot{}).Count(&snapshotCount).Error)
	assert.EqualValues(t, 1, snapshotCount)
}

func TestModelMonitorActualCostImportAllowsConfiguredOrdinaryUserAndRedactsErrors(t *testing.T) {
	db := setupModelMonitorPricingImportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ModelMonitorObservation{}))
	user := model.User{Username: "cost-importer", AffCode: "cost-importer-aff", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	setModelMonitorPricingImportUserIDs(t, []int{user.Id})
	site := model.ModelMonitorSite{Name: "sub2api", SiteType: model.ModelMonitorSiteTypeSub2API, Enabled: true}
	require.NoError(t, db.Create(&site).Error)
	observation := model.ModelMonitorObservation{
		SiteID: site.ID, ChannelID: 10, ModelName: "gpt-5.4",
		UpstreamRequestID: "req_cost_1", Status: model.ModelMonitorStatusAvailable,
		Source: model.ModelMonitorObservationSourceActive, FailureType: model.ModelMonitorFailureTypeNone,
		CostKind: model.ModelMonitorCostKindUnknown,
	}
	require.NoError(t, db.Create(&observation).Error)

	response := performModelMonitorActualCostImport(t, user.Id, `{
		"site_name":"sub2api",
		"records":[{"request_id":"req_cost_1","actual_cost":0.0042}]
	}`)
	require.Equal(t, http.StatusOK, response.Code)
	assert.NotContains(t, strings.ToLower(response.Body.String()), "authorization")
	assert.NotContains(t, strings.ToLower(response.Body.String()), "cookie")

	var stored model.ModelMonitorObservation
	require.NoError(t, db.First(&stored, observation.ID).Error)
	assert.EqualValues(t, 4200, stored.CostMicrousd)
	assert.Equal(t, model.ModelMonitorCostKindActualUpstream, stored.CostKind)
}
