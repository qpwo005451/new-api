package relay

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const opencodeBufferedStreamRetryAttempts = 1

type opencodeBufferedRetryAdaptor interface {
	DoRequest(*gin.Context, *relaycommon.RelayInfo, io.Reader) (any, error)
}

func shouldUseBufferedOpencodeStreamRetry(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if c == nil || info == nil || !info.IsStream || info.RelayFormat != types.RelayFormatOpenAI || info.RelayMode != relayconstant.RelayModeChatCompletions {
		return false
	}
	if !isBufferedOpencodeRetryModel(info.OriginModelName) && !isBufferedOpencodeRetryModel(info.UpstreamModelName) {
		return false
	}
	channelName := strings.ToLower(strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyChannelName)))
	baseURL := strings.ToLower(strings.TrimSpace(info.ChannelBaseUrl))
	return strings.Contains(channelName, "opencode-go") || strings.Contains(baseURL, "opencode")
}

func isBufferedOpencodeRetryModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return model == "deepseek-v4-flash" ||
		model == "deepseek-v4-flash-none" ||
		model == "deepseek-v4-flash-max" ||
		strings.HasSuffix(model, "/deepseek-v4-flash") ||
		strings.HasSuffix(model, "/deepseek-v4-flash-none") ||
		strings.HasSuffix(model, "/deepseek-v4-flash-max")
}

func doBufferedOpencodeStreamRetry(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	adaptor opencodeBufferedRetryAdaptor,
	requestBodyStorage common.BodyStorage,
	firstResp *http.Response,
	statusCodeMappingStr string,
) (*http.Response, *types.NewAPIError) {
	resp := firstResp
	for attempt := 0; attempt <= opencodeBufferedStreamRetryAttempts; attempt++ {
		if resp == nil {
			if _, err := requestBodyStorage.Seek(0, io.SeekStart); err != nil {
				return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			nextResp, err := adaptor.DoRequest(c, info, common.ReaderOnly(requestBodyStorage))
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
			}
			resp = nextResp.(*http.Response)
			info.IsStream = info.IsStream || strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream")
			if resp.StatusCode != http.StatusOK {
				newAPIError := service.RelayErrorHandler(c.Request.Context(), resp, false)
				service.RecordModelMonitorPassiveHTTPFailureAsync(info, resp.StatusCode)
				service.ResetStatusCode(newAPIError, statusCodeMappingStr)
				return nil, newAPIError
			}
		}

		body, complete, readErr := readCompleteChatCompletionSSE(resp.Body, info.RelayMode)
		service.CloseResponseBodyGracefully(resp)
		if complete {
			info.OpenCodeStreamRetryCount = attempt
			info.OpenCodeRecoveredAfterRetry = attempt > 0
			logger.LogInfo(c, fmt.Sprintf("opencode buffered stream completed on attempt %d", attempt+1))
			resp.Body = io.NopCloser(strings.NewReader(body))
			return resp, nil
		}
		if attempt >= opencodeBufferedStreamRetryAttempts {
			info.OpenCodeStreamRetryCount = attempt
			message := "upstream stream terminated unexpectedly before finish_reason"
			if readErr != nil {
				message += ": " + readErr.Error()
			}
			return nil, types.NewOpenAIError(
				errors.New(message),
				types.ErrorCodeReadResponseBodyFailed,
				http.StatusBadGateway,
				types.ErrOptionWithUpstreamErrorInfo("upstream_stream_terminated", ""),
			)
		}

		if readErr != nil {
			logger.LogWarn(c, fmt.Sprintf("opencode buffered stream retry after read error on attempt %d: %s", attempt+1, readErr.Error()))
		} else {
			logger.LogWarn(c, fmt.Sprintf("opencode buffered stream retry after missing finish_reason on attempt %d", attempt+1))
		}
		resp = nil
	}
	return nil, types.NewOpenAIError(fmt.Errorf("opencode buffered stream retry exhausted"), types.ErrorCodeReadResponseBodyFailed, http.StatusBadGateway)
}

func readCompleteChatCompletionSSE(body io.Reader, relayMode int) (string, bool, error) {
	scanner := helper.NewStreamScanner(body)
	scanner.Split(bufio.ScanLines)
	var builder strings.Builder
	receivedFinishReason := false
	for scanner.Scan() {
		line := scanner.Text()
		builder.WriteString(line)
		builder.WriteByte('\n')
		if len(line) < 6 {
			continue
		}
		if line[:5] != "data:" && line[:6] != "[DONE]" {
			continue
		}
		data := strings.TrimSpace(line[5:])
		if data == "" || strings.HasPrefix(data, "[DONE]") {
			continue
		}
		if bufferedStreamChunkHasFinishReason(relayMode, data) {
			receivedFinishReason = true
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF && !receivedFinishReason {
		return builder.String(), false, err
	}
	return builder.String(), receivedFinishReason, nil
}

func bufferedStreamChunkHasFinishReason(relayMode int, data string) bool {
	switch relayMode {
	case relayconstant.RelayModeChatCompletions:
		var streamResponse dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			return false
		}
		return streamResponse.IsFinished()
	case relayconstant.RelayModeCompletions:
		var streamResponse dto.CompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			return false
		}
		for _, choice := range streamResponse.Choices {
			if choice.FinishReason != "" {
				return true
			}
		}
	}
	return false
}
