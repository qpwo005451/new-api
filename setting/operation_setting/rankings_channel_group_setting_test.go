package operation_setting

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRankingsChannelGroups(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{
			name:  "empty value clears configuration",
			value: "",
			valid: true,
		},
		{
			name:  "empty array",
			value: `[]`,
			valid: true,
		},
		{
			name:  "valid groups",
			value: `[{"name":"Acme","channel_ids":[1,2]},{"name":"Beta","channel_ids":[3]}]`,
			valid: true,
		},
		{
			name:  "not json",
			value: `groups`,
			valid: false,
		},
		{
			name:  "json object instead of array",
			value: `{"name":"Acme","channel_ids":[1]}`,
			valid: false,
		},
		{
			name:  "json null",
			value: `null`,
			valid: false,
		},
		{
			name:  "empty group name",
			value: `[{"name":"  ","channel_ids":[1]}]`,
			valid: false,
		},
		{
			name:  "duplicated group name after trim",
			value: `[{"name":"Acme","channel_ids":[1]},{"name":" Acme ","channel_ids":[2]}]`,
			valid: false,
		},
		{
			name:  "duplicate id inside one group",
			value: `[{"name":"Acme","channel_ids":[1,1]}]`,
			valid: false,
		},
		{
			name:  "zero id",
			value: `[{"name":"Acme","channel_ids":[0]}]`,
			valid: false,
		},
		{
			name:  "negative id",
			value: `[{"name":"Acme","channel_ids":[-3]}]`,
			valid: false,
		},
		{
			name:  "same id across groups",
			value: `[{"name":"Acme","channel_ids":[1,2]},{"name":"Beta","channel_ids":[2,3]}]`,
			valid: false,
		},
		{
			name:  "too many groups",
			value: fmt.Sprintf(`[%s]`, strings.TrimSuffix(strings.Repeat(`{"name":"g","channel_ids":[1]},`, MaxRankingsChannelGroups+1), ",")),
			valid: false,
		},
		{
			name:  "too many channel ids in one group",
			value: fmt.Sprintf(`[{"name":"Acme","channel_ids":[%s]}]`, strings.Repeat("1,", MaxRankingsChannelGroupChannels+1)+"2"),
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRankingsChannelGroups(tt.value)
			if tt.valid {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			// The cross-group conflict must name the offending channel and
			// both groups so admins can locate the overlap.
			if strings.Contains(tt.name, "same id across groups") {
				assert.Contains(t, err.Error(), `channel 2 appears in both "Acme" and "Beta"`)
			}
		})
	}
}

func setRankingsChannelGroupsForTest(t *testing.T, value string) {
	t.Helper()
	previous := rankingsChannelGroupSetting.Groups
	rankingsChannelGroupSetting.Groups = value
	t.Cleanup(func() {
		rankingsChannelGroupSetting.Groups = previous
	})
}

func TestGetRankingsChannelGroupsReturnsConfiguredGroups(t *testing.T) {
	setRankingsChannelGroupsForTest(t, `[{"name":"Acme","channel_ids":[1,2]}]`)

	groups := GetRankingsChannelGroups()
	require.Len(t, groups, 1)
	assert.Equal(t, "Acme", groups[0].Name)
	assert.Equal(t, []int{1, 2}, groups[0].ChannelIDs)
}

func TestGetRankingsChannelGroupsDegradesToEmptyWithoutPanic(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "unset", value: ""},
		{name: "whitespace", value: "   "},
		{name: "malformed json", value: `{"name":`},
		{name: "json null", value: `null`},
		{name: "wrong type", value: `"groups"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRankingsChannelGroupsForTest(t, tt.value)
			assert.Empty(t, GetRankingsChannelGroups())
		})
	}
}
