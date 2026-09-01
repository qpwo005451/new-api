package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedRankingChannelQuotaData(t *testing.T) {
	t.Helper()
	rows := []QuotaData{
		// Channel 1 spans two hours inside the window.
		{ModelName: "gpt-a", ChannelID: 1, CreatedAt: 1000, TokenUsed: 10},
		{ModelName: "gpt-b", ChannelID: 1, CreatedAt: 1500, TokenUsed: 15},
		// Channel 2 sits on the window boundary and must be included.
		{ModelName: "gpt-a", ChannelID: 2, CreatedAt: 2000, TokenUsed: 40},
		// Empty model_name with a channel still counts (no model filter here).
		{ModelName: "", ChannelID: 3, CreatedAt: 1200, TokenUsed: 5},
		// A zero-token channel is dropped by the HAVING clause.
		{ModelName: "gpt-a", ChannelID: 4, CreatedAt: 1100, TokenUsed: 0},
		// Rows outside the window must be excluded.
		{ModelName: "gpt-a", ChannelID: 1, CreatedAt: 999, TokenUsed: 999},
		{ModelName: "gpt-a", ChannelID: 2, CreatedAt: 2001, TokenUsed: 999},
	}
	for _, row := range rows {
		require.NoError(t, DB.Create(&row).Error)
	}
}

func TestGetRankingChannelTotalsGroupsByChannelWithinWindow(t *testing.T) {
	truncateTables(t)
	seedRankingChannelQuotaData(t)

	totals, err := GetRankingChannelTotals(1000, 2000)
	require.NoError(t, err)
	// Ordered by total_tokens DESC; channel 4 (zero tokens) is filtered out.
	require.Equal(t, []RankingChannelTotal{
		{ChannelID: 2, TotalTokens: 40},
		{ChannelID: 1, TotalTokens: 25},
		{ChannelID: 3, TotalTokens: 5},
	}, totals)
}

func TestGetRankingChannelTotalsKeepsUnattributedTraffic(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&QuotaData{ModelName: "gpt-a", ChannelID: 0, CreatedAt: 1000, TokenUsed: 7}).Error)

	totals, err := GetRankingChannelTotals(1000, 2000)
	require.NoError(t, err)
	require.Equal(t, []RankingChannelTotal{{ChannelID: 0, TotalTokens: 7}}, totals)
}

func TestGetRankingChannelBucketsGroupByChannelAndHour(t *testing.T) {
	truncateTables(t)
	seedRankingChannelQuotaData(t)

	buckets, err := GetRankingChannelBuckets(1000, 2000, 1000)
	require.NoError(t, err)
	require.ElementsMatch(t, []RankingChannelBucket{
		{ChannelID: 1, Bucket: 1000, Tokens: 25},
		{ChannelID: 3, Bucket: 1000, Tokens: 5},
		{ChannelID: 2, Bucket: 2000, Tokens: 40},
	}, buckets)
	// Buckets are ordered ascending; rows inside one bucket have no
	// guaranteed order across channels.
	for i := 1; i < len(buckets); i++ {
		assert.LessOrEqual(t, buckets[i-1].Bucket, buckets[i].Bucket)
	}
}
