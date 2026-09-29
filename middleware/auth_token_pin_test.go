package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// useTokenPinDB seeds an in-memory database with one admin and one ordinary
// user so the token channel-suffix contract can be exercised end to end.
func useTokenPinDB(t *testing.T) (adminId int, userId int) {
	t.Helper()
	previousDB := model.DB
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.User{}))
	require.NoError(t, database.Create(&model.User{Username: "pin-admin", Role: common.RoleAdminUser, AffCode: "PINADMIN"}).Error)
	require.NoError(t, database.Create(&model.User{Username: "pin-user", Role: common.RoleCommonUser, AffCode: "PINUSER"}).Error)
	var admin, user model.User
	require.NoError(t, database.Where("username = ?", "pin-admin").First(&admin).Error)
	require.NoError(t, database.Where("username = ?", "pin-user").First(&user).Error)
	model.DB = database
	t.Cleanup(func() { model.DB = previousDB })
	return admin.Id, user.Id
}

func TestSetupContextForTokenPinsAdminSuffixChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminId, _ := useTokenPinDB(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	token := &model.Token{Id: 1, UserId: adminId, Key: "sk-pin-test", Name: "pin", Group: "default"}

	require.NoError(t, SetupContextForToken(c, token, "sk-pin-test", "42"))

	pin, found, _ := service.GetChannelConstraints(c).ResolvedPin()
	require.True(t, found, "an admin token suffix must pin a channel")
	assert.Equal(t, 42, pin.ChannelId)
	assert.Equal(t, dto.PinSourceToken, pin.Source)
	assert.Equal(t, dto.PinRankToken, pin.Rank)
	assert.Equal(t, dto.PinRetrySingleAttempt, pin.RetryMode)
}

func TestSetupContextForTokenRejectsSuffixForNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, userId := useTokenPinDB(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	token := &model.Token{Id: 2, UserId: userId, Key: "sk-pin-user", Name: "pin", Group: "default"}

	require.Error(t, SetupContextForToken(c, token, "sk-pin-user", "42"))
	assert.Equal(t, "701e3ae1dc3f7975556d354e0675168d004891c8", c.Writer.Header().Get("specific_channel_version"))

	pin, found, _ := service.GetChannelConstraints(c).ResolvedPin()
	assert.False(t, found, "a non-admin suffix must never pin a channel")
	assert.Equal(t, 0, pin.ChannelId)
}
