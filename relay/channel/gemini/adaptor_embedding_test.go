package gemini

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestConvertEmbeddingRequestGeminiEmbedding2PreservesDimensions(t *testing.T) {
	t.Parallel()

	dimensions := 1024
	request, err := (&Adaptor{}).ConvertEmbeddingRequest(nil, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gemini-embedding-2",
		},
	}, dto.EmbeddingRequest{
		Input:      "test",
		Dimensions: &dimensions,
	})
	require.NoError(t, err)

	payload, ok := request.(map[string]interface{})
	require.True(t, ok)
	requests, ok := payload["requests"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, requests, 1)
	require.Equal(t, 1024, requests[0]["outputDimensionality"])
}
