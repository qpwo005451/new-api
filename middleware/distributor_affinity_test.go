package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func createAffinityTestChannel(t *testing.T, db *gorm.DB, id int, group, modelName string) {
	t.Helper()
	priority := int64(0)
	weight := uint(100)
	require.NoError(t, db.Create(&model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "sk-affinity-test",
		Status:   common.ChannelStatusEnabled,
		Name:     "affinity-test-channel",
		Weight:   &weight,
		Models:   modelName,
		Group:    group,
		Priority: &priority,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

// An affinity cache hit must not pin a request to a channel that the
// model-health circuit breaker has put into cooldown. The pin is cleared and
// selection falls through to the normal health-aware selection path.
func TestDistributeAffinityHitSkipsCoolingChannel(t *testing.T) {
	dsn := strings.ReplaceAll(t.Name(), "/", "_")
	db, err := gorm.Open(sqlite.Open("file:"+dsn+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelBalanceProtection{}))
	origDB := model.DB
	origMem := common.MemoryCacheEnabled
	model.DB = db
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { model.DB = origDB; common.MemoryCacheEnabled = origMem })

	const modelName = "deepseek-sticky"
	createAffinityTestChannel(t, db, 2861, "default", modelName)
	createAffinityTestChannel(t, db, 2862, "default", modelName)
	model.InitChannelCache()

	setting := operation_setting.GetChannelAffinitySetting()
	origSetting := *setting
	setting.Enabled = true
	setting.SwitchOnSuccess = false
	setting.DefaultTTLSeconds = 300
	setting.Rules = []operation_setting.ChannelAffinityRule{{
		Name:              "deepseek-sticky-rule",
		ModelRegex:        []string{"^deepseek-.*$"},
		KeySources:        []operation_setting.ChannelAffinityKeySource{{Type: "context_int", Key: "id"}},
		IncludeRuleName:   true,
		IncludeModelName:  true,
		IncludeUsingGroup: true,
	}}
	t.Cleanup(func() { *setting = origSetting })

	origPolicy := *operation_setting.GetModelHealthPolicySetting()
	policySetting := operation_setting.GetModelHealthPolicySetting()
	policySetting.Enabled = true
	policySetting.Rules = []operation_setting.ModelHealthPolicyRule{{
		Name:             "sticky cooldown",
		Enabled:          true,
		Models:           []string{modelName},
		FailureThreshold: 2,
		CooldownSeconds:  300,
		StatusCodes:      []int{503},
	}}
	t.Cleanup(func() { *policySetting = origPolicy })

	// 2861 crossed the failure threshold and is cooling down.
	service.RecordModelHealthFailure(modelName, "default", 2861, http.StatusServiceUnavailable)
	service.RecordModelHealthFailure(modelName, "default", 2861, http.StatusServiceUnavailable)

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-sticky"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = req
	// Pre-seed affinity: user 1 is pinned to the cooling channel 2861. The
	// first lookup only establishes the cache-key meta; the record call
	// populates the cache exactly like a first real request would.
	ctx.Set("id", 1)
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
	preferred, found := service.GetPreferredChannelByAffinity(ctx, modelName, "default")
	require.False(t, found)
	service.RecordChannelAffinity(ctx, 2861)
	preferred, found = service.GetPreferredChannelByAffinity(ctx, modelName, "default")
	require.True(t, found)
	require.Equal(t, 2861, preferred)

	Distribute()(ctx)

	assert.Equal(t, 2862, common.GetContextKeyInt(ctx, constant.ContextKeyChannelId),
		"affinity hit on a cooling channel must fall through to health-aware selection")
}
