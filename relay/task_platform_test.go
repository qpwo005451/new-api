package relay

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTaskPlatformPriority(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Set("platform", "fallback")
	assert.Equal(t, "fallback", string(GetTaskPlatform(c)))
	c.Set("channel_type", 59)
	assert.Equal(t, "59", string(GetTaskPlatform(c)))
	c.Set("task_plugin_key", "document-parser")
	assert.Equal(t, "document-parser", string(GetTaskPlatform(c)))
}

// TestLegacyTaskChannelTypesResolveToBuiltInTaskPlugins protects the migration
// away from the deleted native task adaptors: every legacy task channel type
// must still resolve to a built-in task plugin, otherwise existing channels
// silently stop being routeable after the merge.
func TestLegacyTaskChannelTypesResolveToBuiltInTaskPlugins(t *testing.T) {
	generation := jsplugin.DefaultRegistry.Generation()
	require.NotNil(t, generation)

	legacy := []struct {
		name     string
		platform constant.TaskPlatform
		wantKey  string
	}{
		{"openai", constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeOpenAI)), "sora"},
		{"ali", constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAli)), "alibaba"},
		{"gemini", constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeGemini)), "google"},
		{"minimax", constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeMiniMax)), "hailuo"},
		{"vertexai", constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVertexAi)), "vertex-ai"},
		{"volcengine", constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVolcEngine)), "doubao"},
		{"kling", constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeKling)), "kling"},
		{"jimeng", constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeJimeng)), "jimeng"},
		{"vidu", constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeVidu)), "vidu"},
		{"doubao", constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeDoubaoVideo)), "doubao"},
		{"sora", constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSora)), "sora"},
		{"suno", constant.TaskPlatformSuno, "sunoapi"},
	}

	for _, tc := range legacy {
		t.Run(tc.name, func(t *testing.T) {
			plugin, found := ResolveTaskPluginForPlatform(generation, tc.platform)
			require.True(t, found, "platform %s must resolve to a built-in task plugin", tc.platform)
			require.NotNil(t, plugin)
			assert.Equal(t, tc.wantKey, plugin.Meta.Key)

			adaptor := GetTaskAdaptor(tc.platform)
			require.NotNil(t, adaptor, "platform %s must still yield a task adaptor", tc.platform)
		})
	}
}

// TestTaskPlatformUnavailableFailsLoudlyWhenPluginSystemDisabled protects the
// failure mode of the plugin migration: with the plugin system switched off a
// deleted native adaptor must fail with an actionable error instead of
// silently mis-routing the request.
func TestTaskPlatformUnavailableFailsLoudlyWhenPluginSystemDisabled(t *testing.T) {
	sora := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSora))
	jsplugin.DefaultRegistry.SetEnabled(false)
	t.Cleanup(func() { jsplugin.DefaultRegistry.SetEnabled(true) })

	assert.Nil(t, GetTaskAdaptor(sora))

	code, message := TaskPlatformUnavailableError(sora)
	assert.Equal(t, "task_plugin_system_disabled", code)
	assert.Equal(t, "the task plugin system is disabled on this gateway", message)
}
