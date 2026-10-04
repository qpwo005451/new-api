package operation_setting

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setModelWeightsForTest(t *testing.T, raw string) {
	t.Helper()
	previous := modelWeightSetting.Weights
	modelWeightSetting.Weights = raw
	t.Cleanup(func() { modelWeightSetting.Weights = previous })
}

func TestEffectiveModelWeightFallsBackToChannelWeight(t *testing.T) {
	setModelWeightsForTest(t, `[{"channel_id":9,"model":"deepseek-v4.1-flash","weight":20}]`)

	assert.Equal(t, 20, EffectiveModelWeight(9, "deepseek-v4.1-flash", 0), "override wins")
	assert.Equal(t, 20, EffectiveModelWeight(9, "DeepSeek-V4.1-Flash", 0), "model match is case-insensitive")
	assert.Equal(t, 7, EffectiveModelWeight(36, "deepseek-v4.1-flash", 7), "missing override falls back")
	assert.Equal(t, 7, EffectiveModelWeight(9, "glm-5.3-flash", 7), "other model falls back")
}

func TestEffectiveModelWeightExplicitZeroIsNotAFallback(t *testing.T) {
	setModelWeightsForTest(t, `[{"channel_id":9,"model":"glm-5.3-flash","weight":0}]`)
	assert.Equal(t, 0, EffectiveModelWeight(9, "glm-5.3-flash", 50), "explicit zero overrides the channel weight")
}

func TestEffectiveModelWeightIgnoresMalformedJSON(t *testing.T) {
	setModelWeightsForTest(t, `{not json`)
	assert.Equal(t, 3, EffectiveModelWeight(9, "deepseek-v4.1-flash", 3))
}

func TestEffectiveModelPriorityFallsBackToChannelPriority(t *testing.T) {
	setModelWeightsForTest(t, `[{"channel_id":9,"model":"deepseek-v4.1-flash","priority":500}]`)

	assert.Equal(t, int64(500), EffectiveModelPriority(9, "deepseek-v4.1-flash", 497), "override wins")
	assert.Equal(t, int64(500), EffectiveModelPriority(9, "DeepSeek-V4.1-Flash", 497), "model match is case-insensitive")
	assert.Equal(t, int64(497), EffectiveModelPriority(36, "deepseek-v4.1-flash", 497), "missing override falls back")
	assert.Equal(t, int64(497), EffectiveModelPriority(9, "glm-5.3-flash", 497), "other model falls back")
}

func TestEffectiveModelPriorityExplicitZeroIsNotAFallback(t *testing.T) {
	setModelWeightsForTest(t, `[{"channel_id":9,"model":"glm-5.3-flash","priority":0}]`)
	assert.Equal(t, int64(0), EffectiveModelPriority(9, "glm-5.3-flash", 500), "explicit zero overrides the channel priority")
}

func TestPriorityOverrideLeavesWeightAtChannelValue(t *testing.T) {
	setModelWeightsForTest(t, `[{"channel_id":9,"model":"m","priority":600}]`)
	assert.Equal(t, 7, EffectiveModelWeight(9, "m", 7))
}

func TestWeightOverrideLeavesPriorityAtChannelValue(t *testing.T) {
	setModelWeightsForTest(t, `[{"channel_id":9,"model":"m","weight":5}]`)
	assert.Equal(t, int64(500), EffectiveModelPriority(9, "m", 500))
}

func TestEffectiveModelPriorityIgnoresMalformedJSON(t *testing.T) {
	setModelWeightsForTest(t, `{not json`)
	assert.Equal(t, int64(9), EffectiveModelPriority(9, "deepseek-v4.1-flash", 9))
}

func TestModelWeightOverrideMergesPriorityAndWeight(t *testing.T) {
	setModelWeightsForTest(t, `[{"channel_id":9,"model":"m","priority":500,"weight":50}]`)
	assert.Equal(t, int64(500), EffectiveModelPriority(9, "m", 497))
	assert.Equal(t, 50, EffectiveModelWeight(9, "m", 1))
}

func TestListModelWeightOverridesFiltersByChannel(t *testing.T) {
	setModelWeightsForTest(t, `[{"channel_id":9,"model":"a","weight":1},{"channel_id":36,"model":"b","weight":2},{"channel_id":9,"model":"c","weight":3}]`)
	got := ListModelWeightOverrides(9)
	require.Len(t, got, 2)
	assert.Equal(t, "a", got[0].Model)
	assert.Equal(t, "c", got[1].Model)
	assert.Empty(t, ListModelWeightOverrides(999))
}

func TestValidateModelWeights(t *testing.T) {
	require.NoError(t, ValidateModelWeights(""))
	require.NoError(t, ValidateModelWeights("[]"))
	require.NoError(t, ValidateModelWeights(`[{"channel_id":9,"model":"a","weight":0}]`))

	assert.Error(t, ValidateModelWeights(`{"channel_id":9}`), "object is not an array")
	assert.Error(t, ValidateModelWeights(`null`), "null is not an array")
	assert.Error(t, ValidateModelWeights(`[{"channel_id":0,"model":"a","weight":1}]`), "channel id must be positive")
	assert.Error(t, ValidateModelWeights(`[{"channel_id":9,"model":"  ","weight":1}]`), "model required")
	assert.Error(t, ValidateModelWeights(`[{"channel_id":9,"model":"a","weight":1000001}]`), "weight upper bound")
	assert.Error(t, ValidateModelWeights(`[{"channel_id":9,"model":"a","weight":1},{"channel_id":9,"model":"A","weight":2}]`), "duplicate (channel, model) case-insensitive")
}

func TestValidateModelWeightsAcceptsPriority(t *testing.T) {
	require.NoError(t, ValidateModelWeights(`[{"channel_id":9,"model":"a","priority":500}]`))
	require.NoError(t, ValidateModelWeights(`[{"channel_id":9,"model":"a","priority":0}]`))
	require.NoError(t, ValidateModelWeights(`[{"channel_id":9,"model":"a","priority":500,"weight":10}]`))

	assert.Error(t, ValidateModelWeights(`[{"channel_id":9,"model":"a","priority":1000000001}]`), "priority upper bound")
	assert.Error(t, ValidateModelWeights(`[{"channel_id":9,"model":"a","priority":-1}]`), "priority lower bound")
	assert.Error(t, ValidateModelWeights(`[{"channel_id":9,"model":"a"}]`), "an entry must set a weight or a priority")
}

func TestValidateModelWeightPresets(t *testing.T) {
	require.NoError(t, ValidateModelWeightPresets(""))
	require.NoError(t, ValidateModelWeightPresets("[]"))
	require.NoError(t, ValidateModelWeightPresets(`[{"name":"默认","weights":[]}]`), "an empty preset clears overrides")
	require.NoError(t, ValidateModelWeightPresets(`[{"name":"全部 ollama","weights":[{"channel_id":46,"model":"deepseek-v4.1-flash","priority":501,"weight":100}]}]`))

	assert.Error(t, ValidateModelWeightPresets(`{"name":"x"}`), "object is not an array")
	assert.Error(t, ValidateModelWeightPresets(`null`), "null is not an array")
	assert.Error(t, ValidateModelWeightPresets(`[{"weights":[]}]`), "name required")
	assert.Error(t, ValidateModelWeightPresets(`[{"name":"  ","weights":[]}]`), "blank name")
	assert.Error(t, ValidateModelWeightPresets(`[{"name":"a","weights":[]},{"name":"A","weights":[]}]`), "duplicate name case-insensitive")
	assert.Error(t, ValidateModelWeightPresets(`[{"name":"a"}]`), "weights must be an explicit array")
	assert.Error(t, ValidateModelWeightPresets(`[{"name":"a","weights":[{"channel_id":0,"model":"m","weight":1}]}]`), "entry rules are reused")
	assert.Error(t, ValidateModelWeightPresets(`[{"name":"a","weights":[{"channel_id":9,"model":"m"}]}]`), "entry must set a weight or a priority")
}

func TestMaxModelWeightPresetNameLength(t *testing.T) {
	long := strings.Repeat("a", MaxModelWeightPresetNameLength+1)
	assert.Error(t, ValidateModelWeightPresets(`[{"name":"`+long+`","weights":[]}]`))
}
