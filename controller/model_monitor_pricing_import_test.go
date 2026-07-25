package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
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
		"group_ratio":{"plus":0.15,"standard":1},
		"models":[
			{
				"model_name":"claude-opus-4-6",
				"quota_type":0,
				"model_ratio":2.5,
				"completion_ratio":5,
				"cache_ratio":0.2,
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

	var snapshots []model.ModelMonitorPriceSnapshot
	require.NoError(t, db.Where("site_id = ?", site.ID).Order("model_name ASC").Find(&snapshots).Error)
	require.Len(t, snapshots, 2)
	assert.Equal(t, "catalog-v1", snapshots[0].Version)
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
