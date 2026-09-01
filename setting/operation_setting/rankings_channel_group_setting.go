package operation_setting

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

const (
	MaxRankingsChannelGroups        = 50
	MaxRankingsChannelGroupChannels = 500
)

// RankingsChannelGroup merges several channel IDs under one display name in
// the public rankings, so operators can hide how traffic splits across
// individual upstream channels.
type RankingsChannelGroup struct {
	Name       string `json:"name"`
	ChannelIDs []int  `json:"channel_ids"`
}

// RankingsChannelGroupSetting keeps the groups as a raw JSON array string so
// the config registry persists it like the other JSON-array options.
type RankingsChannelGroupSetting struct {
	Groups string `json:"groups"` // JSON array of RankingsChannelGroup
}

var rankingsChannelGroupSetting = RankingsChannelGroupSetting{Groups: "[]"}

func init() {
	config.GlobalConfig.Register("rankings_channel_group_setting", &rankingsChannelGroupSetting)
}

// GetRankingsChannelGroups returns the configured groups. An unset or
// malformed value degrades to an empty list so the rankings fall back to raw
// channel names instead of failing the whole snapshot.
func GetRankingsChannelGroups() []RankingsChannelGroup {
	groups := []RankingsChannelGroup{}
	if strings.TrimSpace(rankingsChannelGroupSetting.Groups) == "" {
		return groups
	}
	var parsed []RankingsChannelGroup
	if err := common.UnmarshalJsonStr(rankingsChannelGroupSetting.Groups, &parsed); err != nil {
		return groups
	}
	if parsed == nil {
		return groups
	}
	return parsed
}

// ValidateRankingsChannelGroups enforces the invariants the rankings
// aggregation relies on: a bounded group count, unique trimmed group names,
// unique positive channel IDs within each group, and no channel appearing in
// more than one group (a shared channel would make its tokens show up under
// two display names).
func ValidateRankingsChannelGroups(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var groups []RankingsChannelGroup
	if err := common.UnmarshalJsonStr(value, &groups); err != nil {
		return fmt.Errorf("rankings channel groups must be a JSON array: %w", err)
	}
	if groups == nil {
		// "null" unmarshals into a nil slice without error but is not an array.
		return errors.New("rankings channel groups must be a JSON array")
	}
	if len(groups) > MaxRankingsChannelGroups {
		return fmt.Errorf("too many rankings channel groups (max %d)", MaxRankingsChannelGroups)
	}
	nameSet := make(map[string]struct{}, len(groups))
	channelOwner := make(map[int]string)
	for idx, group := range groups {
		name := strings.TrimSpace(group.Name)
		if name == "" {
			return fmt.Errorf("rankings channel group %d requires a name", idx+1)
		}
		if _, exists := nameSet[name]; exists {
			return fmt.Errorf("rankings channel group name %q is duplicated", name)
		}
		nameSet[name] = struct{}{}
		if len(group.ChannelIDs) > MaxRankingsChannelGroupChannels {
			return fmt.Errorf("rankings channel group %q has too many channel IDs (max %d)", name, MaxRankingsChannelGroupChannels)
		}
		idSet := make(map[int]struct{}, len(group.ChannelIDs))
		for _, channelID := range group.ChannelIDs {
			if channelID <= 0 {
				return fmt.Errorf("rankings channel group %q contains invalid channel id %d", name, channelID)
			}
			if _, exists := idSet[channelID]; exists {
				return fmt.Errorf("rankings channel group %q contains duplicate channel id %d", name, channelID)
			}
			idSet[channelID] = struct{}{}
			if owner, exists := channelOwner[channelID]; exists {
				return fmt.Errorf("channel %d appears in both %q and %q", channelID, owner, name)
			}
			channelOwner[channelID] = name
		}
	}
	return nil
}
