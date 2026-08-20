package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type responsesOverloadRetryTestAdaptor struct {
	responses                []*http.Response
	bodies                   []string
	closeUpstreamConnections []bool
}

func (adaptor *responsesOverloadRetryTestAdaptor) DoRequest(_ *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (any, error) {
	requestBody, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	adaptor.bodies = append(adaptor.bodies, string(requestBody))
	adaptor.closeUpstreamConnections = append(adaptor.closeUpstreamConnections, info.CloseUpstreamConnection)
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

func TestShouldUseResponsesOverloadRetryIncludesNonStreamingInputReasoningPassback(t *testing.T) {
	inputInfo := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelBaseUrl:    "https://ai.input.im/api",
			UpstreamModelName: "deepseek-v4-flash",
		},
		RelayFormat: types.RelayFormatOpenAIResponses,
		IsStream:    false,
		RelayMode:   relayconstant.RelayModeResponses,
	}
	_, enabled := shouldUseResponsesOverloadRetry(inputInfo)
	assert.True(t, enabled)

	ordinaryInfo := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    "https://api.openai.com",
			UpstreamModelName: "gpt-5.6-sol",
		},
		RelayFormat: types.RelayFormatOpenAIResponses,
		IsStream:    false,
		RelayMode:   relayconstant.RelayModeResponses,
	}
	_, enabled = shouldUseResponsesOverloadRetry(ordinaryInfo)
	assert.False(t, enabled)
}

func TestResponsesInputPassbackRetryDoesNotBufferSuccessfulStreamWhenOverloadRetryDisabled(t *testing.T) {
	storage, err := common.CreateBodyStorage([]byte(`{"model":"deepseek-v4-flash"}`))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	streamBody := &countingReadCloser{Reader: strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n")}
	adaptor := &responsesOverloadRetryTestAdaptor{responses: []*http.Response{
		{StatusCode: http.StatusOK, Body: streamBody},
	}}

	responseAny, err := doResponsesRequestWithOverloadRetry(
		responsesRetryTestContext(),
		&relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType: constant.ChannelTypeOpenAI, ChannelId: 9,
				ChannelBaseUrl: "https://ai.input.im", UpstreamModelName: "deepseek-v4-flash",
			},
			RelayMode: relayconstant.RelayModeResponses,
		},
		adaptor,
		storage,
		0,
	)
	require.NoError(t, err)
	assert.Zero(t, streamBody.reads)
	assert.Same(t, streamBody, responseAny.(*http.Response).Body)
}

type countingReadCloser struct {
	io.Reader
	reads int
}

func (body *countingReadCloser) Read(p []byte) (int, error) {
	body.reads++
	return body.Reader.Read(p)
}

func (body *countingReadCloser) Close() error { return nil }

func TestResponsesOverloadRetryReplaysInputReasoningPassbackError(t *testing.T) {
	requestJSON := `{"model":"deepseek-v4-flash","messages":[{"role":"assistant","reasoning_content":"prior thought","tool_calls":[]}],"stream":true}`
	storage, err := common.CreateBodyStorage([]byte(requestJSON))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	errorBody := `{"message":"The ` + "`reasoning_content`" + ` in the thinking mode must be passed back to the API.","type":"invalid_request_error","param":"","code":null}`
	successStream := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"
	adaptor := &responsesOverloadRetryTestAdaptor{responses: []*http.Response{
		{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(errorBody)),
		},
		responsesRetryTestResponse(successStream),
	}}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelId:         9,
			ChannelBaseUrl:    "https://ai.input.im",
			UpstreamModelName: "deepseek-v4-flash",
		},
		IsStream:  true,
		RelayMode: relayconstant.RelayModeResponses,
	}

	responseAny, err := doResponsesRequestWithOverloadRetry(
		responsesRetryTestContext(),
		info,
		adaptor,
		storage,
		1,
	)
	require.NoError(t, err)
	require.Len(t, adaptor.bodies, 2)
	assert.Equal(t, requestJSON, adaptor.bodies[0])
	assert.Equal(t, adaptor.bodies[0], adaptor.bodies[1])
	assert.Equal(t, []bool{false, true}, adaptor.closeUpstreamConnections)
	assert.False(t, info.CloseUpstreamConnection)

	response := responseAny.(*http.Response)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, successStream, string(body))
}

func TestResponsesOverloadRetryDoesNotReplayOtherBadRequests(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		model   string
		body    string
	}{
		{
			name:    "different host",
			baseURL: "https://input.codes",
			model:   "deepseek-v4-flash",
			body:    `{"message":"The ` + "`reasoning_content`" + ` in the thinking mode must be passed back to the API."}`,
		},
		{
			name:    "different model",
			baseURL: "https://ai.input.im",
			model:   "gpt-5.6-luna",
			body:    `{"message":"The ` + "`reasoning_content`" + ` in the thinking mode must be passed back to the API."}`,
		},
		{
			name:    "different message",
			baseURL: "https://ai.input.im",
			model:   "deepseek-v4-flash",
			body:    `{"message":"reasoning_content is invalid"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, err := common.CreateBodyStorage([]byte(`{"model":"deepseek-v4-flash"}`))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, storage.Close()) })
			adaptor := &responsesOverloadRetryTestAdaptor{responses: []*http.Response{
				{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(tt.body))},
				responsesRetryTestResponse("unused"),
			}}

			responseAny, err := doResponsesRequestWithOverloadRetry(
				responsesRetryTestContext(),
				&relaycommon.RelayInfo{
					ChannelMeta: &relaycommon.ChannelMeta{
						ChannelType: constant.ChannelTypeOpenAI, ChannelId: 9, ChannelBaseUrl: tt.baseURL, UpstreamModelName: tt.model,
					},
					RelayMode: relayconstant.RelayModeResponses,
				},
				adaptor,
				storage,
				1,
			)
			require.NoError(t, err)
			assert.Len(t, adaptor.bodies, 1)
			assert.Equal(t, http.StatusBadRequest, responseAny.(*http.Response).StatusCode)
		})
	}
}

func TestResponsesOverloadRetryKeepsIndependentBudgetsForMixedErrors(t *testing.T) {
	requestJSON := `{"model":"deepseek-v4-flash","stream":true}`
	storage, err := common.CreateBodyStorage([]byte(requestJSON))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	passbackBody := `{"message":"The ` + "`reasoning_content`" + ` in the thinking mode must be passed back to the API."}`
	overloadBody := `{"error":{"type":"service_unavailable_error","code":"server_is_overloaded"}}`
	adaptor := &responsesOverloadRetryTestAdaptor{responses: []*http.Response{
		{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(passbackBody))},
		{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader(overloadBody))},
		{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(passbackBody))},
		responsesRetryTestResponse("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"),
	}}
	c := responsesRetryTestContext()
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI, ChannelId: 9,
			ChannelBaseUrl: "https://ai.input.im", UpstreamModelName: "deepseek-v4-flash",
		},
		RelayMode: relayconstant.RelayModeResponses,
	}

	_, err = doResponsesRequestWithOverloadRetry(c, info, adaptor, storage, 1)
	require.NoError(t, err)
	assert.Len(t, adaptor.bodies, 4)
	assert.Equal(t, []bool{false, true, true, true}, adaptor.closeUpstreamConnections)
	assert.False(t, c.GetBool(inputReasoningPassbackRetryExhaustedKey))
}

func TestResponsesOverloadRetryMarksExhaustedInputReasoningPassbackError(t *testing.T) {
	storage, err := common.CreateBodyStorage([]byte(`{"model":"deepseek-v4-flash"}`))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	errorBody := `{"message":"The ` + "`reasoning_content`" + ` in the thinking mode must be passed back to the API."}`
	responses := make([]*http.Response, inputReasoningPassbackMaxRetries+1)
	for i := range responses {
		responses[i] = &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(errorBody)),
		}
	}
	adaptor := &responsesOverloadRetryTestAdaptor{responses: responses}
	c := responsesRetryTestContext()
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType: constant.ChannelTypeOpenAI, ChannelId: 9, ChannelBaseUrl: "https://ai.input.im", UpstreamModelName: "deepseek-v4-flash",
	}, RelayMode: relayconstant.RelayModeResponses}

	_, err = doResponsesRequestWithOverloadRetry(c, info, adaptor, storage, 0)
	require.NoError(t, err)
	assert.True(t, c.GetBool(inputReasoningPassbackRetryExhaustedKey))
	assert.Len(t, adaptor.bodies, inputReasoningPassbackMaxRetries+1)
	assert.False(t, adaptor.closeUpstreamConnections[0])
	for _, closeConnection := range adaptor.closeUpstreamConnections[1:] {
		assert.True(t, closeConnection)
	}
	assert.False(t, info.CloseUpstreamConnection)

	baseErr := types.NewErrorWithStatusCode(
		assert.AnError,
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)
	originalAutomaticDisable := common.AutomaticDisableChannelEnabled
	originalDisableRanges := operation_setting.AutomaticDisableStatusCodeRanges
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: http.StatusBadRequest, End: http.StatusBadRequest}}
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalAutomaticDisable
		operation_setting.AutomaticDisableStatusCodeRanges = originalDisableRanges
	})
	assert.True(t, service.ShouldDisableChannel(baseErr))

	finalErr := markInputReasoningPassbackRetryExhaustedError(c, baseErr)
	assert.True(t, types.IsSkipRetryError(finalErr))
	assert.False(t, service.ShouldDisableChannel(finalErr))
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

func TestResponsesOverloadRetryReplaysServerErrorMessageBeforeOutput(t *testing.T) {
	requestJSON := `{"model":"gpt-5.6-luna","stream":true}`
	storage, err := common.CreateBodyStorage([]byte(requestJSON))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })

	overloadStream := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_failed\"}}\n\n" +
		"data: {\"type\":\"response.failed\",\"response\":{\"id\":\"resp_failed\",\"error\":{\"code\":\"server_error\",\"message\":\"Our servers are currently overloaded. Please try again later.\"}}}\n\n"
	successStream := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n"
	adaptor := &responsesOverloadRetryTestAdaptor{responses: []*http.Response{
		responsesRetryTestResponse(overloadStream),
		responsesRetryTestResponse(successStream),
	}}

	responseAny, err := doResponsesRequestWithOverloadRetry(
		responsesRetryTestContext(),
		&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 36}},
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
