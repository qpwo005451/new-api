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
)

func normalizeSQLitePath(path string) string {
	if path == "" || path == ":memory:" {
		return path
	}
	lowerPath := strings.ToLower(path)
	if strings.Contains(lowerPath, "_pragma=busy_timeout") {
		return path
	}
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + sqliteBusyTimeoutParam + "&" + sqliteJournalModeParam
}

var SQLitePath = normalizeSQLitePath("one-api.db")
