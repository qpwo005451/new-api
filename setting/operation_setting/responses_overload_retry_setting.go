package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

const (
	defaultResponsesOverloadRetryMaxRetries = 2
	maxResponsesOverloadRetryMaxRetries     = 5
)

// ResponsesOverloadRetrySetting controls same-channel retries for OpenAI
// Responses streams that fail with an overload before producing any output.
type ResponsesOverloadRetrySetting struct {
	Enabled    bool `json:"enabled"`
	MaxRetries int  `json:"max_retries"`
}

var responsesOverloadRetrySetting = ResponsesOverloadRetrySetting{
	Enabled:    false,
	MaxRetries: defaultResponsesOverloadRetryMaxRetries,
}

func init() {
	config.GlobalConfig.Register("responses_overload_retry_setting", &responsesOverloadRetrySetting)
}

func GetResponsesOverloadRetrySetting() ResponsesOverloadRetrySetting {
	setting := responsesOverloadRetrySetting
	if setting.MaxRetries < 1 {
		setting.MaxRetries = defaultResponsesOverloadRetryMaxRetries
	}
	if setting.MaxRetries > maxResponsesOverloadRetryMaxRetries {
		setting.MaxRetries = maxResponsesOverloadRetryMaxRetries
	}
	return setting
}
