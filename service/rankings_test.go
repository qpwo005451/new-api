package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRankingChannelDisplayNamesAppliesPriority(t *testing.T) {
	groups := []operation_setting.RankingsChannelGroup{
		{Name: "Acme Group", ChannelIDs: []int{1, 2}},
	}
	dbNames := map[int]string{
		1: "real-acme", // group label must win over the stored channel name
		3: "beta",
		5: "", // channel row exists but has no usable name
		// 7 is absent: deleted channel
	}

	names := rankingChannelDisplayNames([]int{0, 1, 2, 3, 5, 7, 9}, groups, dbNames)

	assert.Equal(t, map[int]string{
		0: rankingOthersLabel, // legacy unattributed traffic
		1: "Acme Group",
		2: "Acme Group",
		3: "beta",
		5: rankingOthersLabel,
		7: rankingOthersLabel,
		9: rankingOthersLabel,
	}, names)
}

func TestBuildRankedChannelsAggregatesByDisplayName(t *testing.T) {
	currentTotals := []model.RankingChannelTotal{
		{ChannelID: 1, TotalTokens: 300},
		{ChannelID: 2, TotalTokens: 100},
		{ChannelID: 3, TotalTokens: 50},
		{ChannelID: 0, TotalTokens: 25},
		{ChannelID: 9, TotalTokens: 10}, // deleted channel, missing from names
	}
	previousTotals := []model.RankingChannelTotal{
		{ChannelID: 1, TotalTokens: 100},
		{ChannelID: 2, TotalTokens: 200},
		{ChannelID: 0, TotalTokens: 25},
	}
	names := map[int]string{
		1: "Acme Group",
		2: "beta",
		3: "deleted-co",
		0: rankingOthersLabel,
	}

	rows := buildRankedChannels(currentTotals, previousTotals, 485, names, true)

	require.Equal(t, []RankedChannel{
		{Rank: 1, ChannelName: "Acme Group", TotalTokens: 300, Share: 0.6186, GrowthPct: 200},
		{Rank: 2, ChannelName: "beta", TotalTokens: 100, Share: 0.2062, GrowthPct: -50},
		{Rank: 3, ChannelName: "deleted-co", TotalTokens: 50, Share: 0.1031, GrowthPct: 100},
		{Rank: 4, ChannelName: rankingOthersLabel, TotalTokens: 35, Share: 0.0722, GrowthPct: 40},
	}, rows)
}

func TestBuildRankedChannelsTieBreaksByNameAndSkipsGrowthWhenDisabled(t *testing.T) {
	currentTotals := []model.RankingChannelTotal{
		{ChannelID: 5, TotalTokens: 100},
		{ChannelID: 4, TotalTokens: 100},
	}
	previousTotals := []model.RankingChannelTotal{
		{ChannelID: 5, TotalTokens: 50},
		{ChannelID: 4, TotalTokens: 50},
	}
	names := map[int]string{4: "b-channel", 5: "a-channel"}

	rows := buildRankedChannels(currentTotals, previousTotals, 200, names, false)

	// Equal tokens resolve alphabetically and growth stays zeroed.
	require.Equal(t, []RankedChannel{
		{Rank: 1, ChannelName: "a-channel", TotalTokens: 100, Share: 0.5, GrowthPct: 0},
		{Rank: 2, ChannelName: "b-channel", TotalTokens: 100, Share: 0.5, GrowthPct: 0},
	}, rows)
}

func TestBuildChannelShareHistoryBucketsTopChannelsAndOthers(t *testing.T) {
	config := rankingPeriodConfig{id: "today", duration: 24 * time.Hour, bucketSize: 3600, labelLayout: "15:04", hasPrevious: true}
	channels := []RankedChannel{
		{Rank: 1, ChannelName: "alpha", TotalTokens: 500, Share: 0.3226},
		{Rank: 2, ChannelName: "beta", TotalTokens: 400, Share: 0.2581},
		{Rank: 3, ChannelName: "gamma", TotalTokens: 300, Share: 0.1935},
		{Rank: 4, ChannelName: "delta", TotalTokens: 200, Share: 0.129},
		{Rank: 5, ChannelName: "epsilon", TotalTokens: 100, Share: 0.0645},
		{Rank: 6, ChannelName: "zeta", TotalTokens: 50, Share: 0.0323},
	}
	buckets := []model.RankingChannelBucket{
		{ChannelID: 1, Bucket: 3600, Tokens: 100},
		{ChannelID: 6, Bucket: 3600, Tokens: 30}, // zeta lands in Others
		{ChannelID: 2, Bucket: 7200, Tokens: 80},
		{ChannelID: 1, Bucket: 7200, Tokens: 40},
		{ChannelID: 7, Bucket: 10800, Tokens: 60}, // unknown id lands in Others
	}
	names := map[int]string{
		1: "alpha", 2: "beta", 3: "gamma", 4: "delta", 5: "epsilon", 6: "zeta",
	}

	history := buildChannelShareHistory(buckets, channels, 1550, names, config)

	require.Equal(t, []ChannelShareChannel{
		{Name: "alpha", Total: 500, Share: 0.3226},
		{Name: "beta", Total: 400, Share: 0.2581},
		{Name: "gamma", Total: 300, Share: 0.1935},
		{Name: "delta", Total: 200, Share: 0.129},
		{Name: "epsilon", Total: 100, Share: 0.0645},
		{Name: rankingOthersLabel, Total: 50, Share: 0.0323},
	}, history.Channels)
	require.Equal(t, 3, history.Buckets)
	// The label format follows the period config the same way the vendor
	// history does; compute it the same way to stay timezone-independent.
	labelFor := func(bucket int64) string {
		return time.Unix(bucket, 0).Format(config.labelLayout)
	}
	require.Equal(t, []ChannelSharePoint{
		{Ts: "1970-01-01T01:00:00Z", Label: labelFor(3600), Channel: "alpha", Share: 0.7692, Tokens: 100},
		{Ts: "1970-01-01T01:00:00Z", Label: labelFor(3600), Channel: rankingOthersLabel, Share: 0.2308, Tokens: 30},
		{Ts: "1970-01-01T02:00:00Z", Label: labelFor(7200), Channel: "alpha", Share: 0.3333, Tokens: 40},
		{Ts: "1970-01-01T02:00:00Z", Label: labelFor(7200), Channel: "beta", Share: 0.6667, Tokens: 80},
		{Ts: "1970-01-01T03:00:00Z", Label: labelFor(10800), Channel: rankingOthersLabel, Share: 1, Tokens: 60},
	}, history.Points)
}
