package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type responsesOverloadRetryTestAdaptor struct {
	responses []*http.Response
	bodies    []string
}

func (adaptor *responsesOverloadRetryTestAdaptor) DoRequest(_ *gin.Context, _ *relaycommon.RelayInfo, body io.Reader) (any, error) {
	requestBody, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	adaptor.bodies = append(adaptor.bodies, string(requestBody))
	response := adaptor.responses[0]
	adaptor.responses = adaptor.responses[1:]
	return response, nil
}

func responsesRetryTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	return context
}

func responsesRetryTestResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestResponsesOverloadRetryReplaysSameRequestBeforeOutput(t *testing.T) {
	requestJSON := `{"model":"gpt-5.6-sol","previous_response_id":"resp_previous","stream":true}`
	storage, err := common.CreateBodyStorage([]byte(requestJSON))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	overloadStream := "event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_failed\"}}\n\n" +
		"event: error\n" +
		"data: {\"type\":\"error\",\"error\":{\"type\":\"service_unavailable_error\",\"code\":\"server_is_overloaded\",\"message\":\"Our servers are currently overloaded. Please try again later.\"}}\n\n"
	successStream := "event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_success\"}}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"
	adaptor := &responsesOverloadRetryTestAdaptor{responses: []*http.Response{
		responsesRetryTestResponse(overloadStream),
		responsesRetryTestResponse(successStream),
	}}

	responseAny, err := doResponsesRequestWithOverloadRetry(
		responsesRetryTestContext(),
		&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 251}},
		adaptor,
		storage,
		1,
	)
	require.NoError(t, err)
	require.Len(t, adaptor.bodies, 2)
	assert.Equal(t, requestJSON, adaptor.bodies[0])
	assert.Equal(t, adaptor.bodies[0], adaptor.bodies[1])

	response := responseAny.(*http.Response)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, successStream, string(body))
}

func TestResponsesOverloadRetryDoesNotReplayAfterProgress(t *testing.T) {
	requestJSON := `{"model":"gpt-5.6-sol","stream":true}`
	storage, err := common.CreateBodyStorage([]byte(requestJSON))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	streamWithProgress := "event: response.created\n" +
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n" +
		"event: error\n" +
		"data: {\"type\":\"error\",\"code\":\"server_is_overloaded\",\"message\":\"Our servers are currently overloaded.\"}\n\n"
	adaptor := &responsesOverloadRetryTestAdaptor{responses: []*http.Response{
		responsesRetryTestResponse(streamWithProgress),
		responsesRetryTestResponse("unused"),
	}}

	responseAny, err := doResponsesRequestWithOverloadRetry(
		responsesRetryTestContext(),
		&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 251}},
		adaptor,
		storage,
		1,
	)
	require.NoError(t, err)
	assert.Len(t, adaptor.bodies, 1)

	response := responseAny.(*http.Response)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, streamWithProgress, string(body))
}

func TestResponsesOverloadRetryReplaysHTTPErrorBeforeOutput(t *testing.T) {
	requestJSON := `{"model":"gpt-5.6-sol","stream":true}`
	storage, err := common.CreateBodyStorage([]byte(requestJSON))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	errorBody := `{"error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"Our servers are currently overloaded."}}`
	successStream := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"
	adaptor := &responsesOverloadRetryTestAdaptor{responses: []*http.Response{
		{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader(errorBody)),
		},
		responsesRetryTestResponse(successStream),
	}}

	responseAny, err := doResponsesRequestWithOverloadRetry(
		responsesRetryTestContext(),
		&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 251}},
		adaptor,
		storage,
		1,
	)
	require.NoError(t, err)
	require.Len(t, adaptor.bodies, 2)
	assert.Equal(t, requestJSON, adaptor.bodies[1])
	response := responseAny.(*http.Response)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, successStream, string(body))
}

func TestResponsesOverloadRetryReturnsLastHTTPErrorAfterExhaustion(t *testing.T) {
	requestJSON := `{"model":"gpt-5.6-sol","stream":true}`
	storage, err := common.CreateBodyStorage([]byte(requestJSON))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	errorBody := `{"error":{"type":"service_unavailable_error","code":"server_is_overloaded"}}`
	adaptor := &responsesOverloadRetryTestAdaptor{responses: []*http.Response{
		{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader(errorBody)),
		},
		{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader(errorBody)),
		},
	}}

	responseAny, err := doResponsesRequestWithOverloadRetry(
		responsesRetryTestContext(),
		&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 251}},
		adaptor,
		storage,
		1,
	)
	require.NoError(t, err)
	require.Len(t, adaptor.bodies, 2)
	response := responseAny.(*http.Response)
	assert.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, errorBody, string(body))
}

func TestResponsesOverloadRetryReturnsLastStreamAfterExhaustion(t *testing.T) {
	requestJSON := `{"model":"gpt-5.6-sol","stream":true}`
	storage, err := common.CreateBodyStorage([]byte(requestJSON))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	firstStream := "data: {\"type\":\"error\",\"code\":\"server_is_overloaded\"}\n\n"
	lastStream := "data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed\",\"error\":{\"type\":\"service_unavailable_error\"}}}\n\n"
	adaptor := &responsesOverloadRetryTestAdaptor{responses: []*http.Response{
		responsesRetryTestResponse(firstStream),
		responsesRetryTestResponse(lastStream),
	}}

	responseAny, err := doResponsesRequestWithOverloadRetry(
		responsesRetryTestContext(),
		&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 251}},
		adaptor,
		storage,
		1,
	)
	require.NoError(t, err)
	require.Len(t, adaptor.bodies, 2)
	response := responseAny.(*http.Response)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, lastStream, string(body))
}

func TestResponsesRetryAfterDelay(t *testing.T) {
	now := time.Date(2026, time.August, 19, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{name: "seconds", value: "3", want: 3 * time.Second},
		{name: "http date", value: now.Add(4 * time.Second).Format(http.TimeFormat), want: 4 * time.Second},
		{name: "past date", value: now.Add(-time.Second).Format(http.TimeFormat), want: 0},
		{name: "invalid", value: "later", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, responsesRetryAfterDelay(tt.value, now))
		})
	}
}
