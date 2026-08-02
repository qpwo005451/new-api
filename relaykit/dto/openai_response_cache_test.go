package dto

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInputTokenDetailsCacheCreationTokensTotal(t *testing.T) {
	tests := []struct {
		name     string
		details  InputTokenDetails
		expected int
	}{
		{
			name:     "legacy cache creation",
			details:  InputTokenDetails{CachedCreationTokens: 12},
			expected: 12,
		},
		{
			name:     "native cache write wins",
			details:  InputTokenDetails{CachedCreationTokens: 12, CacheWriteTokens: 20},
			expected: 20,
		},
		{
			name:     "negative values clamp to zero",
			details:  InputTokenDetails{CachedCreationTokens: -1, CacheWriteTokens: -2},
			expected: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, test.details.CacheCreationTokensTotal())
		})
	}
}
