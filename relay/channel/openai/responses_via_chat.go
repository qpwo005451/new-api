package openai

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func OaiChatToResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	var chatResp dto.OpenAITextResponse
	if err := common.Unmarshal(body, &chatResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := chatResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	info.ObserveResponseModel(chatResp.Model)
	if responseID := helper.GetResponseID(c); responseID != "" {
		chatResp.Id = responseID
	}
	convertResult, err := service.ConvertResponse(c, info, types.RelayFormatOpenAIResponses, &chatResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	responsesResp, ok := convertResult.Value.(*dto.OpenAIResponsesResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI responses response, got %T", convertResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	usage := convertResult.Usage
	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		responsesResp.Usage = relayconvert.UsageFromChatUsage(usage)
	}

	responseBody, err := common.Marshal(responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	if info != nil {
		info.UpstreamResponseID = strings.TrimSpace(responsesResp.ID)
	}
	service.CommitVirtualPoolResponseOwner(c, responsesResp.ID)
	service.IOCopyBytesGracefully(c, resp, responseBody)
	if info.AttemptOutcome != nil && info.AttemptOutcome.IsActive() {
		_ = info.AttemptOutcome.MarkSuccess()
	}
	return usage, nil
}

func OaiChatToResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	responseID := helper.GetResponseID(c)
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, relayconvert.ResponseStreamOptions{
		ID:                 responseID,
		Model:              info.UpstreamModelName,
		EmitSequenceNumber: true,
	})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	streamErr := (*types.NewAPIError)(nil)
	committed := false
	receivedFinishReason := false
	pendingEvents := make([]relayconvert.ChatToResponsesStreamEvent, 0, 4)
	info.DisablePing = true

	sendEvent := func(event relayconvert.ChatToResponsesStreamEvent) bool {
		data, err := common.Marshal(event.Payload)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		if err := helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: event.Type}, string(data)); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		return true
	}
	failResponsesStream := func(err error) bool {
		failureResults, handled := state.FailResponsesStream("server_error", err.Error(), "")
		if !handled {
			return false
		}
		for _, result := range failureResults {
			event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
			if !ok {
				streamErr = types.NewOpenAIError(fmt.Errorf("expected OAI responses stream event, got %T", result.Value), types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return true
			}
			if !sendEvent(event) {
				return true
			}
		}
		return true
	}
	commitEvents := func(events []relayconvert.ChatToResponsesStreamEvent) bool {
		for _, event := range events {
			if !sendEvent(event) {
				return false
			}
		}
		return true
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}

		var errorResp dto.OpenAITextResponse
		if err := common.UnmarshalJsonStr(data, &errorResp); err == nil {
			if oaiError := errorResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
				statusCode := streamErrorStatusCode(oaiError.Code, resp.StatusCode)
				if committed {
					// 已向下游写入过事件：按 Responses 协议补发失败终态事件，并把本此尝试标记为不可重试
					if failResponsesStream(fmt.Errorf("%s", oaiError.Message)) && streamErr != nil {
						sr.Stop(streamErr)
						return
					}
					streamErr = types.WithOpenAIError(*oaiError, statusCode, types.ErrOptionWithSkipRetry())
				} else {
					// 尚未向下游写入任何事件：保持可重试，不做下游写入
					streamErr = types.WithOpenAIError(*oaiError, statusCode)
				}
				sr.Stop(streamErr)
				return
			}
		}

		var chunk dto.ChatCompletionsStreamResponse
		if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
			logger.LogError(c, "failed to unmarshal chat stream response: "+err.Error())
			if failResponsesStream(err) {
				sr.Stop(streamErr)
				return
			}
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		if chunk.IsFinished() {
			receivedFinishReason = true
		}

		info.ObserveResponseModel(chunk.Model)
		results, err := service.ConvertStreamResponseChunk(c, info, state, &chunk)
		if err != nil {
			if failResponsesStream(err) {
				sr.Stop(streamErr)
				return
			}
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		events := make([]relayconvert.ChatToResponsesStreamEvent, 0, len(results))
		for _, result := range results {
			event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
			if !ok {
				streamErr = types.NewOpenAIError(fmt.Errorf("expected OAI responses stream event, got %T", result.Value), types.ErrorCodeBadResponse, http.StatusInternalServerError)
				sr.Stop(streamErr)
				return
			}
			events = append(events, event)
		}
		if !committed {
			pendingEvents = append(pendingEvents, events...)
			if !chatStreamChunkHasMeaningfulOutput(&chunk) {
				return
			}
			committed = true
			if !commitEvents(pendingEvents) {
				sr.Stop(streamErr)
				return
			}
			pendingEvents = nil
			return
		}
		if !commitEvents(events) {
			sr.Stop(streamErr)
		}
	})

	if streamErr != nil {
		return nil, streamErr
	}

	usage := state.Usage()
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, state.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		state.SetUsage(usage)
	}

	finalResults, err := service.FinalizeStreamResponse(c, info, state)
	if err != nil {
		if failResponsesStream(err) {
			return usage, streamErr
		}
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	finalEvents := make([]relayconvert.ChatToResponsesStreamEvent, 0, len(finalResults))
	for _, result := range finalResults {
		event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
		if !ok {
			return nil, types.NewOpenAIError(fmt.Errorf("expected OAI responses stream event, got %T", result.Value), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		finalEvents = append(finalEvents, event)
	}
	if !committed {
		pendingEvents = append(pendingEvents, finalEvents...)
		committed = true
		if !commitEvents(pendingEvents) {
			return nil, streamErr
		}
		if receivedFinishReason && info.AttemptOutcome != nil && info.AttemptOutcome.IsActive() {
			_ = info.AttemptOutcome.MarkSuccess()
		}
		return usage, nil
	}
	for _, event := range finalEvents {
		if !sendEvent(event) {
			return nil, streamErr
		}
	}

	if receivedFinishReason && info.AttemptOutcome != nil && info.AttemptOutcome.IsActive() {
		_ = info.AttemptOutcome.MarkSuccess()
	}

	return usage, nil
}

func chatStreamChunkHasMeaningfulOutput(chunk *dto.ChatCompletionsStreamResponse) bool {
	if chunk == nil {
		return false
	}
	for _, choice := range chunk.Choices {
		if choice.Delta.GetContentString() != "" || choice.Delta.GetReasoningContent() != "" || len(choice.Delta.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

func streamErrorStatusCode(code any, fallback int) int {
	statusCode := fallback
	switch value := code.(type) {
	case float64:
		statusCode = int(value)
	case int:
		statusCode = value
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
			statusCode = parsed
		}
	}
	if statusCode < 100 || statusCode > 599 {
		return http.StatusInternalServerError
	}
	return statusCode
}
