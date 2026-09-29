package common

import "strings"

type DatabaseType string

const (
	DatabaseTypeMySQL      DatabaseType = "mysql"
	DatabaseTypeSQLite     DatabaseType = "sqlite"
	DatabaseTypePostgreSQL DatabaseType = "postgres"
	DatabaseTypeClickHouse DatabaseType = "clickhouse"
)

var mainDatabaseType = DatabaseTypeSQLite
var logDatabaseType = DatabaseTypeSQLite

func MainDatabaseType() DatabaseType {
	return mainDatabaseType
}

func LogDatabaseType() DatabaseType {
	return logDatabaseType
}

func SetMainDatabaseType(databaseType DatabaseType) {
	mainDatabaseType = databaseType
}

func SetLogDatabaseType(databaseType DatabaseType) {
	logDatabaseType = databaseType
}

func SetDatabaseTypes(mainType DatabaseType, logType DatabaseType) {
	mainDatabaseType = mainType
	logDatabaseType = logType
}

func UsingMainDatabase(databaseType DatabaseType) bool {
	return mainDatabaseType == databaseType
}

func UsingLogDatabase(databaseType DatabaseType) bool {
	return logDatabaseType == databaseType
}

const (
	sqliteBusyTimeoutParam = "_pragma=busy_timeout(30000)"
	// WAL lets concurrent dashboard reads proceed while the relay writes
	// logs; in rollback-journal mode readers block writers and lock-upgrade
	// paths return immediate SQLITE_BUSY that ignores busy_timeout, which
	// surfaced as spurious "database error" dashboard responses.
	sqliteJournalModeParam = "_pragma=journal_mode(WAL)"
	// BEGIN IMMEDIATE takes the write lock when the transaction starts, so
	// writers serialize through the busy timeout instead of failing on a stale
	// read snapshot (SQLITE_BUSY_SNAPSHOT), which the busy handler cannot cover.
	sqliteTxLockParam = "_txlock=immediate"
)

// normalizeSQLitePath completes a SQLite DSN with the parameters the pure-Go
// driver only honours as DSN parameters: the `_pragma=busy_timeout(30000)` form
// (the plain `_busy_timeout=` alias is silently ignored, see #6805), WAL
// journal mode, and `_txlock=immediate`. Parameters the caller already supplied
// are preserved.
func normalizeSQLitePath(path string) string {
	if path == "" || path == ":memory:" {
		return path
	}
	lowerPath := strings.ToLower(path)
	missing := make([]string, 0, 3)
	if !strings.Contains(lowerPath, "_pragma=busy_timeout") {
		missing = append(missing, sqliteBusyTimeoutParam)
	}
	if !strings.Contains(lowerPath, "_pragma=journal_mode") {
		missing = append(missing, sqliteJournalModeParam)
	}
	if !strings.Contains(lowerPath, "_txlock") {
		missing = append(missing, sqliteTxLockParam)
	}
	if len(missing) == 0 {
		return path
	}
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + strings.Join(missing, "&")
}

// SQLitePath is the DSN for the default SQLite database. normalizeSQLitePath
// turns it into
//
//	one-api.db?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate
//
// Both non-obvious parameters are required for concurrent correctness:
//
//  1. The busy timeout must be passed as `_pragma=busy_timeout(30000)`. The
//     pure-Go driver (modernc.org/sqlite, used through
//     github.com/glebarez/sqlite) silently ignores the plain `_busy_timeout=`
//     form, so without this the effective timeout stays at SQLite's 5s default
//     and concurrent writes surface as "database is locked" (see #6805).
//
//  2. `_txlock=immediate` (BEGIN IMMEDIATE) must be enabled. Without it, a
//     transaction that first SELECTs (establishing a read snapshot) and then
//     writes can hit SQLITE_BUSY_SNAPSHOT when another connection commits in
//     between; the busy handler does not cover that case, so the write fails
//     instantly no matter the timeout. BEGIN IMMEDIATE takes the write lock up
//     front, so writers serialize through the busy timeout instead of dying on
//     a stale snapshot. Autocommit SELECTs stay concurrent because WAL keeps
//     readers unlocked.
var SQLitePath = normalizeSQLitePath("one-api.db")
