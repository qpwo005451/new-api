package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeVirtualModelMappingPreservesExistingMappings(t *testing.T) {
	mapping := mergeVirtualModelMapping(
		`{"other-alias":"other-upstream","auto-subagent-codex":"stale-model"}`,
		"auto-subagent-codex",
		"gpt-5.6-terra",
	)

	var decoded map[string]string
	require.NoError(t, common.UnmarshalJsonStr(mapping, &decoded))
	assert.Equal(t, "other-upstream", decoded["other-alias"])
	assert.Equal(t, "gpt-5.6-terra", decoded["auto-subagent-codex"])
}

func TestSetupContextForPreparedVirtualCandidateSetsRouteContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	modelMapping := `{"other-alias":"other-upstream"}`
	candidate := service.VirtualPoolCandidate{
		Channel: &model.Channel{
			Id:           3001,
			Key:          "bound-key",
			Type:         constant.ChannelTypeOpenAI,
			ModelMapping: &modelMapping,
		},
		UpstreamModel:    "prepared-upstream",
		ReasoningEffort:  "high",
		Group:            "vip",
		KeyIndex:         0,
		AccountIdentity:  "3001",
		FinalMappedModel: "prepared-upstream",
	}

	apiErr := SetupContextForPreparedVirtualCandidate(ctx, candidate, "virtual-model")
	require.Nil(t, apiErr)
	assert.Equal(t, 3001, common.GetContextKeyInt(ctx, constant.ContextKeyChannelId))
	assert.Equal(t, "bound-key", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
	var decodedMapping map[string]string
	require.NoError(t, common.UnmarshalJsonStr(common.GetContextKeyString(ctx, constant.ContextKeyChannelModelMapping), &decodedMapping))
	assert.Equal(t, "other-upstream", decodedMapping["other-alias"])
	assert.Equal(t, "prepared-upstream", decodedMapping["virtual-model"])
}
