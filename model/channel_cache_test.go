package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelCacheTestDB(t *testing.T) {
	t.Helper()

	previousDB := DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousGroupChannels := group2model2channels
	previousChannels := channelsIDM
	previousProtections := channel2balanceProtection
	previousAdvancedConfigs := channel2advancedCustomConfig
	t.Cleanup(func() {
		DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		group2model2channels = previousGroupChannels
		channelsIDM = previousChannels
		channel2balanceProtection = previousProtections
		channel2advancedCustomConfig = previousAdvancedConfigs
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ChannelBalanceProtection{}))

	DB = db
	common.MemoryCacheEnabled = true
}

func createChannelCacheTestFixture(t *testing.T) *Channel {
	t.Helper()

	channel := &Channel{
		Name:   "cache-test",
		Status: common.ChannelStatusEnabled,
		Models: "free-model,paid-model",
		Group:  "svip",
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "svip",
		Model:     "free-model",
		ChannelId: channel.Id,
		Enabled:   true,
	}).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "svip",
		Model:     "paid-model",
		ChannelId: channel.Id,
		Enabled:   true,
	}).Error)
	return channel
}

func TestInitChannelCacheKeepsPreviousSnapshotWhenDatabaseReadFails(t *testing.T) {
	setupChannelCacheTestDB(t)
	channel := createChannelCacheTestFixture(t)

	InitChannelCache()
	cached, err := GetRandomSatisfiedChannel("svip", "paid-model", 0, nil)
	require.NoError(t, err)
	require.NotNil(t, cached)
	assert.Equal(t, channel.Id, cached.Id)

	require.NoError(t, DB.Migrator().DropTable(&Ability{}))
	InitChannelCache()

	cached, err = GetRandomSatisfiedChannel("svip", "paid-model", 0, nil)
	require.NoError(t, err)
	require.NotNil(t, cached)
	assert.Equal(t, channel.Id, cached.Id)
}

func TestInitChannelCacheKeepsPreviousBalanceProtectionWhenProtectionReadFails(t *testing.T) {
	setupChannelCacheTestDB(t)
	channel := createChannelCacheTestFixture(t)
	protection := &ChannelBalanceProtection{
		ChannelId:  channel.Id,
		Enabled:    true,
		State:      BalanceProtectionStateProtected,
		FreeModels: `["free-model"]`,
	}
	require.NoError(t, DB.Create(protection).Error)

	InitChannelCache()
	cached, err := GetRandomSatisfiedChannel("svip", "paid-model", 0, nil)
	require.NoError(t, err)
	assert.Nil(t, cached)

	require.NoError(t, DB.Migrator().DropTable(&ChannelBalanceProtection{}))
	InitChannelCache()

	cached, err = GetRandomSatisfiedChannel("svip", "paid-model", 0, nil)
	require.NoError(t, err)
	assert.Nil(t, cached)
}

func TestGetRandomSatisfiedChannelSinglePassPriorityFallback(t *testing.T) {
	setupChannelCacheTestDB(t)

	setting := operation_setting.GetModelRetryPolicySetting()
	previousModels := append([]string(nil), setting.SinglePassPriorityModels...)
	setting.SinglePassPriorityModels = []string{"single-pass-model"}
	t.Cleanup(func() {
		setting.SinglePassPriorityModels = previousModels
	})

	for _, priority := range []int64{300, 200, 100} {
		channel := &Channel{
			Name:     "priority-channel",
			Status:   common.ChannelStatusEnabled,
			Models:   "single-pass-model,ordinary-model",
			Group:    "svip",
			Priority: &priority,
		}
		require.NoError(t, DB.Create(channel).Error)
		for _, modelName := range []string{"single-pass-model", "ordinary-model"} {
			require.NoError(t, DB.Create(&Ability{
				Group:     "svip",
				Model:     modelName,
				ChannelId: channel.Id,
				Enabled:   true,
				Priority:  &priority,
			}).Error)
		}
	}

	for _, memoryCacheEnabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "memory cache", false: "database"}[memoryCacheEnabled], func(t *testing.T) {
			common.MemoryCacheEnabled = memoryCacheEnabled
			if memoryCacheEnabled {
				InitChannelCache()
			}

			for retry, wantPriority := range []int64{300, 200, 100} {
				channel, err := GetRandomSatisfiedChannel("svip", "single-pass-model", retry, nil)
				require.NoError(t, err)
				require.NotNil(t, channel)
				assert.Equal(t, wantPriority, channel.GetPriority())
			}

			channel, err := GetRandomSatisfiedChannel("svip", "single-pass-model", 3, nil)
			require.ErrorIs(t, err, ErrPriorityFallbackExhausted)
			assert.Nil(t, channel)

			channel, err = GetRandomSatisfiedChannel("svip", "ordinary-model", 3, nil)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, int64(100), channel.GetPriority())
		})
	}
}

func TestGetRandomSatisfiedChannelUsesPerModelWeightOverride(t *testing.T) {
	setupChannelCacheTestDB(t)

	channelA := &Channel{Name: "a", Status: common.ChannelStatusEnabled, Models: "shared-model", Group: "svip", Priority: common.GetPointer(int64(10))}
	channelB := &Channel{Name: "b", Status: common.ChannelStatusEnabled, Models: "shared-model", Group: "svip", Priority: common.GetPointer(int64(10))}
	require.NoError(t, DB.Create(channelA).Error)
	require.NoError(t, DB.Create(channelB).Error)
	require.NoError(t, channelA.UpdateAbilities(nil))
	require.NoError(t, channelB.UpdateAbilities(nil))
	InitChannelCache()

	previous := operation_setting.GetModelWeightSetting().Weights
	operation_setting.GetModelWeightSetting().Weights = `[{"channel_id":` + strconv.Itoa(channelB.Id) + `,"model":"shared-model","weight":100000}]`
	t.Cleanup(func() { operation_setting.GetModelWeightSetting().Weights = previous })

	picked := 0
	for range 200 {
		channel, err := GetRandomSatisfiedChannel("svip", "shared-model", 0, nil)
		require.NoError(t, err)
		require.NotNil(t, channel)
		if channel.Id == channelB.Id {
			picked++
		}
	}
	assert.Greater(t, picked, 190, "the overridden channel must dominate selection")
}

func TestSelectSatisfiedChannelFromCandidatesUsesPerModelWeightOverride(t *testing.T) {
	setupChannelCacheTestDB(t)

	channelA := &Channel{Name: "a", Status: common.ChannelStatusEnabled, Models: "shared-model", Group: "svip", Priority: common.GetPointer(int64(10))}
	channelB := &Channel{Name: "b", Status: common.ChannelStatusEnabled, Models: "shared-model", Group: "svip", Priority: common.GetPointer(int64(10))}
	require.NoError(t, DB.Create(channelA).Error)
	require.NoError(t, DB.Create(channelB).Error)
	require.NoError(t, channelA.UpdateAbilities(nil))
	require.NoError(t, channelB.UpdateAbilities(nil))
	InitChannelCache()

	previous := operation_setting.GetModelWeightSetting().Weights
	operation_setting.GetModelWeightSetting().Weights = `[{"channel_id":` + strconv.Itoa(channelA.Id) + `,"model":"shared-model","weight":100000}]`
	t.Cleanup(func() { operation_setting.GetModelWeightSetting().Weights = previous })

	candidates, err := GetSatisfiedChannelsInPriorityOrder("svip", "shared-model", nil)
	require.NoError(t, err)

	picked := 0
	for range 200 {
		channel, err := SelectSatisfiedChannelFromCandidates(candidates, 0, "shared-model")
		require.NoError(t, err)
		require.NotNil(t, channel)
		if channel.Id == channelA.Id {
			picked++
		}
	}
	assert.Greater(t, picked, 190, "the overridden channel must dominate selection")
}

func TestGetChannelWithoutMemoryCacheUsesPerModelWeightOverride(t *testing.T) {
	setupChannelCacheTestDB(t)
	common.MemoryCacheEnabled = false

	channelA := &Channel{Name: "a", Status: common.ChannelStatusEnabled, Models: "shared-model", Group: "svip", Priority: common.GetPointer(int64(10))}
	channelB := &Channel{Name: "b", Status: common.ChannelStatusEnabled, Models: "shared-model", Group: "svip", Priority: common.GetPointer(int64(10))}
	require.NoError(t, DB.Create(channelA).Error)
	require.NoError(t, DB.Create(channelB).Error)
	require.NoError(t, channelA.UpdateAbilities(nil))
	require.NoError(t, channelB.UpdateAbilities(nil))

	previous := operation_setting.GetModelWeightSetting().Weights
	operation_setting.GetModelWeightSetting().Weights = `[{"channel_id":` + strconv.Itoa(channelB.Id) + `,"model":"shared-model","weight":100000}]`
	t.Cleanup(func() { operation_setting.GetModelWeightSetting().Weights = previous })

	picked := 0
	for range 200 {
		channel, err := GetChannel("svip", "shared-model", 0, nil)
		require.NoError(t, err)
		require.NotNil(t, channel)
		if channel.Id == channelB.Id {
			picked++
		}
	}
	assert.Greater(t, picked, 190)
}

// createPriorityOverrideFixture creates two channels that serve the same two
// models at different channel-level priorities. A per-model priority override
// must move the lower channel into the top tier for one model only.
func createPriorityOverrideFixture(t *testing.T) (high *Channel, low *Channel) {
	t.Helper()

	high = &Channel{Name: "high", Status: common.ChannelStatusEnabled, Models: "shared-model,other-model", Group: "svip", Priority: common.GetPointer(int64(20))}
	low = &Channel{Name: "low", Status: common.ChannelStatusEnabled, Models: "shared-model,other-model", Group: "svip", Priority: common.GetPointer(int64(10))}
	require.NoError(t, DB.Create(high).Error)
	require.NoError(t, DB.Create(low).Error)
	require.NoError(t, high.UpdateAbilities(nil))
	require.NoError(t, low.UpdateAbilities(nil))
	return high, low
}

func setModelWeightsForSelectionTest(t *testing.T, raw string) {
	t.Helper()
	previous := operation_setting.GetModelWeightSetting().Weights
	operation_setting.GetModelWeightSetting().Weights = raw
	t.Cleanup(func() { operation_setting.GetModelWeightSetting().Weights = previous })
}

func TestGetRandomSatisfiedChannelUsesPerModelPriorityOverride(t *testing.T) {
	setupChannelCacheTestDB(t)
	_, low := createPriorityOverrideFixture(t)
	InitChannelCache()

	setModelWeightsForSelectionTest(t, `[{"channel_id":`+strconv.Itoa(low.Id)+`,"model":"shared-model","priority":20}]`)

	pickedLow := 0
	for range 200 {
		channel, err := GetRandomSatisfiedChannel("svip", "shared-model", 0, nil)
		require.NoError(t, err)
		require.NotNil(t, channel)
		if channel.Id == low.Id {
			pickedLow++
		}
	}
	assert.Greater(t, pickedLow, 60, "the priority override lifts the lower-priority channel into the top tier")
	assert.Less(t, pickedLow, 140, "both channels must share the top tier")
}

func TestPerModelPriorityOverrideDoesNotAffectOtherModels(t *testing.T) {
	setupChannelCacheTestDB(t)
	high, low := createPriorityOverrideFixture(t)
	InitChannelCache()

	setModelWeightsForSelectionTest(t, `[{"channel_id":`+strconv.Itoa(low.Id)+`,"model":"shared-model","priority":20}]`)

	for range 200 {
		channel, err := GetRandomSatisfiedChannel("svip", "other-model", 0, nil)
		require.NoError(t, err)
		require.NotNil(t, channel)
		assert.Equal(t, high.Id, channel.Id, "other models keep the channel-level priority")
	}
}

func TestSelectSatisfiedChannelFromCandidatesUsesPerModelPriorityOverride(t *testing.T) {
	setupChannelCacheTestDB(t)
	_, low := createPriorityOverrideFixture(t)
	InitChannelCache()

	setModelWeightsForSelectionTest(t, `[{"channel_id":`+strconv.Itoa(low.Id)+`,"model":"shared-model","priority":20}]`)

	candidates, err := GetSatisfiedChannelsInPriorityOrder("svip", "shared-model", nil)
	require.NoError(t, err)

	pickedLow := 0
	for range 200 {
		channel, err := SelectSatisfiedChannelFromCandidates(candidates, 0, "shared-model")
		require.NoError(t, err)
		require.NotNil(t, channel)
		if channel.Id == low.Id {
			pickedLow++
		}
	}
	assert.Greater(t, pickedLow, 60)
	assert.Less(t, pickedLow, 140)
}

func TestGetChannelWithoutMemoryCacheUsesPerModelPriorityOverride(t *testing.T) {
	setupChannelCacheTestDB(t)
	common.MemoryCacheEnabled = false
	_, low := createPriorityOverrideFixture(t)

	setModelWeightsForSelectionTest(t, `[{"channel_id":`+strconv.Itoa(low.Id)+`,"model":"shared-model","priority":20}]`)

	pickedLow := 0
	for range 200 {
		channel, err := GetChannel("svip", "shared-model", 0, nil)
		require.NoError(t, err)
		require.NotNil(t, channel)
		if channel.Id == low.Id {
			pickedLow++
		}
	}
	assert.Greater(t, pickedLow, 60)
	assert.Less(t, pickedLow, 140)
}
