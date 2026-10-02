package model

import (
	"fmt"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openRoutingStatsLogDB points the package log database at one of the three
// supported dialects so the aggregation is exercised on every database the
// project ships. MySQL and PostgreSQL need TEST_MYSQL_DSN / TEST_POSTGRES_DSN;
// without them the subtest is skipped.
func openRoutingStatsLogDB(t *testing.T, dialect string) *gorm.DB {
	t.Helper()

	originalLogDB := LOG_DB
	originalLogDatabaseType := common.LogDatabaseType()
	t.Cleanup(func() {
		LOG_DB = originalLogDB
		common.SetLogDatabaseType(originalLogDatabaseType)
	})

	var driver gorm.Dialector
	var databaseType common.DatabaseType
	switch dialect {
	case "mysql":
		dsn := os.Getenv("TEST_MYSQL_DSN")
		if dsn == "" {
			t.Skip("TEST_MYSQL_DSN is not configured")
		}
		driver = mysql.Open(dsn)
		databaseType = common.DatabaseTypeMySQL
	case "postgres":
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("TEST_POSTGRES_DSN is not configured")
		}
		driver = postgres.Open(dsn)
		databaseType = common.DatabaseTypePostgreSQL
	default:
		driver = sqlite.Open(":memory:")
		databaseType = common.DatabaseTypeSQLite
	}

	db, err := gorm.Open(driver, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	t.Cleanup(func() {
		require.NoError(t, db.Migrator().DropTable("logs"))
	})

	LOG_DB = db
	common.SetLogDatabaseType(databaseType)
	// Reserved-word quoting follows the log database type.
	initCol()
	return db
}

type routingStatsLogFixture struct {
	logType    int
	model      string
	channelID  int
	createdAt  int64
	group      string
	useChannel []int
	ruleName   string
	keyFP      string
	retryAs    []string
	useTime    int
}

// seedRoutingStatsLog writes one log row whose admin_info carries exactly the
// fields the routing aggregation reads back: the attempted channel trail, the
// affinity binding, and the per-attempt policy decisions.
func seedRoutingStatsLog(t *testing.T, db *gorm.DB, fixture routingStatsLogFixture) {
	t.Helper()

	// The relay records an attempt's channel twice: once when the channel is
	// selected and again right before the upstream call. Seed that shape so the
	// aggregation is exercised against trails as they are stored in production.
	useChannel := make([]string, 0, 2*len(fixture.useChannel))
	for _, id := range fixture.useChannel {
		value := fmt.Sprintf("%d", id)
		useChannel = append(useChannel, value, value)
	}
	adminInfo := map[string]any{"use_channel": useChannel}
	if fixture.ruleName != "" {
		adminInfo["channel_affinity"] = map[string]any{
			"rule_name":       fixture.ruleName,
			"using_group":     "default",
			"key_fingerprint": fixture.keyFP,
			"ttl_seconds":     1800,
		}
	}
	events := make([]map[string]any, 0, len(fixture.retryAs))
	for _, reason := range fixture.retryAs {
		events = append(events, map[string]any{
			"channel_id": fixture.channelID,
			"decision":   map[string]any{"action": "retry", "reason": reason, "source": "system"},
		})
	}
	if len(events) > 0 {
		adminInfo["request_policy"] = events
	}

	raw, err := common.Marshal(map[string]any{"admin_info": adminInfo})
	require.NoError(t, err)

	require.NoError(t, db.Create(&Log{
		UserId:    1,
		CreatedAt: fixture.createdAt,
		Type:      fixture.logType,
		Username:  "tester",
		ModelName: fixture.model,
		ChannelId: fixture.channelID,
		Group:     fixture.group,
		UseTime:   fixture.useTime,
		Other:     string(raw),
	}).Error)
}

func seedRoutingStatsFixture(t *testing.T, db *gorm.DB) {
	t.Helper()

	// Served on the first attempt: the trail repeats channel 9 because the
	// attempt is recorded twice, which is not a switch.
	seedRoutingStatsLog(t, db, routingStatsLogFixture{
		logType: LogTypeConsume, model: "deepseek-v4.1-flash", createdAt: 1000, channelID: 9,
		useChannel: []int{9}, useTime: 100,
	})
	// Switched 9 -> 47 and succeeded, on a sticky session.
	seedRoutingStatsLog(t, db, routingStatsLogFixture{
		logType: LogTypeConsume, model: "deepseek-v4.1-flash", createdAt: 1010, channelID: 47,
		useChannel: []int{9, 47}, ruleName: "deepseek glm session stickiness", keyFP: "fp-1",
		retryAs: []string{"channel_error"}, useTime: 200,
	})
	// Switched 9 -> 47 -> 9 and still failed, on the same sticky session.
	seedRoutingStatsLog(t, db, routingStatsLogFixture{
		logType: LogTypeError, model: "deepseek-v4.1-flash", createdAt: 1020, channelID: 9,
		useChannel: []int{9, 47, 9}, ruleName: "deepseek glm session stickiness", keyFP: "fp-1",
		retryAs: []string{"channel_error", "retry_status_matched"}, useTime: 300,
	})
	// A different model and channel, no switch.
	seedRoutingStatsLog(t, db, routingStatsLogFixture{
		logType: LogTypeConsume, model: "glm-5.3-flash", createdAt: 1030, channelID: 21,
		group: "vip", useChannel: []int{21}, useTime: 400,
	})
}

// TestQueryRoutingStatsAggregatesDistributionAndSwitches pins the aggregation
// contract: the per-model/channel distribution comes from an exact SQL group-by,
// while the switch, reason and affinity figures come from reading the decision
// trail stored in other.admin_info.
func TestQueryRoutingStatsAggregatesDistributionAndSwitches(t *testing.T) {
	seedRoutingStatsFixture(t, openRoutingStatsLogDB(t, "sqlite"))

	stats, err := QueryRoutingStats(RoutingStatsQuery{})
	require.NoError(t, err)

	assert.EqualValues(t, 4, stats.Requests)
	assert.EqualValues(t, 1, stats.Errors)
	assert.False(t, stats.Truncated)
	assert.EqualValues(t, 4, stats.Scanned)

	assert.Equal(t, []RoutingModelChannelStat{
		{ModelName: "deepseek-v4.1-flash", ChannelID: 9, Requests: 2, Errors: 1, AvgUseTime: 200},
		{ModelName: "deepseek-v4.1-flash", ChannelID: 47, Requests: 1, AvgUseTime: 200},
		{ModelName: "glm-5.3-flash", ChannelID: 21, Requests: 1, AvgUseTime: 400},
	}, stats.ByModelChannel)

	assert.EqualValues(t, 2, stats.Switched)
	assert.EqualValues(t, 1, stats.SwitchedSuccess)
	assert.EqualValues(t, 1, stats.SwitchedFailed)
	assert.EqualValues(t, 2, stats.Sticky)
	assert.Equal(t, []RoutingSwitchStat{
		{From: 9, To: 47, Count: 2},
		{From: 47, To: 9, Count: 1},
	}, stats.Switches)
	assert.Equal(t, []RoutingReasonStat{
		{Reason: "channel_error", Count: 2},
		{Reason: "retry_status_matched", Count: 1},
	}, stats.SwitchReasons)
	assert.Equal(t, []RoutingAffinityStat{
		{RuleName: "deepseek glm session stickiness", StickyRequests: 2, DistinctKeys: 1},
	}, stats.AffinityByRule)
}

func TestQueryRoutingStatsAppliesFilters(t *testing.T) {
	seedRoutingStatsFixture(t, openRoutingStatsLogDB(t, "sqlite"))

	stats, err := QueryRoutingStats(RoutingStatsQuery{ModelName: "glm-5.3-flash"})
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats.Requests)
	assert.EqualValues(t, 0, stats.Switched)
	assert.Empty(t, stats.Switches)

	byChannel, err := QueryRoutingStats(RoutingStatsQuery{ChannelID: 47})
	require.NoError(t, err)
	assert.EqualValues(t, 1, byChannel.Requests)
}

// TestQueryRoutingStatsFlagsScanTruncation keeps the switch analysis bounded:
// the distribution stays exact while the decision-trail scan reports that it
// only read the most recent rows.
func TestQueryRoutingStatsFlagsScanTruncation(t *testing.T) {
	seedRoutingStatsFixture(t, openRoutingStatsLogDB(t, "sqlite"))

	stats, err := QueryRoutingStats(RoutingStatsQuery{MaxScanRows: 2})
	require.NoError(t, err)

	assert.True(t, stats.Truncated)
	assert.EqualValues(t, 2, stats.Scanned)
	// The exact distribution is unaffected by the scan cap.
	assert.EqualValues(t, 4, stats.Requests)
	// Only the two most recent rows were inspected: the failing 9 -> 47 -> 9
	// request and the single-channel glm request.
	assert.EqualValues(t, 1, stats.Switched)
	assert.EqualValues(t, 1, stats.SwitchedFailed)
}

// TestQueryRoutingStatsBucketsTrendOverTheWindow pins the trend contract: the
// series is bucketed over the requested window, every bucket is emitted (so the
// chart has a continuous axis), and switch counts land in the bucket that holds
// the request.
func TestQueryRoutingStatsBucketsTrendOverTheWindow(t *testing.T) {
	seedRoutingStatsFixture(t, openRoutingStatsLogDB(t, "sqlite"))

	stats, err := QueryRoutingStats(RoutingStatsQuery{
		StartTimestamp: 1000,
		EndTimestamp:   1099,
		BucketSeconds:  20,
	})
	require.NoError(t, err)

	assert.Equal(t, RoutingStatsWindow{Start: 1000, End: 1099, BucketSeconds: 20}, stats.Window)
	assert.Equal(t, []RoutingTrendPoint{
		{Timestamp: 1000, Requests: 2, Switched: 1},
		{Timestamp: 1020, Requests: 2, Switched: 1},
		{Timestamp: 1040},
		{Timestamp: 1060},
		{Timestamp: 1080},
	}, stats.Trend)
}

// TestQueryRoutingStatsDerivesTrendBucketFromWindow keeps the default chart
// granularity bounded: a one hour window renders 30 buckets of two minutes.
func TestQueryRoutingStatsDerivesTrendBucketFromWindow(t *testing.T) {
	seedRoutingStatsFixture(t, openRoutingStatsLogDB(t, "sqlite"))

	stats, err := QueryRoutingStats(RoutingStatsQuery{
		StartTimestamp: 1000,
		EndTimestamp:   4600,
	})
	require.NoError(t, err)

	assert.EqualValues(t, 120, stats.Window.BucketSeconds)
	assert.Len(t, stats.Trend, 31)
	assert.EqualValues(t, 1000, stats.Trend[0].Timestamp)
	assert.EqualValues(t, 4600, stats.Trend[len(stats.Trend)-1].Timestamp)
}

// TestQueryRoutingStatsDerivesWindowFromTrail covers the unfiltered call the
// API makes when the caller sends no time range.
func TestQueryRoutingStatsDerivesWindowFromTrail(t *testing.T) {
	seedRoutingStatsFixture(t, openRoutingStatsLogDB(t, "sqlite"))

	stats, err := QueryRoutingStats(RoutingStatsQuery{})
	require.NoError(t, err)

	assert.EqualValues(t, 1000, stats.Window.Start)
	assert.EqualValues(t, 1030, stats.Window.End)
	assert.Len(t, stats.Trend, 1)
	assert.Equal(t, RoutingTrendPoint{Timestamp: 1000, Requests: 4, Switched: 2}, stats.Trend[0])
}

// TestQueryRoutingStatsIsDatabasePortable keeps the log aggregation portable
// across SQLite, MySQL and PostgreSQL: the distribution comes from a plain
// group-by, and the decision trail is filtered in SQL and parsed in Go, so no
// dialect-specific JSON or date function is involved.
func TestQueryRoutingStatsIsDatabasePortable(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := openRoutingStatsLogDB(t, dialect)
			seedRoutingStatsFixture(t, db)

			stats, err := QueryRoutingStats(RoutingStatsQuery{
				StartTimestamp: 1000,
				EndTimestamp:   1099,
				BucketSeconds:  20,
			})
			require.NoError(t, err)

			assert.EqualValues(t, 4, stats.Requests)
			assert.EqualValues(t, 1, stats.Errors)
			assert.Equal(t, []RoutingModelChannelStat{
				{ModelName: "deepseek-v4.1-flash", ChannelID: 9, Requests: 2, Errors: 1, AvgUseTime: 200},
				{ModelName: "deepseek-v4.1-flash", ChannelID: 47, Requests: 1, AvgUseTime: 200},
				{ModelName: "glm-5.3-flash", ChannelID: 21, Requests: 1, AvgUseTime: 400},
			}, stats.ByModelChannel)
			assert.Equal(t, []RoutingSwitchStat{
				{From: 9, To: 47, Count: 2},
				{From: 47, To: 9, Count: 1},
			}, stats.Switches)
			assert.Equal(t, []RoutingTrendPoint{
				{Timestamp: 1000, Requests: 2, Switched: 1},
				{Timestamp: 1020, Requests: 2, Switched: 1},
				{Timestamp: 1040},
				{Timestamp: 1060},
				{Timestamp: 1080},
			}, stats.Trend)

			// "group" is a reserved word, so the filter must use the quoting
			// the log database dialect expects.
			byGroup, err := QueryRoutingStats(RoutingStatsQuery{
				StartTimestamp: 1000,
				EndTimestamp:   1099,
				Group:          "vip",
			})
			require.NoError(t, err)
			assert.EqualValues(t, 1, byGroup.Requests)
			assert.Equal(t, []RoutingModelChannelStat{
				{ModelName: "glm-5.3-flash", ChannelID: 21, Requests: 1, AvgUseTime: 400},
			}, byGroup.ByModelChannel)
		})
	}
}
