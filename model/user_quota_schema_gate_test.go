package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// useQuotaSchemaGateDB opens an in-memory SQLite database whose users table is
// created with raw DDL. Raw DDL is required because AutoMigrate always emits
// 64-bit column types, while this gate exists to reject legacy 32-bit wallets.
func useQuotaSchemaGateDB(t *testing.T, ddl string) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousType := common.MainDatabaseType()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	if ddl != "" {
		require.NoError(t, db.Exec("CREATE TABLE users ("+ddl+")").Error)
	}
	DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB = previousDB
		common.SetMainDatabaseType(previousType)
	})
	return db
}

func quotaColumnDDL(column string, columnType string) string {
	ddl := "id INTEGER PRIMARY KEY"
	for _, name := range userQuotaColumns {
		if name == column {
			ddl += fmt.Sprintf(", %s %s", name, columnType)
			continue
		}
		ddl += fmt.Sprintf(", %s BIGINT", name)
	}
	return ddl
}

func TestEnsureUserQuotaColumnsRejectsLegacy32BitWallet(t *testing.T) {
	for _, column := range userQuotaColumns {
		t.Run(column, func(t *testing.T) {
			db := useQuotaSchemaGateDB(t, quotaColumnDDL(column, "int"))
			err := ensureUserQuotaColumns(db, common.DatabaseTypeMySQL)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "users."+column+" uses")
			assert.Contains(t, err.Error(), "32-bit is not supported")
		})
	}
	t.Run("postgres_int4", func(t *testing.T) {
		db := useQuotaSchemaGateDB(t, quotaColumnDDL("quota", "int4"))
		err := ensureUserQuotaColumns(db, common.DatabaseTypePostgreSQL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "users.quota uses")
		assert.Contains(t, err.Error(), "32-bit is not supported")
	})
}

func TestEnsureUserQuotaColumnsAccepts64BitWallet(t *testing.T) {
	for _, dbType := range []common.DatabaseType{common.DatabaseTypeMySQL, common.DatabaseTypePostgreSQL} {
		t.Run(string(dbType), func(t *testing.T) {
			ddl := "id INTEGER PRIMARY KEY, quota BIGINT, used_quota BIGINT, aff_quota BIGINT, aff_history BIGINT"
			db := useQuotaSchemaGateDB(t, ddl)
			require.NoError(t, ensureUserQuotaColumns(db, dbType))
		})
	}
}

func TestEnsureUserQuotaColumnsSkipsSQLiteAndMissingTable(t *testing.T) {
	t.Run("sqlite_is_exempt", func(t *testing.T) {
		db := useQuotaSchemaGateDB(t, quotaColumnDDL("quota", "int"))
		require.NoError(t, ensureUserQuotaColumns(db, common.DatabaseTypeSQLite))
	})
	t.Run("missing_table", func(t *testing.T) {
		db := useQuotaSchemaGateDB(t, "")
		require.NoError(t, ensureUserQuotaColumns(db, common.DatabaseTypeMySQL))
	})
	t.Run("nil_database", func(t *testing.T) {
		require.NoError(t, ensureUserQuotaColumns(nil, common.DatabaseTypeMySQL))
	})
}

func TestEnsureUserQuotaColumnsSkipFlagBypassesCheck(t *testing.T) {
	for _, value := range []string{"true", "1", "TRUE"} {
		t.Run("enabled_"+value, func(t *testing.T) {
			t.Setenv("SKIP_64BIT_QUOTA_SCHEMA_CHECK", value)
			db := useQuotaSchemaGateDB(t, quotaColumnDDL("quota", "int"))
			require.NoError(t, ensureUserQuotaColumns(db, common.DatabaseTypeMySQL))
		})
	}
	t.Run("disabled_false", func(t *testing.T) {
		t.Setenv("SKIP_64BIT_QUOTA_SCHEMA_CHECK", "false")
		db := useQuotaSchemaGateDB(t, quotaColumnDDL("quota", "int"))
		require.Error(t, ensureUserQuotaColumns(db, common.DatabaseTypeMySQL))
	})
}

func TestIs64BitIntegerTypeMatchesDialectSpelling(t *testing.T) {
	cases := []struct {
		dbType   common.DatabaseType
		dataType string
		want     bool
	}{
		{common.DatabaseTypeMySQL, "bigint", true},
		{common.DatabaseTypeMySQL, "BIGINT", true},
		{common.DatabaseTypeMySQL, " BigInt ", true},
		{common.DatabaseTypeMySQL, "unsigned bigint", true},
		{common.DatabaseTypeMySQL, "bigint unsigned", true},
		{common.DatabaseTypeMySQL, "int", false},
		{common.DatabaseTypeMySQL, "integer", false},
		{common.DatabaseTypeMySQL, "mediumint", false},
		{common.DatabaseTypeMySQL, "smallint", false},
		{common.DatabaseTypeMySQL, "numeric", false},
		{common.DatabaseTypeMySQL, "", false},
		{common.DatabaseTypePostgreSQL, "bigint", true},
		{common.DatabaseTypePostgreSQL, "int8", true},
		{common.DatabaseTypePostgreSQL, "INT8", true},
		{common.DatabaseTypePostgreSQL, "int4", false},
		{common.DatabaseTypePostgreSQL, "integer", false},
		{common.DatabaseTypePostgreSQL, "serial", false},
		{common.DatabaseTypeSQLite, "integer", false},
	}
	for _, tc := range cases {
		t.Run(string(tc.dbType)+"_"+tc.dataType, func(t *testing.T) {
			assert.Equal(t, tc.want, is64BitIntegerType(tc.dbType, tc.dataType))
		})
	}
}
