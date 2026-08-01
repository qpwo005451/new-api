package ratio_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepSeekV4Pricing(t *testing.T) {
	InitRatioSettings()

	tests := []struct {
		model           string
		inputRatio      float64
		completionRatio float64
		cacheRatio      float64
	}{
		{
			model:           "deepseek-v4-flash",
			inputRatio:      0.14 / 2,
			completionRatio: 2,
			cacheRatio:      0.0028 / 0.14,
		},
		{
			model:           "deepseek-v4-flash-none",
			inputRatio:      0.14 / 2,
			completionRatio: 2,
			cacheRatio:      0.0028 / 0.14,
		},
		{
			model:           "deepseek-v4-flash-max",
			inputRatio:      0.14 / 2,
			completionRatio: 2,
			cacheRatio:      0.0028 / 0.14,
		},
		{
			model:           "deepseek-v4-pro",
			inputRatio:      0.435 / 2,
			completionRatio: 2,
			cacheRatio:      0.003625 / 0.435,
		},
		{
			model:           "deepseek-v4-pro-none",
			inputRatio:      0.435 / 2,
			completionRatio: 2,
			cacheRatio:      0.003625 / 0.435,
		},
		{
			model:           "deepseek-v4-pro-max",
			inputRatio:      0.435 / 2,
			completionRatio: 2,
			cacheRatio:      0.003625 / 0.435,
		},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			inputRatio, found, matchedModel := GetModelRatio(tt.model)
			require.True(t, found)
			assert.Equal(t, tt.model, matchedModel)
			assert.Equal(t, tt.inputRatio, inputRatio)
			assert.Equal(t, tt.completionRatio, GetCompletionRatio(tt.model))

			cacheRatio, found := GetCacheRatio(tt.model)
			require.True(t, found)
			assert.Equal(t, tt.cacheRatio, cacheRatio)
		})
	}
}
