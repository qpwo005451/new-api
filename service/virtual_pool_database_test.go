package service

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type virtualPoolDatabaseContract struct {
	name      string
	env       string
	dialector func(string) gorm.Dialector
	dbType    common.DatabaseType
}

func TestVirtualPoolDatabaseContract(t *testing.T) {
	tests := []virtualPoolDatabaseContract{
		{
			name: "sqlite",
			dialector: func(string) gorm.Dialector {
				return sqlite.Open(":memory:")
			},
			dbType: common.DatabaseTypeSQLite,
		},
		{
			name: "mysql",
			env:  "TEST_MYSQL_DSN",
			dialector: func(dsn string) gorm.Dialector {
				return mysql.Open(dsn)
			},
			dbType: common.DatabaseTypeMySQL,
		},
		{
			name: "postgres",
			env:  "TEST_POSTGRES_DSN",
			dialector: func(dsn string) gorm.Dialector {
				return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			},
			dbType: common.DatabaseTypePostgreSQL,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dsn := ""
			if test.env != "" {
				dsn = strings.TrimSpace(os.Getenv(test.env))
				if dsn == "" {
					t.Skip(test.env + " is not configured")
				}
			}
			runVirtualPoolDatabaseContract(t, test, dsn)
		})
	}
}

func runVirtualPoolDatabaseContract(t *testing.T, test virtualPoolDatabaseContract, dsn string) {
	t.Helper()
	const optionPrefix = "model_retry_policy_setting"
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousSettings := *operation_setting.GetModelRetryPolicySetting()
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	db, err := gorm.Open(test.dialector(dsn), &gorm.Config{})
	require.NoError(t, err)
	deletePolicyOptions := func() {
		rows, err := model.AllOption()
		require.NoError(t, err)
		for _, option := range rows {
			if strings.HasPrefix(option.Key, optionPrefix+".") {
				require.NoError(t, db.Delete(&model.Option{Key: option.Key}).Error)
			}
		}
	}
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		deletePolicyOptions()
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		*operation_setting.GetModelRetryPolicySetting() = previousSettings
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
		_ = sqlDB.Close()
	})
	model.DB = db
	common.SetMainDatabaseType(test.dbType)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	deletePolicyOptions()

	setting := operation_setting.GetModelRetryPolicySetting()
	*setting = operation_setting.ModelRetryPolicySetting{
		VirtualPoolSticky: operation_setting.VirtualPoolStickySetting{
			Enabled:               true,
			SessionMode:           operation_setting.VirtualPoolSessionModeThread,
			BindingMode:           operation_setting.VirtualPoolBindingModeRedis,
			MultiKeyPolicy:        operation_setting.VirtualPoolMultiKeyPolicyBindIndex,
			PendingLeaseSeconds:   45,
			ConfirmedTTLSeconds:   1800,
			CapacityWaitMillis:    2500,
			CapacityLeaseSeconds:  300,
			RedisRequiredForReady: true,
			Groups:                []string{"default"},
			Models:                []string{"deepseek v4.1 flash"},
		},
	}
	configMap, err := config.ConfigToMap(setting)
	require.NoError(t, err)
	for key, value := range configMap {
		require.NoError(t, model.UpdateOption("model_retry_policy_setting."+key, value))
	}

	*setting = operation_setting.ModelRetryPolicySetting{}
	options := make(map[string]string)
	rows, err := model.AllOption()
	require.NoError(t, err)
	for _, option := range rows {
		options[option.Key] = option.Value
	}
	require.NoError(t, config.GlobalConfig.LoadFromDB(options))
	assert.True(t, setting.VirtualPoolSticky.Enabled)
	assert.Equal(t, operation_setting.VirtualPoolBindingModeRedis, setting.VirtualPoolSticky.BindingMode)
	assert.Equal(t, 45, setting.VirtualPoolSticky.PendingLeaseSeconds)
	assert.Equal(t, 1800, setting.VirtualPoolSticky.ConfirmedTTLSeconds)
	assert.Equal(t, []string{"deepseek v4.1 flash"}, setting.VirtualPoolSticky.Models)
}
