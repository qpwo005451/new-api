package service

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const (
	rankingCacheTTL         = 5 * time.Minute
	rankingLeaderboardLimit = 20
	rankingHistoryLimit     = 10
	rankingVendorLimit      = 5
	rankingMoverLimit       = 6
	rankingChannelLimit     = 5
	rankingOthersLabel      = "Others"
	rankingUnknownVendor    = "Unknown"
)

type RankingsResponse struct {
	Models              []RankedModel       `json:"models"`
	Vendors             []RankedVendor      `json:"vendors"`
	Channels            []RankedChannel     `json:"channels"`
	TopMovers           []RankingMover      `json:"top_movers"`
	TopDroppers         []RankingMover      `json:"top_droppers"`
	ModelsHistory       ModelHistorySeries  `json:"models_history"`
	VendorShareHistory  VendorShareSeries   `json:"vendor_share_history"`
	ChannelShareHistory ChannelShareHistory `json:"channel_share_history"`
}

type RankedModel struct {
	Rank         int     `json:"rank"`
	PreviousRank *int    `json:"previous_rank,omitempty"`
	ModelName    string  `json:"model_name"`
	Vendor       string  `json:"vendor"`
	VendorIcon   string  `json:"vendor_icon,omitempty"`
	Category     string  `json:"category"`
	TotalTokens  int64   `json:"total_tokens"`
	Share        float64 `json:"share"`
	GrowthPct    float64 `json:"growth_pct"`
}

type RankedVendor struct {
	Rank        int     `json:"rank"`
	Vendor      string  `json:"vendor"`
	VendorIcon  string  `json:"vendor_icon,omitempty"`
	TotalTokens int64   `json:"total_tokens"`
	Share       float64 `json:"share"`
	GrowthPct   float64 `json:"growth_pct"`
	ModelsCount int     `json:"models_count"`
	TopModel    string  `json:"top_model"`
}

type RankingMover struct {
	ModelName   string  `json:"model_name"`
	Vendor      string  `json:"vendor"`
	VendorIcon  string  `json:"vendor_icon,omitempty"`
	RankDelta   int     `json:"rank_delta"`
	CurrentRank int     `json:"current_rank"`
	GrowthPct   float64 `json:"growth_pct"`
}

type ModelHistoryPoint struct {
	Ts     string `json:"ts"`
	Label  string `json:"label"`
	Model  string `json:"model"`
	Vendor string `json:"vendor"`
	Tokens int64  `json:"tokens"`
}

type ModelHistoryModel struct {
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
	Total  int64  `json:"total"`
}

type ModelHistorySeries struct {
	Points  []ModelHistoryPoint `json:"points"`
	Models  []ModelHistoryModel `json:"models"`
	Buckets int                 `json:"buckets"`
}

type VendorSharePoint struct {
	Ts     string  `json:"ts"`
	Label  string  `json:"label"`
	Vendor string  `json:"vendor"`
	Share  float64 `json:"share"`
	Tokens int64   `json:"tokens"`
}

type VendorShareVendor struct {
	Name  string  `json:"name"`
	Total int64   `json:"total"`
	Share float64 `json:"share"`
}

type VendorShareSeries struct {
	Points  []VendorSharePoint  `json:"points"`
	Vendors []VendorShareVendor `json:"vendors"`
	Buckets int                 `json:"buckets"`
}

// Channel share series mirror the vendor share series shape: one row per
// displayed channel plus an aggregated "Others" entry, and per-bucket points.
type ChannelSharePoint struct {
	Ts      string  `json:"ts"`
	Label   string  `json:"label"`
	Channel string  `json:"channel"`
	Share   float64 `json:"share"`
	Tokens  int64   `json:"tokens"`
}

type ChannelShareChannel struct {
	Name  string  `json:"name"`
	Total int64   `json:"total"`
	Share float64 `json:"share"`
}

type ChannelShareHistory struct {
	Points   []ChannelSharePoint   `json:"points"`
	Channels []ChannelShareChannel `json:"channels"`
	Buckets  int                   `json:"buckets"`
}

// RankedChannel aggregates quota_data rows by the resolved display name of
// channel_id, so grouped channels appear once under their group label.
type RankedChannel struct {
	Rank        int     `json:"rank"`
	ChannelName string  `json:"channel_name"`
	TotalTokens int64   `json:"total_tokens"`
	Share       float64 `json:"share"`
	GrowthPct   float64 `json:"growth_pct"`
}

type rankingPeriodConfig struct {
	id          string
	duration    time.Duration
	bucketSize  int64
	labelLayout string
	hasPrevious bool
}

type rankingCacheItem struct {
	expiresAt time.Time
	data      *RankingsResponse
}

type rankingModelMeta struct {
	vendor     string
	vendorIcon string
}

type vendorAggregate struct {
	name           string
	icon           string
	totalTokens    int64
	previousTokens int64
	models         map[string]struct{}
	topModel       string
	topModelTokens int64
}

type channelAggregate struct {
	name           string
	totalTokens    int64
	previousTokens int64
}

var (
	rankingCacheMu sync.Mutex
	rankingCache   = map[string]rankingCacheItem{}
)

func GetRankingsSnapshot(period string) (*RankingsResponse, error) {
	config, err := rankingConfig(period)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	rankingCacheMu.Lock()
	if item, ok := rankingCache[config.id]; ok && now.Before(item.expiresAt) {
		rankingCacheMu.Unlock()
		return item.data, nil
	}
	rankingCacheMu.Unlock()

	data, err := buildRankingsSnapshot(config, now)
	if err != nil {
		return nil, err
	}

	rankingCacheMu.Lock()
	rankingCache[config.id] = rankingCacheItem{
		expiresAt: now.Add(rankingCacheTTL),
		data:      data,
	}
	rankingCacheMu.Unlock()

	return data, nil
}

func rankingConfig(period string) (rankingPeriodConfig, error) {
	switch period {
	case "", "week":
		return rankingPeriodConfig{id: "week", duration: 7 * 24 * time.Hour, bucketSize: 24 * 3600, labelLayout: "Jan 2", hasPrevious: true}, nil
	case "today":
		return rankingPeriodConfig{id: "today", duration: 24 * time.Hour, bucketSize: 3600, labelLayout: "15:04", hasPrevious: true}, nil
	case "month":
		return rankingPeriodConfig{id: "month", duration: 30 * 24 * time.Hour, bucketSize: 24 * 3600, labelLayout: "Jan 2", hasPrevious: true}, nil
	case "year":
		return rankingPeriodConfig{id: "year", duration: 365 * 24 * time.Hour, bucketSize: 7 * 24 * 3600, labelLayout: "Jan 2", hasPrevious: true}, nil
	default:
		return rankingPeriodConfig{}, fmt.Errorf("invalid ranking period: %s", period)
	}
}

func buildRankingsSnapshot(config rankingPeriodConfig, now time.Time) (*RankingsResponse, error) {
	startTime, endTime := rankingTimeRange(config, now)
	currentTotals, err := model.GetRankingQuotaTotals(startTime, endTime)
	if err != nil {
		return nil, err
	}
	currentBuckets, err := model.GetRankingQuotaBuckets(startTime, endTime, config.bucketSize)
	if err != nil {
		return nil, err
	}

	var previousTotals []model.RankingQuotaTotal
	var previousChannelTotals []model.RankingChannelTotal
	if config.hasPrevious {
		previousStart, previousEnd := previousRankingTimeRange(config, startTime)
		previousTotals, err = model.GetRankingQuotaTotals(previousStart, previousEnd)
		if err != nil {
			return nil, err
		}
		previousChannelTotals, err = model.GetRankingChannelTotals(previousStart, previousEnd)
		if err != nil {
			return nil, err
		}
	}

	currentChannelTotals, err := model.GetRankingChannelTotals(startTime, endTime)
	if err != nil {
		return nil, err
	}
	currentChannelBuckets, err := model.GetRankingChannelBuckets(startTime, endTime, config.bucketSize)
	if err != nil {
		return nil, err
	}

	meta := buildRankingModelMeta()
	totalTokens := sumRankingTokens(currentTotals)
	previousRankByModel := rankingRankMap(previousTotals)
	previousTokensByModel := rankingTokenMap(previousTotals)

	channelIDSet := make(map[int]struct{})
	for _, item := range currentChannelTotals {
		channelIDSet[item.ChannelID] = struct{}{}
	}
	for _, item := range previousChannelTotals {
		channelIDSet[item.ChannelID] = struct{}{}
	}
	channelIDs := make([]int, 0, len(channelIDSet))
	for id := range channelIDSet {
		channelIDs = append(channelIDs, id)
	}
	channelNames := resolveRankingChannelNames(channelIDs)

	rankedModels := buildRankedModels(currentTotals, totalTokens, previousRankByModel, previousTokensByModel, meta, config.hasPrevious)
	vendors := buildRankedVendors(currentTotals, previousTotals, totalTokens, meta, config.hasPrevious)
	modelHistory := buildModelHistory(currentBuckets, currentTotals, meta, config)
	vendorHistory := buildVendorShareHistory(currentBuckets, vendors, totalTokens, meta, config)
	rankedChannels := buildRankedChannels(currentChannelTotals, previousChannelTotals, totalTokens, channelNames, config.hasPrevious)
	channelHistory := buildChannelShareHistory(currentChannelBuckets, rankedChannels, totalTokens, channelNames, config)
	movers, droppers := buildRankingMovers(rankedModels)

	return &RankingsResponse{
		Models:              limitRankedModels(rankedModels, rankingLeaderboardLimit),
		Vendors:             vendors,
		Channels:            rankedChannels,
		TopMovers:           movers,
		TopDroppers:         droppers,
		ModelsHistory:       modelHistory,
		VendorShareHistory:  vendorHistory,
		ChannelShareHistory: channelHistory,
	}, nil
}

func rankingTimeRange(config rankingPeriodConfig, now time.Time) (int64, int64) {
	endTime := now.Unix()
	if config.duration <= 0 {
		return 0, endTime
	}
	return now.Add(-config.duration).Unix(), endTime
}

func previousRankingTimeRange(config rankingPeriodConfig, currentStart int64) (int64, int64) {
	previousEnd := currentStart - 1
	previousStart := time.Unix(currentStart, 0).Add(-config.duration).Unix()
	return previousStart, previousEnd
}

func buildRankingModelMeta() map[string]rankingModelMeta {
	vendorByID := make(map[int]model.PricingVendor)
	for _, vendor := range model.GetVendors() {
		vendorByID[vendor.ID] = vendor
	}

	meta := make(map[string]rankingModelMeta)
	for _, pricing := range model.GetPricing() {
		item := rankingModelMeta{vendor: rankingUnknownVendor}
		if vendor, ok := vendorByID[pricing.VendorID]; ok {
			item.vendor = vendor.Name
			item.vendorIcon = vendor.Icon
		} else if pricing.OwnerBy != "" {
			item.vendor = pricing.OwnerBy
		}
		meta[pricing.ModelName] = item
	}
	return meta
}

func modelMeta(modelName string, meta map[string]rankingModelMeta) rankingModelMeta {
	if item, ok := meta[modelName]; ok && item.vendor != "" {
		return item
	}
	return rankingModelMeta{vendor: rankingUnknownVendor}
}

func buildRankedModels(totals []model.RankingQuotaTotal, totalTokens int64, previousRanks map[string]int, previousTokens map[string]int64, meta map[string]rankingModelMeta, showGrowth bool) []RankedModel {
	rows := make([]RankedModel, 0, len(totals))
	for idx, item := range totals {
		modelMeta := modelMeta(item.ModelName, meta)
		var previousRank *int
		if rank, ok := previousRanks[item.ModelName]; ok {
			rankCopy := rank
			previousRank = &rankCopy
		}
		growth := 0.0
		if showGrowth {
			growth = rankingGrowthPct(item.TotalTokens, previousTokens[item.ModelName])
		}
		rows = append(rows, RankedModel{
			Rank:         idx + 1,
			PreviousRank: previousRank,
			ModelName:    item.ModelName,
			Vendor:       modelMeta.vendor,
			VendorIcon:   modelMeta.vendorIcon,
			Category:     "all",
			TotalTokens:  item.TotalTokens,
			Share:        rankingShare(item.TotalTokens, totalTokens),
			GrowthPct:    growth,
		})
	}
	return rows
}

func buildRankedVendors(currentTotals []model.RankingQuotaTotal, previousTotals []model.RankingQuotaTotal, totalTokens int64, meta map[string]rankingModelMeta, showGrowth bool) []RankedVendor {
	aggregates := make(map[string]*vendorAggregate)
	for _, item := range currentTotals {
		modelMeta := modelMeta(item.ModelName, meta)
		agg := ensureVendorAggregate(aggregates, modelMeta)
		agg.totalTokens += item.TotalTokens
		agg.models[item.ModelName] = struct{}{}
		if item.TotalTokens > agg.topModelTokens {
			agg.topModel = item.ModelName
			agg.topModelTokens = item.TotalTokens
		}
	}
	for _, item := range previousTotals {
		modelMeta := modelMeta(item.ModelName, meta)
		agg := ensureVendorAggregate(aggregates, modelMeta)
		agg.previousTokens += item.TotalTokens
	}

	rows := make([]RankedVendor, 0, len(aggregates))
	for _, agg := range aggregates {
		if agg.totalTokens <= 0 {
			continue
		}
		growth := 0.0
		if showGrowth {
			growth = rankingGrowthPct(agg.totalTokens, agg.previousTokens)
		}
		rows = append(rows, RankedVendor{
			Vendor:      agg.name,
			VendorIcon:  agg.icon,
			TotalTokens: agg.totalTokens,
			Share:       rankingShare(agg.totalTokens, totalTokens),
			GrowthPct:   growth,
			ModelsCount: len(agg.models),
			TopModel:    agg.topModel,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalTokens == rows[j].TotalTokens {
			return rows[i].Vendor < rows[j].Vendor
		}
		return rows[i].TotalTokens > rows[j].TotalTokens
	})
	for idx := range rows {
		rows[idx].Rank = idx + 1
	}
	return rows
}

func ensureVendorAggregate(aggregates map[string]*vendorAggregate, meta rankingModelMeta) *vendorAggregate {
	name := meta.vendor
	if name == "" {
		name = rankingUnknownVendor
	}
	agg, ok := aggregates[name]
	if !ok {
		agg = &vendorAggregate{
			name:   name,
			icon:   meta.vendorIcon,
			models: make(map[string]struct{}),
		}
		aggregates[name] = agg
	}
	if agg.icon == "" && meta.vendorIcon != "" {
		agg.icon = meta.vendorIcon
	}
	return agg
}

func buildModelHistory(buckets []model.RankingQuotaBucket, totals []model.RankingQuotaTotal, meta map[string]rankingModelMeta, config rankingPeriodConfig) ModelHistorySeries {
	topModels := make(map[string]struct{})
	models := make([]ModelHistoryModel, 0, minInt(len(totals), rankingHistoryLimit)+1)
	otherTotal := int64(0)
	for idx, item := range totals {
		if idx < rankingHistoryLimit {
			topModels[item.ModelName] = struct{}{}
			modelMeta := modelMeta(item.ModelName, meta)
			models = append(models, ModelHistoryModel{Name: item.ModelName, Vendor: modelMeta.vendor, Total: item.TotalTokens})
			continue
		}
		otherTotal += item.TotalTokens
	}
	if otherTotal > 0 {
		models = append(models, ModelHistoryModel{Name: rankingOthersLabel, Vendor: "Various", Total: otherTotal})
	}

	bucketSet := make(map[int64]struct{})
	tokensByBucketAndModel := make(map[int64]map[string]int64)
	for _, item := range buckets {
		modelName := item.ModelName
		if _, ok := topModels[modelName]; !ok {
			modelName = rankingOthersLabel
		}
		bucketSet[item.Bucket] = struct{}{}
		if _, ok := tokensByBucketAndModel[item.Bucket]; !ok {
			tokensByBucketAndModel[item.Bucket] = make(map[string]int64)
		}
		tokensByBucketAndModel[item.Bucket][modelName] += item.Tokens
	}

	sortedBuckets := sortedRankingBuckets(bucketSet)
	points := make([]ModelHistoryPoint, 0, len(sortedBuckets)*len(models))
	for _, bucket := range sortedBuckets {
		for _, historyModel := range models {
			tokens := tokensByBucketAndModel[bucket][historyModel.Name]
			if tokens <= 0 {
				continue
			}
			points = append(points, ModelHistoryPoint{
				Ts:     rankingBucketTs(bucket),
				Label:  rankingBucketLabel(bucket, config),
				Model:  historyModel.Name,
				Vendor: historyModel.Vendor,
				Tokens: tokens,
			})
		}
	}

	return ModelHistorySeries{
		Points:  points,
		Models:  models,
		Buckets: len(sortedBuckets),
	}
}

func buildVendorShareHistory(buckets []model.RankingQuotaBucket, vendors []RankedVendor, totalTokens int64, meta map[string]rankingModelMeta, config rankingPeriodConfig) VendorShareSeries {
	topVendors := make(map[string]struct{})
	vendorRows := make([]VendorShareVendor, 0, minInt(len(vendors), rankingVendorLimit)+1)
	otherTotal := int64(0)
	for idx, vendor := range vendors {
		if idx < rankingVendorLimit {
			topVendors[vendor.Vendor] = struct{}{}
			vendorRows = append(vendorRows, VendorShareVendor{Name: vendor.Vendor, Total: vendor.TotalTokens, Share: vendor.Share})
			continue
		}
		otherTotal += vendor.TotalTokens
	}
	if otherTotal > 0 {
		vendorRows = append(vendorRows, VendorShareVendor{Name: rankingOthersLabel, Total: otherTotal, Share: rankingShare(otherTotal, totalTokens)})
	}

	bucketSet := make(map[int64]struct{})
	tokensByBucketAndVendor := make(map[int64]map[string]int64)
	totalsByBucket := make(map[int64]int64)
	for _, item := range buckets {
		modelMeta := modelMeta(item.ModelName, meta)
		vendorName := modelMeta.vendor
		if _, ok := topVendors[vendorName]; !ok {
			vendorName = rankingOthersLabel
		}
		bucketSet[item.Bucket] = struct{}{}
		if _, ok := tokensByBucketAndVendor[item.Bucket]; !ok {
			tokensByBucketAndVendor[item.Bucket] = make(map[string]int64)
		}
		tokensByBucketAndVendor[item.Bucket][vendorName] += item.Tokens
		totalsByBucket[item.Bucket] += item.Tokens
	}

	sortedBuckets := sortedRankingBuckets(bucketSet)
	points := make([]VendorSharePoint, 0, len(sortedBuckets)*len(vendorRows))
	for _, bucket := range sortedBuckets {
		for _, vendor := range vendorRows {
			tokens := tokensByBucketAndVendor[bucket][vendor.Name]
			if tokens <= 0 {
				continue
			}
			points = append(points, VendorSharePoint{
				Ts:     rankingBucketTs(bucket),
				Label:  rankingBucketLabel(bucket, config),
				Vendor: vendor.Name,
				Share:  rankingShare(tokens, totalsByBucket[bucket]),
				Tokens: tokens,
			})
		}
	}

	return VendorShareSeries{
		Points:  points,
		Vendors: vendorRows,
		Buckets: len(sortedBuckets),
	}
}

// resolveRankingChannelNames maps every ranking channel ID to its display
// name. Admin-configured channel groups win over the stored channel name, and
// IDs that cannot be resolved (deleted channels, legacy channel_id = 0 rows)
// collapse into the shared "Others" bucket.
func resolveRankingChannelNames(ids []int) map[int]string {
	groups := operation_setting.GetRankingsChannelGroups()
	grouped := make(map[int]struct{})
	for _, group := range groups {
		for _, channelID := range group.ChannelIDs {
			grouped[channelID] = struct{}{}
		}
	}

	pending := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := grouped[id]; ok {
			continue
		}
		pending = append(pending, id)
	}

	dbNames := make(map[int]string, len(pending))
	if len(pending) > 0 {
		if common.MemoryCacheEnabled {
			for _, id := range pending {
				if channel, err := model.CacheGetChannel(id); err == nil && channel != nil {
					dbNames[id] = channel.Name
				}
			}
		} else if channels, err := model.GetChannelsByIds(pending); err == nil {
			for _, channel := range channels {
				dbNames[channel.Id] = channel.Name
			}
		} else {
			// Display names are cosmetic for the leaderboard; a lookup failure
			// degrades those channels into "Others" instead of failing the
			// whole snapshot.
			common.SysError("failed to resolve ranking channel names: " + err.Error())
		}
	}

	return rankingChannelDisplayNames(ids, groups, dbNames)
}

// rankingChannelDisplayNames applies the rankings naming policy in priority
// order: group label first, then the stored channel name, then the shared
// "Others" bucket so the leaderboard never leaks stale channel references.
func rankingChannelDisplayNames(ids []int, groups []operation_setting.RankingsChannelGroup, dbNames map[int]string) map[int]string {
	groupNameByID := make(map[int]string)
	for _, group := range groups {
		for _, channelID := range group.ChannelIDs {
			groupNameByID[channelID] = group.Name
		}
	}

	names := make(map[int]string, len(ids))
	for _, id := range ids {
		if name, ok := groupNameByID[id]; ok && name != "" {
			names[id] = name
			continue
		}
		if name := dbNames[id]; name != "" {
			names[id] = name
			continue
		}
		names[id] = rankingOthersLabel
	}
	return names
}

func channelRankingName(channelID int, names map[int]string) string {
	if name, ok := names[channelID]; ok && name != "" {
		return name
	}
	return rankingOthersLabel
}

func ensureChannelAggregate(aggregates map[string]*channelAggregate, name string) *channelAggregate {
	agg, ok := aggregates[name]
	if !ok {
		agg = &channelAggregate{name: name}
		aggregates[name] = agg
	}
	return agg
}

func buildRankedChannels(currentTotals []model.RankingChannelTotal, previousTotals []model.RankingChannelTotal, totalTokens int64, names map[int]string, showGrowth bool) []RankedChannel {
	aggregates := make(map[string]*channelAggregate)
	for _, item := range currentTotals {
		agg := ensureChannelAggregate(aggregates, channelRankingName(item.ChannelID, names))
		agg.totalTokens += item.TotalTokens
	}
	for _, item := range previousTotals {
		agg := ensureChannelAggregate(aggregates, channelRankingName(item.ChannelID, names))
		agg.previousTokens += item.TotalTokens
	}

	rows := make([]RankedChannel, 0, len(aggregates))
	for _, agg := range aggregates {
		if agg.totalTokens <= 0 {
			continue
		}
		growth := 0.0
		if showGrowth {
			growth = rankingGrowthPct(agg.totalTokens, agg.previousTokens)
		}
		rows = append(rows, RankedChannel{
			ChannelName: agg.name,
			TotalTokens: agg.totalTokens,
			Share:       rankingShare(agg.totalTokens, totalTokens),
			GrowthPct:   growth,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].TotalTokens == rows[j].TotalTokens {
			return rows[i].ChannelName < rows[j].ChannelName
		}
		return rows[i].TotalTokens > rows[j].TotalTokens
	})
	for idx := range rows {
		rows[idx].Rank = idx + 1
	}
	return rows
}

func buildChannelShareHistory(buckets []model.RankingChannelBucket, channels []RankedChannel, totalTokens int64, names map[int]string, config rankingPeriodConfig) ChannelShareHistory {
	topChannels := make(map[string]struct{})
	channelRows := make([]ChannelShareChannel, 0, minInt(len(channels), rankingChannelLimit)+1)
	otherTotal := int64(0)
	for idx, channel := range channels {
		if idx < rankingChannelLimit {
			topChannels[channel.ChannelName] = struct{}{}
			channelRows = append(channelRows, ChannelShareChannel{Name: channel.ChannelName, Total: channel.TotalTokens, Share: channel.Share})
			continue
		}
		otherTotal += channel.TotalTokens
	}
	if otherTotal > 0 {
		channelRows = append(channelRows, ChannelShareChannel{Name: rankingOthersLabel, Total: otherTotal, Share: rankingShare(otherTotal, totalTokens)})
	}

	bucketSet := make(map[int64]struct{})
	tokensByBucketAndChannel := make(map[int64]map[string]int64)
	totalsByBucket := make(map[int64]int64)
	for _, item := range buckets {
		channelName := channelRankingName(item.ChannelID, names)
		if _, ok := topChannels[channelName]; !ok {
			channelName = rankingOthersLabel
		}
		bucketSet[item.Bucket] = struct{}{}
		if _, ok := tokensByBucketAndChannel[item.Bucket]; !ok {
			tokensByBucketAndChannel[item.Bucket] = make(map[string]int64)
		}
		tokensByBucketAndChannel[item.Bucket][channelName] += item.Tokens
		totalsByBucket[item.Bucket] += item.Tokens
	}

	sortedBuckets := sortedRankingBuckets(bucketSet)
	points := make([]ChannelSharePoint, 0, len(sortedBuckets)*len(channelRows))
	for _, bucket := range sortedBuckets {
		for _, channel := range channelRows {
			tokens := tokensByBucketAndChannel[bucket][channel.Name]
			if tokens <= 0 {
				continue
			}
			points = append(points, ChannelSharePoint{
				Ts:      rankingBucketTs(bucket),
				Label:   rankingBucketLabel(bucket, config),
				Channel: channel.Name,
				Share:   rankingShare(tokens, totalsByBucket[bucket]),
				Tokens:  tokens,
			})
		}
	}

	return ChannelShareHistory{
		Points:   points,
		Channels: channelRows,
		Buckets:  len(sortedBuckets),
	}
}

func buildRankingMovers(models []RankedModel) ([]RankingMover, []RankingMover) {
	movers := make([]RankingMover, 0)
	droppers := make([]RankingMover, 0)
	for _, item := range models {
		if item.PreviousRank == nil {
			continue
		}
		delta := *item.PreviousRank - item.Rank
		if delta == 0 {
			continue
		}
		row := RankingMover{
			ModelName:   item.ModelName,
			Vendor:      item.Vendor,
			VendorIcon:  item.VendorIcon,
			RankDelta:   delta,
			CurrentRank: item.Rank,
			GrowthPct:   item.GrowthPct,
		}
		if delta > 0 {
			movers = append(movers, row)
		} else {
			droppers = append(droppers, row)
		}
	}
	sort.Slice(movers, func(i, j int) bool {
		if movers[i].RankDelta == movers[j].RankDelta {
			return movers[i].GrowthPct > movers[j].GrowthPct
		}
		return movers[i].RankDelta > movers[j].RankDelta
	})
	sort.Slice(droppers, func(i, j int) bool {
		if droppers[i].RankDelta == droppers[j].RankDelta {
			return droppers[i].GrowthPct < droppers[j].GrowthPct
		}
		return droppers[i].RankDelta < droppers[j].RankDelta
	})
	return limitRankingMovers(movers, rankingMoverLimit), limitRankingMovers(droppers, rankingMoverLimit)
}

func sortedRankingBuckets(bucketSet map[int64]struct{}) []int64 {
	buckets := make([]int64, 0, len(bucketSet))
	for bucket := range bucketSet {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i] < buckets[j]
	})
	return buckets
}

func rankingBucketTs(bucket int64) string {
	return time.Unix(bucket, 0).UTC().Format(time.RFC3339)
}

func rankingBucketLabel(bucket int64, config rankingPeriodConfig) string {
	return time.Unix(bucket, 0).Format(config.labelLayout)
}

func rankingRankMap(totals []model.RankingQuotaTotal) map[string]int {
	ranks := make(map[string]int, len(totals))
	for idx, item := range totals {
		ranks[item.ModelName] = idx + 1
	}
	return ranks
}

func rankingTokenMap(totals []model.RankingQuotaTotal) map[string]int64 {
	tokens := make(map[string]int64, len(totals))
	for _, item := range totals {
		tokens[item.ModelName] = item.TotalTokens
	}
	return tokens
}

func sumRankingTokens(totals []model.RankingQuotaTotal) int64 {
	total := int64(0)
	for _, item := range totals {
		total += item.TotalTokens
	}
	return total
}

func rankingShare(value int64, total int64) float64 {
	if total <= 0 || value <= 0 {
		return 0
	}
	return roundRankingFloat(float64(value) / float64(total))
}

func rankingGrowthPct(current int64, previous int64) float64 {
	if previous <= 0 {
		if current > 0 {
			return 100
		}
		return 0
	}
	return roundRankingFloat((float64(current-previous) / float64(previous)) * 100)
}

func roundRankingFloat(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func limitRankedModels(rows []RankedModel, limit int) []RankedModel {
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

func limitRankingMovers(rows []RankingMover, limit int) []RankingMover {
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
