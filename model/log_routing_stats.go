package model

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// routingStatsLogTypes are the terminal log types that carry a routing decision
// trail. Pending (in-flight) rows are excluded because their outcome, and
// therefore whether they ever switched channels, is not yet known.
var routingStatsLogTypes = []int{LogTypeConsume, LogTypeError}

const defaultRoutingStatsScanRows = 50000

// routingStatsMaxBuckets bounds the trend series so that a wide window cannot
// produce an unbounded chart payload.
const routingStatsMaxBuckets = 200

// routingStatsBucketLadder holds the candidate trend bucket widths in seconds,
// from the finest granularity to the coarsest one.
var routingStatsBucketLadder = []int64{60, 120, 300, 600, 900, 1800, 3600, 7200, 21600, 43200, 86400, 172800, 604800}

type RoutingStatsQuery struct {
	StartTimestamp int64
	EndTimestamp   int64
	ModelName      string
	Group          string
	ChannelID      int
	// MaxScanRows bounds the decision-trail scan. Distribution figures stay
	// exact because they come from a SQL group-by; switch and affinity figures
	// cover only the most recent MaxScanRows matching requests.
	MaxScanRows int
	// BucketSeconds overrides the trend bucket width. Zero derives a width from
	// the requested window.
	BucketSeconds int64
}

type RoutingModelChannelStat struct {
	ModelName string `json:"model_name"`
	ChannelID int    `json:"channel_id"`
	// ChannelName stays empty when the channel no longer exists.
	ChannelName string  `json:"channel_name"`
	Requests    int64   `json:"requests"`
	Errors      int64   `json:"errors"`
	AvgUseTime  float64 `json:"avg_use_time"`
	// Priority and Weight are the effective routing values channel selection
	// uses: the per-model override when one is configured, otherwise the
	// channel value. They stay unset when the channel cannot be read.
	Priority *int64 `json:"priority,omitempty"`
	Weight   *uint  `json:"weight,omitempty"`
}

type RoutingSwitchStat struct {
	From  int   `json:"from"`
	To    int   `json:"to"`
	Count int64 `json:"count"`
}

type RoutingReasonStat struct {
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

type RoutingAffinityStat struct {
	RuleName       string `json:"rule_name"`
	StickyRequests int64  `json:"sticky_requests"`
	DistinctKeys   int64  `json:"distinct_keys"`
}

// RoutingStatsWindow is the trend axis: the covered time range and the width of
// one bucket.
type RoutingStatsWindow struct {
	Start         int64 `json:"start"`
	End           int64 `json:"end"`
	BucketSeconds int64 `json:"bucket_seconds"`
}

type RoutingTrendPoint struct {
	Timestamp int64 `json:"timestamp"`
	Requests  int64 `json:"requests"`
	Switched  int64 `json:"switched"`
}

type RoutingStats struct {
	Requests       int64                     `json:"requests"`
	Errors         int64                     `json:"errors"`
	ByModelChannel []RoutingModelChannelStat `json:"by_model_channel"`

	Window RoutingStatsWindow  `json:"window"`
	Trend  []RoutingTrendPoint `json:"trend"`

	Scanned   int64 `json:"scanned"`
	Truncated bool  `json:"truncated"`

	Switched        int64 `json:"switched"`
	SwitchedSuccess int64 `json:"switched_success"`
	SwitchedFailed  int64 `json:"switched_failed"`
	Sticky          int64 `json:"sticky"`

	Switches       []RoutingSwitchStat   `json:"switches"`
	SwitchReasons  []RoutingReasonStat   `json:"switch_reasons"`
	AffinityByRule []RoutingAffinityStat `json:"affinity_by_rule"`
}

// routingStatsAdminInfo mirrors the subset of LogOther's admin_info that the
// routing aggregation reads. It is parsed from storage rather than through the
// live LogOther type so the query works on rows written by any past version.
type routingStatsAdminInfo struct {
	// UseChannel lists every channel selection in order. The relay appends the
	// selected channel once when it is chosen and again right before the
	// upstream call, so one attempt normally repeats the same id.
	UseChannel      []string `json:"use_channel"`
	ChannelAffinity *struct {
		RuleName       string `json:"rule_name"`
		KeyFingerprint string `json:"key_fingerprint"`
	} `json:"channel_affinity"`
	RequestPolicy []struct {
		Decision struct {
			Action string `json:"action"`
			Reason string `json:"reason"`
		} `json:"decision"`
	} `json:"request_policy"`
}

func (query RoutingStatsQuery) apply(tx *gorm.DB) *gorm.DB {
	tx = tx.Where("type in ?", routingStatsLogTypes)
	if query.StartTimestamp != 0 {
		tx = tx.Where("created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp != 0 {
		tx = tx.Where("created_at <= ?", query.EndTimestamp)
	}
	if query.ModelName != "" {
		tx = tx.Where("model_name = ?", query.ModelName)
	}
	if query.Group != "" {
		tx = tx.Where(logGroupCol+" = ?", query.Group)
	}
	if query.ChannelID != 0 {
		tx = tx.Where("channel_id = ?", query.ChannelID)
	}
	return tx
}

func queryRoutingStatsDistribution(query RoutingStatsQuery) ([]RoutingModelChannelStat, int64, error) {
	type distributionRow struct {
		ModelName    string
		ChannelID    int
		Requests     int64
		TotalUseTime int64
	}
	// Totals are summed as integers because AVG() returns a dialect-specific
	// decimal that does not scan into a float across SQLite, MySQL and
	// PostgreSQL. Errors are counted in a second pass instead of a CASE
	// expression so every supported dialect can run the same SQL.
	var rows []distributionRow
	if err := query.apply(LOG_DB.Table("logs")).
		Select("model_name, channel_id, count(*) requests, sum(use_time) total_use_time").
		Group("model_name, channel_id").
		Order("model_name asc, channel_id asc").
		Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	var errorRows []distributionRow
	if err := query.apply(LOG_DB.Table("logs")).
		Select("model_name, channel_id, count(*) requests, sum(use_time) total_use_time").
		Where("type = ?", LogTypeError).
		Group("model_name, channel_id").
		Scan(&errorRows).Error; err != nil {
		return nil, 0, err
	}
	errors := make(map[[2]any]int64, len(errorRows))
	for _, row := range errorRows {
		errors[[2]any{row.ModelName, row.ChannelID}] = row.Requests
	}

	stats := make([]RoutingModelChannelStat, 0, len(rows))
	var total int64
	for _, row := range rows {
		total += row.Requests
		stat := RoutingModelChannelStat{
			ModelName: row.ModelName,
			ChannelID: row.ChannelID,
			Requests:  row.Requests,
			Errors:    errors[[2]any{row.ModelName, row.ChannelID}],
		}
		if row.Requests > 0 {
			stat.AvgUseTime = math.Round(float64(row.TotalUseTime)/float64(row.Requests)*10) / 10
		}
		stats = append(stats, stat)
	}
	return stats, total, nil
}

func queryRoutingStatsTrail(query RoutingStatsQuery) ([]Log, bool, error) {
	maxScanRows := query.MaxScanRows
	if maxScanRows <= 0 {
		maxScanRows = defaultRoutingStatsScanRows
	}

	// Read one extra row to detect that the window holds more than the cap.
	var rows []Log
	if err := query.apply(LOG_DB.Table("logs")).
		Select("id, type, channel_id, created_at, other").
		Order("id desc").
		Limit(maxScanRows + 1).
		Scan(&rows).Error; err != nil {
		return nil, false, err
	}
	if len(rows) > maxScanRows {
		return rows[:maxScanRows], true, nil
	}
	return rows, false, nil
}

// routingUseChannelTrail collapses the repeats the relay writes for one attempt
// into the ordered list of channels a request actually used. Only a change
// between consecutive entries means the request left its channel.
func routingUseChannelTrail(values []string) []int {
	trail := make([]int, 0, len(values))
	for _, value := range values {
		channelID, err := strconv.Atoi(value)
		if err != nil {
			continue
		}
		if len(trail) > 0 && trail[len(trail)-1] == channelID {
			continue
		}
		trail = append(trail, channelID)
	}
	return trail
}

// routingStatsBucketSeconds picks the trend bucket width for a window. The
// target is a readable number of points, so a one hour window renders two
// minute buckets and a week renders six hour buckets.
func routingStatsBucketSeconds(span int64) int64 {
	if span <= 0 {
		return routingStatsBucketLadder[0]
	}
	bucket := routingStatsBucketLadder[len(routingStatsBucketLadder)-1]
	for _, candidate := range routingStatsBucketLadder {
		if span/candidate <= 40 {
			bucket = candidate
			break
		}
	}
	for span/bucket+1 > routingStatsMaxBuckets {
		bucket *= 2
	}
	return bucket
}

// buildRoutingTrend resolves the trend axis and returns one zero-filled bucket
// per step so the chart always draws a continuous line. When the caller sends
// no window, the axis spans the inspected trail. Switch counts can only come
// from the decision trail, so the series describes the inspected rows.
func buildRoutingTrend(query RoutingStatsQuery, rows []Log) (RoutingStatsWindow, []RoutingTrendPoint) {
	window := RoutingStatsWindow{Start: query.StartTimestamp, End: query.EndTimestamp}
	if window.Start == 0 {
		for _, row := range rows {
			if window.Start == 0 || row.CreatedAt < window.Start {
				window.Start = row.CreatedAt
			}
		}
	}
	if window.End == 0 {
		for _, row := range rows {
			if row.CreatedAt > window.End {
				window.End = row.CreatedAt
			}
		}
	}
	if window.Start == 0 || window.End < window.Start {
		return window, nil
	}

	window.BucketSeconds = query.BucketSeconds
	if window.BucketSeconds <= 0 {
		window.BucketSeconds = routingStatsBucketSeconds(window.End - window.Start)
	}
	for (window.End-window.Start)/window.BucketSeconds+1 > routingStatsMaxBuckets {
		window.BucketSeconds *= 2
	}

	points := make([]RoutingTrendPoint, 0, 32)
	for timestamp := window.Start; timestamp <= window.End; timestamp += window.BucketSeconds {
		points = append(points, RoutingTrendPoint{Timestamp: timestamp})
	}
	return window, points
}

// QueryRoutingStats aggregates how traffic was distributed across channels and
// how often a request had to switch channels. The distribution comes from an
// exact SQL group-by; switching, its reasons and affinity stickiness are read
// back from the decision trail stored in other.admin_info.
func QueryRoutingStats(query RoutingStatsQuery) (*RoutingStats, error) {
	byModelChannel, totalRequests, err := queryRoutingStatsDistribution(query)
	if err != nil {
		return nil, err
	}

	rows, truncated, err := queryRoutingStatsTrail(query)
	if err != nil {
		return nil, err
	}

	window, trend := buildRoutingTrend(query, rows)

	stats := &RoutingStats{
		Requests:       totalRequests,
		ByModelChannel: byModelChannel,
		Window:         window,
		Trend:          trend,
		Scanned:        int64(len(rows)),
		Truncated:      truncated,
	}
	for _, row := range byModelChannel {
		stats.Errors += row.Errors
	}

	switchCounts := map[[2]int]int64{}
	reasonCounts := map[string]int64{}
	ruleRequests := map[string]int64{}
	ruleKeys := map[string]map[string]struct{}{}

	for _, row := range rows {
		// Every inspected row counts towards the trend, including rows whose
		// admin_info cannot be read back.
		index := -1
		if len(trend) > 0 && row.CreatedAt >= window.Start && row.CreatedAt <= window.End {
			index = int((row.CreatedAt - window.Start) / window.BucketSeconds)
			trend[index].Requests++
		}

		var other struct {
			AdminInfo routingStatsAdminInfo `json:"admin_info"`
		}
		if err := common.UnmarshalJsonStr(row.Other, &other); err != nil {
			continue
		}
		info := other.AdminInfo

		if info.ChannelAffinity != nil && info.ChannelAffinity.RuleName != "" {
			stats.Sticky++
			rule := info.ChannelAffinity.RuleName
			ruleRequests[rule]++
			if ruleKeys[rule] == nil {
				ruleKeys[rule] = map[string]struct{}{}
			}
			ruleKeys[rule][info.ChannelAffinity.KeyFingerprint] = struct{}{}
		}

		// A request that never left its first channel still records that
		// channel more than once, so a switch requires a change between
		// consecutive selections rather than a trail longer than one entry.
		trail := routingUseChannelTrail(info.UseChannel)
		if len(trail) < 2 {
			continue
		}
		if index >= 0 {
			trend[index].Switched++
		}
		stats.Switched++
		if row.Type == LogTypeError {
			stats.SwitchedFailed++
		} else {
			stats.SwitchedSuccess++
		}
		for i := 1; i < len(trail); i++ {
			switchCounts[[2]int{trail[i-1], trail[i]}]++
		}
		for _, event := range info.RequestPolicy {
			if event.Decision.Action == "retry" && event.Decision.Reason != "" {
				reasonCounts[event.Decision.Reason]++
			}
		}
	}

	switches := make([]RoutingSwitchStat, 0, len(switchCounts))
	for pair, count := range switchCounts {
		switches = append(switches, RoutingSwitchStat{From: pair[0], To: pair[1], Count: count})
	}
	slices.SortFunc(switches, func(a, b RoutingSwitchStat) int {
		if a.Count != b.Count {
			return int(b.Count - a.Count)
		}
		if a.From != b.From {
			return a.From - b.From
		}
		return a.To - b.To
	})
	stats.Switches = switches

	reasons := make([]RoutingReasonStat, 0, len(reasonCounts))
	for reason, count := range reasonCounts {
		reasons = append(reasons, RoutingReasonStat{Reason: reason, Count: count})
	}
	slices.SortFunc(reasons, func(a, b RoutingReasonStat) int {
		if a.Count != b.Count {
			return int(b.Count - a.Count)
		}
		return strings.Compare(a.Reason, b.Reason)
	})
	stats.SwitchReasons = reasons

	affinity := make([]RoutingAffinityStat, 0, len(ruleRequests))
	for rule, count := range ruleRequests {
		affinity = append(affinity, RoutingAffinityStat{
			RuleName:       rule,
			StickyRequests: count,
			DistinctKeys:   int64(len(ruleKeys[rule])),
		})
	}
	slices.SortFunc(affinity, func(a, b RoutingAffinityStat) int {
		if a.StickyRequests != b.StickyRequests {
			return int(b.StickyRequests - a.StickyRequests)
		}
		return strings.Compare(a.RuleName, b.RuleName)
	})
	stats.AffinityByRule = affinity
	return stats, nil
}
