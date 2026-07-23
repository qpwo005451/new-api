package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelInfoScanSupportsSQLiteTextAndEmptyValues(t *testing.T) {
	tests := []struct {
		name string
		value any
		want ChannelInfo
	}{
		{name: "database null", value: nil},
		{name: "empty bytes", value: []byte{}},
		{name: "whitespace string", value: "  \n\t"},
		{name: "json null", value: []byte("null")},
		{
			name:  "valid json text",
			value: `{"is_multi_key":true,"multi_key_size":2,"multi_key_mode":"polling"}`,
			want: ChannelInfo{
				IsMultiKey:   true,
				MultiKeySize: 2,
				MultiKeyMode: constant.MultiKeyModePolling,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ChannelInfo{
				IsMultiKey:   true,
				MultiKeySize: 2,
			}

			require.NoError(t, info.Scan(tt.value))
			assert.Equal(t, tt.want, info)
		})
	}
}

func TestRecordMultiKeyFailureUsesThresholdAndEnableClearsState(t *testing.T) {
	setupChannelCacheTestDB(t)

	channel := &Channel{
		Name:   "multi-key-failure-test",
		Key:    "key-one\nkey-two",
		Status: common.ChannelStatusEnabled,
		Models: "test-model",
		Group:  "default",
		ChannelInfo: ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 2,
		},
	}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     channel.Group,
		Model:     "test-model",
		ChannelId: channel.Id,
		Enabled:   true,
	}).Error)
	InitChannelCache()

	first, err := RecordMultiKeyFailure(channel.Id, "key-one", 2, "upstream unavailable")
	require.NoError(t, err)
	assert.Equal(t, 1, first.FailureCount)
	assert.False(t, first.KeyAutoDisabled)

	second, err := RecordMultiKeyFailure(channel.Id, "key-one", 2, "upstream unavailable")
	require.NoError(t, err)
	assert.Equal(t, 2, second.FailureCount)
	assert.True(t, second.KeyAutoDisabled)
	assert.False(t, second.ChannelDisabled)

	stored, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.Equal(t, 2, stored.ChannelInfo.MultiKeyFailureCount[0])

	assert.True(t, UpdateChannelStatus(channel.Id, "key-one", common.ChannelStatusEnabled, "recovery succeeded"))

	stored, err = GetChannelById(channel.Id, true)
	require.NoError(t, err)
	_, statusTracked := stored.ChannelInfo.MultiKeyStatusList[0]
	_, failureTracked := stored.ChannelInfo.MultiKeyFailureCount[0]
	assert.False(t, statusTracked)
	assert.False(t, failureTracked)

	_, err = RecordMultiKeyFailure(channel.Id, "key-one", 1, "upstream unavailable")
	require.NoError(t, err)
	_, err = RecordMultiKeyFailure(channel.Id, "key-two", 1, "upstream unavailable")
	require.NoError(t, err)

	stored, err = GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)

	cached, err := GetRandomSatisfiedChannel(channel.Group, "test-model", 0, "")
	require.NoError(t, err)
	assert.Nil(t, cached)

	assert.True(t, UpdateChannelStatus(channel.Id, "key-one", common.ChannelStatusEnabled, "recovery succeeded"))
	cached, err = GetRandomSatisfiedChannel(channel.Group, "test-model", 0, "")
	require.NoError(t, err)
	require.NotNil(t, cached)
	assert.Equal(t, channel.Id, cached.Id)
}
