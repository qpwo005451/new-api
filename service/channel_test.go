package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
)

func TestShouldTrackMultiKeyFailureClassifiesKeyHealthErrors(t *testing.T) {
	t.Setenv("MULTI_KEY_FAILURE_THRESHOLD", "10")

	tests := []struct {
		name     string
		err      *types.NewAPIError
		expected bool
	}{
		{
			name:     "upstream server error",
			err:      types.NewOpenAIError(errors.New("upstream unavailable"), "server_error", http.StatusInternalServerError),
			expected: true,
		},
		{
			name:     "rate limit",
			err:      types.NewOpenAIError(errors.New("rate limited"), "rate_limit", http.StatusTooManyRequests),
			expected: true,
		},
		{
			name:     "invalid Google API key message",
			err:      types.NewOpenAIError(errors.New("API key not valid. Please pass a valid API key."), "invalid_argument", http.StatusBadRequest),
			expected: true,
		},
		{
			name:     "client request error",
			err:      types.NewOpenAIError(errors.New("messages is required"), types.ErrorCodeInvalidRequest, http.StatusBadRequest),
			expected: false,
		},
		{
			name: "skip retry server error",
			err: types.NewOpenAIError(
				errors.New("local conversion failed"),
				types.ErrorCodeConvertRequestFailed,
				http.StatusInternalServerError,
				types.ErrOptionWithSkipRetry(),
			),
			expected: false,
		},
		{
			name: "upstream skip retry server error",
			err: types.NewOpenAIError(
				errors.New("upstream unavailable"),
				"mock_upstream_unavailable",
				http.StatusInternalServerError,
				types.ErrOptionWithSkipRetry(),
			),
			expected: true,
		},
		{
			name: "local channel override error",
			err: types.NewOpenAIError(
				errors.New("invalid channel override"),
				types.ErrorCodeChannelParamOverrideInvalid,
				http.StatusInternalServerError,
				types.ErrOptionWithSkipRetry(),
			),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, ShouldTrackMultiKeyFailure(test.err))
		})
	}
}

func TestShouldTrackMultiKeyFailureRequiresThreshold(t *testing.T) {
	t.Setenv("MULTI_KEY_FAILURE_THRESHOLD", "0")
	err := types.NewOpenAIError(errors.New("upstream unavailable"), "server_error", http.StatusInternalServerError)

	assert.False(t, ShouldTrackMultiKeyFailure(err))
}
