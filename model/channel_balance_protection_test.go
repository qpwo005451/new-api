package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelBalanceProtectionTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &ChannelBalanceProtection{}))

	previousDB := DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})
}

func createBalanceProtectionTestChannel(t *testing.T) *Channel {
	t.Helper()

	channel := &Channel{
		Name:   "input",
		Status: common.ChannelStatusEnabled,
		Models: "free-model,paid-model",
		Group:  "default",
	}
	require.NoError(t, DB.Create(channel).Error)
	return channel
}

func enableBalanceProtectionForTest(t *testing.T, channel *Channel) {
	t.Helper()

	tx := DB.Begin()
	require.NoError(t, tx.Error)
	needsImmediateCheck, err := saveChannelBalanceProtection(tx, channel, &ChannelBalanceProtectionView{
		Enabled:              true,
		TriggerBalance:       2,
		RecoveryBalance:      5,
		CheckIntervalMinutes: 1,
		FreeModels:           []string{"free-model"},
		NotifyEnabled:        true,
	})
	require.NoError(t, err)
	require.True(t, needsImmediateCheck)
	require.NoError(t, tx.Commit().Error)
}

func TestChannelBalanceProtectionHysteresis(t *testing.T) {
	setupChannelBalanceProtectionTestDB(t)
	channel := createBalanceProtectionTestChannel(t)
	enableBalanceProtectionForTest(t, channel)

	protection, err := GetChannelBalanceProtection(channel.Id)
	require.NoError(t, err)
	require.NotNil(t, protection)
	assert.Equal(t, BalanceProtectionStatePending, protection.State)
	assert.True(t, protection.IsActive())

	balance := 1.99
	transition, err := RecordChannelBalanceProtectionCheck(channel.Id, &balance, "")
	require.NoError(t, err)
	assert.Equal(t, BalanceProtectionStateProtected, transition.After.State)

	balance = 3
	transition, err = RecordChannelBalanceProtectionCheck(channel.Id, &balance, "")
	require.NoError(t, err)
	assert.Equal(t, BalanceProtectionStateProtected, transition.After.State)

	balance = 5
	transition, err = RecordChannelBalanceProtectionCheck(channel.Id, &balance, "")
	require.NoError(t, err)
	assert.Equal(t, BalanceProtectionStateNormal, transition.After.State)

	balance = 3
	transition, err = RecordChannelBalanceProtectionCheck(channel.Id, &balance, "")
	require.NoError(t, err)
	assert.Equal(t, BalanceProtectionStateNormal, transition.After.State)

	balance = 1
	transition, err = RecordChannelBalanceProtectionCheck(channel.Id, &balance, "")
	require.NoError(t, err)
	assert.Equal(t, BalanceProtectionStateProtected, transition.After.State)
}

func TestChannelBalanceProtectionEntersUnknownAfterTenFailures(t *testing.T) {
	setupChannelBalanceProtectionTestDB(t)
	channel := createBalanceProtectionTestChannel(t)
	enableBalanceProtectionForTest(t, channel)

	for attempt := 1; attempt <= BalanceProtectionFailureLimit; attempt++ {
		transition, err := RecordChannelBalanceProtectionCheck(channel.Id, nil, "upstream unavailable")
		require.NoError(t, err)
		require.NotNil(t, transition)
		assert.Equal(t, attempt, transition.After.ConsecutiveFailures)
		if attempt < BalanceProtectionFailureLimit {
			assert.Equal(t, BalanceProtectionStatePending, transition.After.State)
		} else {
			assert.Equal(t, BalanceProtectionStateUnknown, transition.After.State)
		}
	}

	balance := 3.0
	transition, err := RecordChannelBalanceProtectionCheck(channel.Id, &balance, "")
	require.NoError(t, err)
	assert.Equal(t, BalanceProtectionStateProtected, transition.After.State)
	assert.Zero(t, transition.After.ConsecutiveFailures)
	assert.Empty(t, transition.After.LastError)
}

func TestChannelBalanceProtectionMatchesCallerModelExactly(t *testing.T) {
	protection := &ChannelBalanceProtection{
		Enabled:    true,
		State:      BalanceProtectionStateProtected,
		FreeModels: `["grok-4.5"]`,
	}

	assert.True(t, protection.AllowsModel("grok-4.5"))
	assert.False(t, protection.AllowsModel("GROK-4.5"))
	assert.False(t, protection.AllowsModel("grok-4.5-free"))
	assert.True(t, protection.AllowsModel(" grok-4.5 "))
}

func TestChannelBalanceProtectionCopyIsDisabledAndDeleteCleansUp(t *testing.T) {
	setupChannelBalanceProtectionTestDB(t)
	source := createBalanceProtectionTestChannel(t)
	enableBalanceProtectionForTest(t, source)
	target := createBalanceProtectionTestChannel(t)

	require.NoError(t, CopyChannelBalanceProtection(source.Id, target.Id))
	copied, err := GetChannelBalanceProtection(target.Id)
	require.NoError(t, err)
	require.NotNil(t, copied)
	assert.False(t, copied.Enabled)
	assert.Equal(t, BalanceProtectionStateDisabled, copied.State)
	assert.Equal(t, []string{"free-model"}, copied.GetFreeModels())

	require.NoError(t, target.Delete())
	deleted, err := GetChannelBalanceProtection(target.Id)
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

func TestIsChannelModelAllowedFailsOpenOnDatabaseReadError(t *testing.T) {
	setupChannelBalanceProtectionTestDB(t)
	channel := createBalanceProtectionTestChannel(t)

	require.NoError(t, DB.Migrator().DropTable(&ChannelBalanceProtection{}))
	assert.True(t, IsChannelModelAllowed(channel.Id, "paid-model"))
}

func TestSaveChannelBalanceProtectionRejectsModelsNotExposedByChannel(t *testing.T) {
	setupChannelBalanceProtectionTestDB(t)
	channel := createBalanceProtectionTestChannel(t)

	tx := DB.Begin()
	require.NoError(t, tx.Error)
	_, err := saveChannelBalanceProtection(tx, channel, &ChannelBalanceProtectionView{
		Enabled:              true,
		TriggerBalance:       2,
		RecoveryBalance:      5,
		CheckIntervalMinutes: 1,
		FreeModels:           []string{"missing-model"},
		NotifyEnabled:        true,
	})
	tx.Rollback()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not exposed by the channel")
}

func TestFilterChannelsForRequestFallsBackForPaidModels(t *testing.T) {
	previousChannels := channelsIDM
	previousProtections := channel2balanceProtection
	channelsIDM = map[int]*Channel{
		1: {Id: 1},
		2: {Id: 2},
	}
	channel2balanceProtection = map[int]*ChannelBalanceProtection{
		1: {
			ChannelId:  1,
			Enabled:    true,
			State:      BalanceProtectionStateProtected,
			FreeModels: `["free-model"]`,
		},
	}
	t.Cleanup(func() {
		channelsIDM = previousChannels
		channel2balanceProtection = previousProtections
	})

	assert.Equal(t, []int{1, 2}, filterChannelsByRequestPathAndModel([]int{1, 2}, "", "free-model"))
	assert.Equal(t, []int{2}, filterChannelsByRequestPathAndModel([]int{1, 2}, "", "paid-model"))
}

func TestFilterAbilitiesForRequestProtectsDirectDatabaseRouting(t *testing.T) {
	setupChannelBalanceProtectionTestDB(t)
	channel := createBalanceProtectionTestChannel(t)
	enableBalanceProtectionForTest(t, channel)

	abilities := []Ability{{ChannelId: channel.Id}}

	assert.Equal(
		t,
		abilities,
		filterAbilitiesByRequestPathAndModel(abilities, "", "free-model"),
	)
	assert.Empty(t, filterAbilitiesByRequestPathAndModel(abilities, "", "paid-model"))
}
