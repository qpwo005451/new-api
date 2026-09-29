package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModelHealthCooldownsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/log/model_health_cooldowns", nil)

	GetModelHealthCooldowns(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Binding string `json:"binding"`
			Entries []struct {
				ModelName       string `json:"model_name"`
				ChannelID       int    `json:"channel_id"`
				Cooling         bool   `json:"cooling"`
				ConsecutiveErrs int    `json:"consecutive_errors"`
				CooldownUntil   int64  `json:"cooldown_until"`
			} `json:"entries"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.NotEmpty(t, body.Data.Binding)
	assert.NotNil(t, body.Data.Entries)
}

func TestResetModelHealthCooldownsHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/log/model_health_cooldowns/reset", nil)

	ResetModelHealthCooldowns(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Cleared int `json:"cleared"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.GreaterOrEqual(t, body.Data.Cleared, 0)
}
