package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLogClientAliasTest(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalOptionMap := common.OptionMap
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.OptionMap = originalOptionMap
	})

	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.OptionMap = map[string]string{}

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Option{}))

	require.NoError(t, model.UpdateOption(operation_setting.ClientAliasOptionKey, "{}"))
}

func performLogClientAliasRequest(t *testing.T, method string, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, "/api/log/client_aliases", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	if method == http.MethodPut {
		UpdateLogClientAlias(c)
	} else {
		GetLogClientAliases(c)
	}
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &resp))
	return w, resp
}

func TestUpdateLogClientAliasPersistsAndReturnsMap(t *testing.T) {
	setupLogClientAliasTest(t)

	_, resp := performLogClientAliasRequest(t, http.MethodPut,
		`{"user_agent":"  OpenAI/JS 6.47.0  ","name":"  Prime Agent  "}`)

	require.Equal(t, true, resp["success"])
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Prime Agent", data["OpenAI/JS 6.47.0"])
	assert.Equal(t, "Prime Agent",
		operation_setting.GetClientAliases()["OpenAI/JS 6.47.0"],
		"registered config must reflect the new alias")

	var option model.Option
	require.NoError(t, model.DB.Where("key = ?", operation_setting.ClientAliasOptionKey).First(&option).Error)
	require.NoError(t, common.UnmarshalJsonStr(option.Value, &data))
	assert.Equal(t, "Prime Agent", data["OpenAI/JS 6.47.0"], "alias must persist to the options table")
}

func TestUpdateLogClientAliasEmptyNameRemovesEntry(t *testing.T) {
	setupLogClientAliasTest(t)

	_, resp := performLogClientAliasRequest(t, http.MethodPut,
		`{"user_agent":"OpenAI/JS 6.47.0","name":"Prime Agent"}`)
	require.Equal(t, true, resp["success"])

	_, resp = performLogClientAliasRequest(t, http.MethodPut,
		`{"user_agent":"OpenAI/JS 6.47.0","name":""}`)
	require.Equal(t, true, resp["success"])
	data := resp["data"].(map[string]any)
	assert.NotContains(t, data, "OpenAI/JS 6.47.0")
	assert.NotContains(t, operation_setting.GetClientAliases(), "OpenAI/JS 6.47.0")
}

func TestUpdateLogClientAliasRejectsInvalidInput(t *testing.T) {
	setupLogClientAliasTest(t)

	_, resp := performLogClientAliasRequest(t, http.MethodPut, `{"user_agent":"   ","name":"X"}`)
	require.Equal(t, false, resp["success"])
	assert.NotEmpty(t, resp["message"])
	assert.Empty(t, operation_setting.GetClientAliases())
}

func TestGetLogClientAliasesReturnsConfiguredMap(t *testing.T) {
	setupLogClientAliasTest(t)

	_, seed := performLogClientAliasRequest(t, http.MethodPut,
		`{"user_agent":"codex_cli_rs/0.42.0","name":"Codex CLI"}`)
	require.Equal(t, true, seed["success"])

	_, resp := performLogClientAliasRequest(t, http.MethodGet, "")
	require.Equal(t, true, resp["success"])
	data, ok := resp["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Codex CLI", data["codex_cli_rs/0.42.0"])
}
