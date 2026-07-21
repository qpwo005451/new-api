package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestBuildClaudeUsageFromOpenAIUsageNormalizesCacheWritePrefixes(t *testing.T) {
	usage := buildClaudeUsageFromOpenAIUsage(&dto.Usage{
		PromptTokens:     3619,
		CompletionTokens: 36,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:     2921,
			CacheWriteTokens: 3616,
		},
	})

	require.NotNil(t, usage)
	require.Equal(t, 0, usage.InputTokens)
	require.Equal(t, 2921, usage.CacheReadInputTokens)
	require.Equal(t, 3616, usage.CacheCreationInputTokens)
	require.Equal(t, 36, usage.OutputTokens)
}
