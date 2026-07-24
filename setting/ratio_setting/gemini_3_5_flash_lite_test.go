package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGemini35FlashLitePricing(t *testing.T) {
	InitRatioSettings()

	inputRatio, found, matchedModel := GetModelRatio("gemini-3.5-flash-lite")
	require.True(t, found)
	assert.Equal(t, "gemini-3.5-flash-lite", matchedModel)
	assert.Equal(t, 0.125, inputRatio)
	assert.Equal(t, 6.0, GetCompletionRatio("gemini-3.5-flash-lite"))

	cacheRatio, found := GetCacheRatio("gemini-3.5-flash-lite")
	require.True(t, found)
	assert.Equal(t, 0.1, cacheRatio)
}
