package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
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
	cached, err := GetRandomSatisfiedChannel("svip", "paid-model", 0, "")
	require.NoError(t, err)
	require.NotNil(t, cached)
	assert.Equal(t, channel.Id, cached.Id)

	require.NoError(t, DB.Migrator().DropTable(&Ability{}))
	InitChannelCache()

	cached, err = GetRandomSatisfiedChannel("svip", "paid-model", 0, "")
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
	cached, err := GetRandomSatisfiedChannel("svip", "paid-model", 0, "")
	require.NoError(t, err)
	assert.Nil(t, cached)

	require.NoError(t, DB.Migrator().DropTable(&ChannelBalanceProtection{}))
	InitChannelCache()

	cached, err = GetRandomSatisfiedChannel("svip", "paid-model", 0, "")
	require.NoError(t, err)
	assert.Nil(t, cached)
}
