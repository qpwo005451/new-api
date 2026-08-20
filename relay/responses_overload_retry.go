package relay

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

const (
	responsesOverloadRetryPrefixLimit       = 1 << 20
	inputReasoningPassbackErrorMessage      = "The `reasoning_content` in the thinking mode must be passed back to the API."
	inputDeepSeekV4FlashModel               = "deepseek-v4-flash"
	inputAPIHost                            = "ai.input.im"
	inputReasoningPassbackRetryExhaustedKey = "input_reasoning_passback_retry_exhausted"
	inputReasoningPassbackMaxRetries        = 5
)

type responsesPreOutputRetryReason int

const (
	responsesPreOutputRetryNone responsesPreOutputRetryReason = iota
	responsesPreOutputRetryOverload
	responsesPreOutputRetryInputReasoningPassback
)

type responsesOverloadRetryAdaptor interface {
	DoRequest(*gin.Context, *relaycommon.RelayInfo, io.Reader) (any, error)
}

type responsesStreamErrorEnvelope struct {
	Type     string `json:"type"`
	Code     any    `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
	Error    any    `json:"error,omitempty"`
	Response *struct {
		Error any `json:"error,omitempty"`
	} `json:"response,omitempty"`
}

type prefixedResponseBody struct {
	io.Reader
	closer io.Closer
}

func (body *prefixedResponseBody) Close() error {
	return body.closer.Close()
}

func shouldUseResponsesOverloadRetry(info *relaycommon.RelayInfo) (int, bool) {
	if info == nil || info.RelayFormat != types.RelayFormatOpenAIResponses {
		return 0, false
	}
	inputReasoningRetry := isInputReasoningPassbackRetryTarget(info)
	setting := operation_setting.GetResponsesOverloadRetrySetting()
	if setting.Enabled && info.IsStream {
		return setting.MaxRetries, true
	}
	if inputReasoningRetry {
		return 0, true
	}
	return 0, false
}

func isInputReasoningPassbackRetryTarget(info *relaycommon.RelayInfo) bool {
	if info == nil || info.RelayMode != relayconstant.RelayModeResponses ||
		info.ChannelType != constant.ChannelTypeOpenAI ||
		model_setting.GetGlobalSettings().PassThroughRequestEnabled ||
		info.ChannelSetting.PassThroughBodyEnabled ||
		!strings.EqualFold(strings.TrimSpace(info.UpstreamModelName), inputDeepSeekV4FlashModel) {
		return false
	}
	baseURL := strings.TrimSpace(info.ChannelBaseUrl)
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" {
		parsed, err = url.Parse("//" + baseURL)
		if err != nil {
			return false
		}
	}
	return strings.EqualFold(parsed.Hostname(), inputAPIHost)
}

func doResponsesRequestWithOverloadRetry(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	adaptor responsesOverloadRetryAdaptor,
	requestBodyStorage common.BodyStorage,
	maxOverloadRetries int,
) (any, error) {
	originalCloseUpstreamConnection := info.CloseUpstreamConnection
	defer func() { info.CloseUpstreamConnection = originalCloseUpstreamConnection }()
	closeUpstreamConnection := false
	overloadRetries := 0
	inputReasoningPassbackRetries := 0
	for {
		info.CloseUpstreamConnection = originalCloseUpstreamConnection || closeUpstreamConnection
		if _, err := requestBodyStorage.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek Responses request body for overload retry: %w", err)
		}

		resp, err := adaptor.DoRequest(c, info, common.ReaderOnly(requestBodyStorage))
		if err != nil {
			return nil, err
		}
		httpResp, ok := resp.(*http.Response)
		if !ok || httpResp == nil {
			return resp, nil
		}

		retryReason := responsesPreOutputRetryNone
		if httpResp.StatusCode == http.StatusOK {
			if maxOverloadRetries > 0 && bufferResponsesStreamUntilProgress(httpResp) {
				retryReason = responsesPreOutputRetryOverload
			}
		} else {
			retryReason = bufferResponsesHTTPError(httpResp, info)
		}

		retryIndex := 0
		retryLimit := 0
		switch retryReason {
		case responsesPreOutputRetryOverload:
			retryIndex = overloadRetries
			retryLimit = maxOverloadRetries
		case responsesPreOutputRetryInputReasoningPassback:
			retryIndex = inputReasoningPassbackRetries
			retryLimit = inputReasoningPassbackMaxRetries
		}
		if retryReason == responsesPreOutputRetryNone || retryIndex >= retryLimit {
			if retryReason == responsesPreOutputRetryInputReasoningPassback {
				c.Set(inputReasoningPassbackRetryExhaustedKey, true)
			}
			return httpResp, nil
		}

		service.CloseResponseBodyGracefully(httpResp)
		message := "OpenAI Responses overload before output"
		switch retryReason {
		case responsesPreOutputRetryOverload:
			overloadRetries++
		case responsesPreOutputRetryInputReasoningPassback:
			message = "Input reasoning passback rejected before output"
			inputReasoningPassbackRetries++
			closeUpstreamConnection = true
		}
		logger.LogWarn(c, fmt.Sprintf(
			"%s; retrying same channel and request body (attempt %d/%d, channel #%d)",
			message,
			retryIndex+1,
			retryLimit,
			info.ChannelId,
		))
		if !waitResponsesPreOutputRetry(c, retryReason, retryIndex, httpResp.Header.Get("Retry-After")) {
			return nil, service.RelayRequestContext(c).Err()
		}
	}
}

func waitResponsesPreOutputRetry(
	c *gin.Context,
	reason responsesPreOutputRetryReason,
	retryIndex int,
	retryAfter string,
) bool {
	if reason != responsesPreOutputRetryInputReasoningPassback {
		return waitResponsesOverloadRetry(c, retryIndex, retryAfter)
	}
	delay := 100*time.Millisecond + time.Duration(rand.Int64N(int64(200*time.Millisecond)+1))
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-service.RelayRequestContext(c).Done():
		return false
	}
}

func waitResponsesOverloadRetry(c *gin.Context, retryIndex int, retryAfter string) bool {
	baseDelay := 300 * time.Millisecond * time.Duration(1<<min(retryIndex, 3))
	delay := baseDelay + time.Duration(rand.Int64N(int64(baseDelay/2)+1))
	if serverDelay := responsesRetryAfterDelay(retryAfter, time.Now()); serverDelay > delay {
		delay = serverDelay
	}
	if delay > 5*time.Second {
		delay = 5 * time.Second
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-service.RelayRequestContext(c).Done():
		return false
	}
}

func responsesRetryAfterDelay(value string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	when, err := http.ParseTime(value)
	if err != nil || !when.After(now) {
		return 0
	}
	return when.Sub(now)
}

// bufferResponsesStreamUntilProgress keeps lifecycle-only events away from the
// client until the stream either produces meaningful output or reports an
// overload. The original bytes are restored before returning in every case.
func bufferResponsesHTTPError(resp *http.Response, info *relaycommon.RelayInfo) responsesPreOutputRetryReason {
	if resp == nil || resp.Body == nil {
		return responsesPreOutputRetryNone
	}
	originalBody := resp.Body
	body, err := io.ReadAll(io.LimitReader(originalBody, responsesOverloadRetryPrefixLimit+1))
	resp.Body = &prefixedResponseBody{
		Reader: io.MultiReader(bytes.NewReader(body), originalBody),
		closer: originalBody,
	}
	if err != nil || len(body) > responsesOverloadRetryPrefixLimit {
		return responsesPreOutputRetryNone
	}
	var envelope responsesStreamErrorEnvelope
	if common.Unmarshal(body, &envelope) != nil {
		return responsesPreOutputRetryNone
	}
	if isResponsesOverloadError(envelope.Code, envelope.Type+" "+envelope.Message, envelope.Error) ||
		(envelope.Response != nil && isResponsesOverloadError(nil, "", envelope.Response.Error)) {
		return responsesPreOutputRetryOverload
	}
	if resp.StatusCode == http.StatusBadRequest && isInputReasoningPassbackRetryTarget(info) &&
		isInputReasoningPassbackError(envelope) {
		return responsesPreOutputRetryInputReasoningPassback
	}
	return responsesPreOutputRetryNone
}

func isInputReasoningPassbackError(envelope responsesStreamErrorEnvelope) bool {
	if strings.TrimSpace(envelope.Message) == inputReasoningPassbackErrorMessage {
		return true
	}
	if openAIErr := dto.GetOpenAIError(envelope.Error); openAIErr != nil {
		return strings.TrimSpace(openAIErr.Message) == inputReasoningPassbackErrorMessage
	}
	return false
}

func bufferResponsesStreamUntilProgress(resp *http.Response) bool {
	if resp == nil || resp.Body == nil {
		return false
	}

	originalBody := resp.Body
	reader := bufio.NewReader(originalBody)
	var prefix bytes.Buffer
	var dataLines []string

	restore := func() {
		resp.Body = &prefixedResponseBody{
			Reader: io.MultiReader(bytes.NewReader(prefix.Bytes()), reader),
			closer: originalBody,
		}
	}

	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			prefix.WriteString(line)
			trimmed := strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(trimmed, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
			}

			if trimmed == "" {
				if responsesStreamFrameIsOverload(dataLines) {
					restore()
					return true
				}
				if responsesStreamFrameHasProgress(dataLines) {
					restore()
					return false
				}
				dataLines = dataLines[:0]
			}
		}

		if prefix.Len() > responsesOverloadRetryPrefixLimit {
			restore()
			return false
		}
		if err != nil {
			if len(dataLines) > 0 && responsesStreamFrameIsOverload(dataLines) {
				restore()
				return true
			}
			restore()
			return false
		}
	}
}

func responsesStreamFrameIsOverload(dataLines []string) bool {
	payload := strings.Join(dataLines, "\n")
	if payload == "" || payload == "[DONE]" {
		return false
	}
	var envelope responsesStreamErrorEnvelope
	if common.UnmarshalJsonStr(payload, &envelope) != nil {
		return false
	}
	if envelope.Type != "error" && envelope.Type != "response.failed" && envelope.Type != "response.error" {
		return false
	}
	return isResponsesOverloadError(envelope.Code, envelope.Type+" "+envelope.Message, envelope.Error) ||
		(envelope.Response != nil && isResponsesOverloadError(nil, "", envelope.Response.Error))
}

func responsesStreamFrameHasProgress(dataLines []string) bool {
	payload := strings.Join(dataLines, "\n")
	if payload == "" {
		return false
	}
	if payload == "[DONE]" {
		return true
	}
	var envelope responsesStreamErrorEnvelope
	if common.UnmarshalJsonStr(payload, &envelope) != nil {
		return true
	}
	switch envelope.Type {
	case "response.created", "response.in_progress", "response.queued":
		return false
	default:
		return true
	}
}

func isResponsesOverloadError(code any, message string, errorField any) bool {
	parts := []string{fmt.Sprint(code), message}
	if openAIErr := dto.GetOpenAIError(errorField); openAIErr != nil {
		parts = append(parts, openAIErr.Type, fmt.Sprint(openAIErr.Code), openAIErr.Message)
	}
	text := strings.ToLower(strings.Join(parts, " "))
	return strings.Contains(text, "service_unavailable_error") ||
		strings.Contains(text, "server_is_overloaded") ||
		strings.Contains(text, "overloaded_error") ||
		strings.Contains(text, "our servers are currently overloaded")
}
