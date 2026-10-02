package operation_setting

import (
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
