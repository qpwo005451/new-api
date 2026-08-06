package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelMappedHelperAppliesVirtualReasoningEffortByRequestShape(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("responses", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(ctx, constant.ContextKeyVirtualReasoningEffort, "high")
		request := &dto.OpenAIResponsesRequest{Model: "auto-subagent"}
		info := &relaycommon.RelayInfo{
			OriginModelName: "auto-subagent",
			ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "auto-subagent"},
		}

		require.NoError(t, ModelMappedHelper(ctx, info, request))
		require.NotNil(t, request.Reasoning)
		assert.Equal(t, "high", request.Reasoning.Effort)
		assert.Equal(t, "high", info.ReasoningEffort)
	})

	t.Run("chat completions", func(t *testing.T) {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(ctx, constant.ContextKeyVirtualReasoningEffort, "low")
		request := &dto.GeneralOpenAIRequest{Model: "auto-subagent"}
		info := &relaycommon.RelayInfo{
			OriginModelName: "auto-subagent",
			ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "auto-subagent"},
		}

		require.NoError(t, ModelMappedHelper(ctx, info, request))
		assert.Equal(t, "low", request.ReasoningEffort)
		assert.Equal(t, "low", info.ReasoningEffort)
	})
}

func TestModelMappedHelperKeepsClientReasoningEffortWhenNoVirtualMappingExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	request := &dto.OpenAIResponsesRequest{
		Model:     "ordinary-model",
		Reasoning: &dto.Reasoning{Effort: "xhigh"},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "ordinary-model",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "ordinary-model"},
	}

	require.NoError(t, ModelMappedHelper(ctx, info, request))
	require.NotNil(t, request.Reasoning)
	assert.Equal(t, "xhigh", request.Reasoning.Effort)
}
