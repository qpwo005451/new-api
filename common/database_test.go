package common

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestNormalizeSQLitePathAddsBusyTimeout(t *testing.T) {
	assert.Equal(t, "data/new-api.db?_pragma=busy_timeout(30000)", normalizeSQLitePath("data/new-api.db"))
	assert.Equal(t, "data/new-api.db?mode=rwc&_pragma=busy_timeout(30000)", normalizeSQLitePath("data/new-api.db?mode=rwc"))
	assert.Equal(t, "data/new-api.db?_pragma=busy_timeout(5000)", normalizeSQLitePath("data/new-api.db?_pragma=busy_timeout(5000)"))
	assert.Equal(t, ":memory:", normalizeSQLitePath(":memory:"))
}

func TestNormalizeSQLitePathConfiguresDriverBusyTimeout(t *testing.T) {
	dsn := normalizeSQLitePath(filepath.Join(t.TempDir(), "new-api.db"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	var timeout int
	require.NoError(t, db.Raw("PRAGMA busy_timeout").Scan(&timeout).Error)
	assert.Equal(t, 30000, timeout)
}
