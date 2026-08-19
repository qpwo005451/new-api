package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResponsesOverloadRetrySettingDefaultsToDisabled(t *testing.T) {
	original := responsesOverloadRetrySetting
	responsesOverloadRetrySetting = ResponsesOverloadRetrySetting{
		Enabled:    false,
		MaxRetries: defaultResponsesOverloadRetryMaxRetries,
	}
	t.Cleanup(func() { responsesOverloadRetrySetting = original })

	setting := GetResponsesOverloadRetrySetting()
	assert.False(t, setting.Enabled)
	assert.Equal(t, 2, setting.MaxRetries)
}

func TestResponsesOverloadRetrySettingNormalizesRetryLimit(t *testing.T) {
	original := responsesOverloadRetrySetting
	t.Cleanup(func() { responsesOverloadRetrySetting = original })

	responsesOverloadRetrySetting.MaxRetries = 0
	assert.Equal(t, 2, GetResponsesOverloadRetrySetting().MaxRetries)

	responsesOverloadRetrySetting.MaxRetries = 99
	assert.Equal(t, 5, GetResponsesOverloadRetrySetting().MaxRetries)
}
