package types

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestErrOptionWithHideErrMsgPreservesCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("dial tcp: connection refused")
	apiErr := NewError(
		cause,
		ErrorCodeDoRequestFailed,
		ErrOptionWithHideErrMsg("upstream connection was refused"),
	)

	require.Equal(t, "upstream connection was refused", apiErr.Error())
	require.ErrorIs(t, apiErr, cause)
}

func TestWithOpenAIErrorDropsProviderMetadata(t *testing.T) {
	t.Parallel()

	apiErr := WithOpenAIError(OpenAIError{
		Message:  "provider failed",
		Type:     "server_error",
		Code:     "server_error",
		Metadata: []byte(`{"raw":"Authorization: Bearer secret-token"}`),
	}, 500)

	require.Equal(t, "provider failed", apiErr.Error())
	require.Empty(t, apiErr.ToOpenAIError().Metadata)
	require.NotContains(t, apiErr.Error(), "secret-token")
}
