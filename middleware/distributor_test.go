package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupContextForSelectedChannelSetsTypedOriginalModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	channel := &model.Channel{Id: 1, Type: 1, Name: "test-channel"}

	setupErr := SetupContextForSelectedChannel(ctx, channel, "virtual-model")
	require.Nil(t, setupErr)

	assert.Equal(t, "virtual-model", common.GetContextKeyString(ctx, constant.ContextKeyOriginalModel))
	assert.Equal(t, "virtual-model", ctx.GetString(string(constant.ContextKeyOriginalModel)))
}
