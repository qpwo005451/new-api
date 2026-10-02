package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	affinityPlacementTestModel = "deepseek-v4.1-flash"
	affinityPlacementTestGroup = "svip"
)

// setupAffinityPlacementTest builds one enabled channel per entry of weights and
// loads them into the in-memory routing cache.
func setupAffinityPlacementTest(t *testing.T, weights map[int]int) {
	t.Helper()

	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}, &model.ChannelBalanceProtection{}))
	model.DB = db
	common.MemoryCacheEnabled = true

	priority := int64(0)
	for id, weight := range weights {
		channelWeight := uint(weight)
		require.NoError(t, db.Create(&model.Channel{
			Id:       id,
			Type:     constant.ChannelTypeOpenAI,
			Key:      fmt.Sprintf("key-%d", id),
			Status:   common.ChannelStatusEnabled,
			Name:     fmt.Sprintf("channel-%d", id),
			Weight:   &channelWeight,
			Models:   affinityPlacementTestModel,
			Group:    affinityPlacementTestGroup,
			Priority: &priority,
		}).Error)
		require.NoError(t, db.Create(&model.Ability{
			Group:     affinityPlacementTestGroup,
			Model:     affinityPlacementTestModel,
			ChannelId: id,
			Enabled:   true,
			Priority:  &priority,
			Weight:    channelWeight,
		}).Error)
	}
	model.InitChannelCache()

	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		affinityPlacementOnce = sync.Once{}
		affinityPlacementStore = nil
		if originalMemoryCacheEnabled && originalDB != nil && originalDB.Migrator().HasTable(&model.Channel{}) {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})
}

func TestPlaceChannelAffinitySessionSpreadsSessionsByWeight(t *testing.T) {
	setupAffinityPlacementTest(t, map[int]int{101: 50, 102: 40, 103: 10})
	setting := &operation_setting.ChannelAffinitySetting{
		Enabled:   true,
		Placement: operation_setting.ChannelAffinityPlacementBalanced,
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	placed := map[int]int{}
	for i := range 20 {
		key := fmt.Sprintf("channel_affinity:rule:%s:%s:session-%d", affinityPlacementTestModel, affinityPlacementTestGroup, i)
		channelID, ok := PlaceChannelAffinitySession(ctx, setting, key, affinityPlacementTestModel, affinityPlacementTestGroup, 1800)
		require.True(t, ok)
		placed[channelID]++
	}

	// 50/40/10 of twenty sessions, so the split matches the weights exactly.
	assert.Equal(t, map[int]int{101: 10, 102: 8, 103: 2}, placed)
}

func TestPlaceChannelAffinitySessionSkipsZeroWeightCandidates(t *testing.T) {
	setupAffinityPlacementTest(t, map[int]int{101: 50, 102: 0})
	setting := &operation_setting.ChannelAffinitySetting{
		Enabled:   true,
		Placement: operation_setting.ChannelAffinityPlacementBalanced,
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	for i := range 5 {
		key := fmt.Sprintf("channel_affinity:rule:%s:%s:session-%d", affinityPlacementTestModel, affinityPlacementTestGroup, i)
		channelID, ok := PlaceChannelAffinitySession(ctx, setting, key, affinityPlacementTestModel, affinityPlacementTestGroup, 1800)
		require.True(t, ok)
		assert.Equal(t, 101, channelID)
	}
}

func TestPlaceChannelAffinitySessionKeepsWeightedDrawWhenDisabled(t *testing.T) {
	setupAffinityPlacementTest(t, map[int]int{101: 50, 102: 50})
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	key := fmt.Sprintf("channel_affinity:rule:%s:%s:session-1", affinityPlacementTestModel, affinityPlacementTestGroup)

	for _, setting := range []*operation_setting.ChannelAffinitySetting{
		nil,
		{Enabled: true},
		{Enabled: true, Placement: operation_setting.ChannelAffinityPlacementWeighted},
		{Enabled: true, Placement: "unsupported-mode"},
	} {
		channelID, ok := PlaceChannelAffinitySession(ctx, setting, key, affinityPlacementTestModel, affinityPlacementTestGroup, 1800)
		assert.False(t, ok)
		assert.Zero(t, channelID)
	}

	// An auto-group request resolves its group during selection, so placement
	// cannot scope candidates to a group.
	balanced := &operation_setting.ChannelAffinitySetting{
		Enabled:   true,
		Placement: operation_setting.ChannelAffinityPlacementBalanced,
	}
	_, ok := PlaceChannelAffinitySession(ctx, balanced, key, affinityPlacementTestModel, "auto", 1800)
	assert.False(t, ok)
}

func TestAffinityPlacementLedgerExpiresSessions(t *testing.T) {
	ledger := &affinityPlacementLedger{entries: map[string]affinityPlacementEntry{}}
	start := time.Now()
	ledger.place("scope:a", 7, time.Minute, start)
	ledger.place("scope:b", 9, time.Minute, start)

	assert.Equal(t, map[int]int{7: 1, 9: 1}, ledger.counts("scope", start.Add(30*time.Second)))
	assert.Empty(t, ledger.counts("scope", start.Add(2*time.Minute)))
	assert.Empty(t, ledger.counts("other-scope", start))
}

func TestGetPreferredChannelByAffinityPlacesSessionWithoutBinding(t *testing.T) {
	setupAffinityPlacementTest(t, map[int]int{101: 50, 102: 40, 103: 10})

	original := *operation_setting.GetChannelAffinitySetting()
	t.Cleanup(func() { *operation_setting.GetChannelAffinitySetting() = original })

	installRule := func(placement string) {
		*operation_setting.GetChannelAffinitySetting() = operation_setting.ChannelAffinitySetting{
			Enabled:           true,
			DefaultTTLSeconds: 1800,
			Placement:         placement,
			Rules: []operation_setting.ChannelAffinityRule{
				{
					Name:       "placement rule",
					ModelRegex: []string{"^deepseek-.*$"},
					KeySources: []operation_setting.ChannelAffinityKeySource{
						{Type: "request_header", Key: "Session_id"},
					},
					SessionMode:       "prefer",
					TTLSeconds:        1800,
					IncludeRuleName:   true,
					IncludeModelName:  true,
					IncludeUsingGroup: true,
				},
			},
		}
	}
	sessionContext := func(session string) *gin.Context {
		request := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader("{}"))
		request.Header.Set("Session_id", session)
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = request
		return ctx
	}

	// Placement is opt-in: without it a session without a binding is not pinned
	// and keeps the plain weighted draw.
	installRule("")
	_, found := GetPreferredChannelByAffinity(sessionContext("placement-session-0"), affinityPlacementTestModel, affinityPlacementTestGroup)
	assert.False(t, found)

	installRule(operation_setting.ChannelAffinityPlacementBalanced)
	channelID, found := GetPreferredChannelByAffinity(sessionContext("placement-session-1"), affinityPlacementTestModel, affinityPlacementTestGroup)
	require.True(t, found)
	assert.Contains(t, []int{101, 102, 103}, channelID)

	// A second session starts on another channel so the live sessions of the rule
	// stay spread over the configured weights.
	otherChannelID, found := GetPreferredChannelByAffinity(sessionContext("placement-session-2"), affinityPlacementTestModel, affinityPlacementTestGroup)
	require.True(t, found)
	assert.NotEqual(t, channelID, otherChannelID)
}
