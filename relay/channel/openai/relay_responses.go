package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	if responsesResponse.HasImageGenerationCall() {
		c.Set("image_generation_call", true)
		c.Set("image_generation_call_quality", responsesResponse.GetQuality())
		c.Set("image_generation_call_size", responsesResponse.GetSize())
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := dto.Usage{}
	if responsesResponse.Usage != nil {
		usage.PromptTokens = responsesResponse.Usage.InputTokens
		usage.CompletionTokens = responsesResponse.Usage.OutputTokens
		usage.TotalTokens = responsesResponse.Usage.TotalTokens
		if responsesResponse.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = responsesResponse.Usage.InputTokensDetails.CachedTokens
		}
	}
	if info == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return &usage, nil
	}
	// 解析 Tools 用量
	for _, tool := range responsesResponse.Tools {
		buildToolinfo, ok := info.ResponsesUsageInfo.BuiltInTools[common.Interface2String(tool["type"])]
		if !ok || buildToolinfo == nil {
			logger.LogError(c, fmt.Sprintf("BuiltInTools not found for tool type: %v", tool["type"]))
			continue
		}
		buildToolinfo.CallCount++
	}
	return &usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder
	responseID := helper.GetResponseID(c)
	firstOutputSeen := false
	toolOutputIndex := 0
	terminalEventSeen := false
	var streamErr *types.NewAPIError

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
			sr.Error(err)
			return
		}
		if streamResponse.Type == "response.output_text.delta" && !firstOutputSeen {
			if toolCalls, ok := parseTextToolCalls(streamResponse.Delta, info); ok {
				for _, toolCall := range toolCalls {
					callID := fmt.Sprintf("%s_tool_%d", responseID, toolOutputIndex)
					outputIndex := toolOutputIndex
					toolOutputIndex++
					emitTextToolCallEvent(c, sr, dto.ResponsesStreamResponse{
						Type:        dto.ResponsesOutputTypeItemAdded,
						OutputIndex: &outputIndex,
						ItemID:      callID,
						Item: &dto.ResponsesOutput{
							Type:      "function_call",
							ID:        callID,
							Status:    "in_progress",
							CallId:    callID,
							Name:      toolCall.Name,
							Arguments: []byte(`""`),
						},
					})
					emitTextToolCallEvent(c, sr, dto.ResponsesStreamResponse{
						Type:        "response.function_call_arguments.delta",
						OutputIndex: &outputIndex,
						ItemID:      callID,
						Delta:       string(toolCall.Arguments),
					})
					emitTextToolCallEvent(c, sr, dto.ResponsesStreamResponse{
						Type:        "response.function_call_arguments.done",
						OutputIndex: &outputIndex,
						ItemID:      callID,
					})
					arguments, err := common.Marshal(string(toolCall.Arguments))
					if err != nil {
						sr.Error(err)
						return
					}
					emitTextToolCallEvent(c, sr, dto.ResponsesStreamResponse{
						Type:        dto.ResponsesOutputTypeItemDone,
						OutputIndex: &outputIndex,
						Item: &dto.ResponsesOutput{
							Type:      "function_call",
							ID:        callID,
							Status:    "completed",
							CallId:    callID,
							Name:      toolCall.Name,
							Arguments: arguments,
						},
					})
					if sr.IsStopped() {
						return
					}
				}
				firstOutputSeen = true
				return
			}
		}
		sendResponsesStreamData(c, streamResponse, data)
		switch streamResponse.Type {
		case "error":
			if streamResponse.Message == "" {
				return
			}
			streamErr = types.WithOpenAIError(types.OpenAIError{
				Message: streamResponse.Message,
				Type:    "upstream_error",
				Code:    streamResponse.Code,
				Param:   streamResponse.Param,
			}, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
			sr.Error(streamErr)
		case "response.failed", "response.error":
			if streamErr == nil && streamResponse.Response != nil {
				if oaiErr := streamResponse.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Message != "" {
					streamErr = types.WithOpenAIError(*oaiErr, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
				}
			}
			if streamErr == nil {
				streamErr = types.NewOpenAIError(
					fmt.Errorf("responses stream error: %s", streamResponse.Type),
					types.ErrorCodeBadResponse,
					http.StatusBadGateway,
					types.ErrOptionWithSkipRetry(),
				)
			}
			sr.Stop(streamErr)
			return
		case "response.completed", "response.done", "response.incomplete":
			terminalEventSeen = true
			if streamResponse.Response != nil {
				if streamResponse.Response.Usage != nil {
					if streamResponse.Response.Usage.InputTokens != 0 {
						usage.PromptTokens = streamResponse.Response.Usage.InputTokens
					}
					if streamResponse.Response.Usage.OutputTokens != 0 {
						usage.CompletionTokens = streamResponse.Response.Usage.OutputTokens
					}
					if streamResponse.Response.Usage.TotalTokens != 0 {
						usage.TotalTokens = streamResponse.Response.Usage.TotalTokens
					}
					if streamResponse.Response.Usage.InputTokensDetails != nil {
						usage.PromptTokensDetails.CachedTokens = streamResponse.Response.Usage.InputTokensDetails.CachedTokens
					}
				}
				if streamResponse.Response.HasImageGenerationCall() {
					c.Set("image_generation_call", true)
					c.Set("image_generation_call_quality", streamResponse.Response.GetQuality())
					c.Set("image_generation_call_size", streamResponse.Response.GetSize())
				}
			}
		case "response.output_text.delta":
			firstOutputSeen = true
			// 处理输出文本
			responseTextBuilder.WriteString(streamResponse.Delta)
		case "response.reasoning_summary_text.delta", dto.ResponsesOutputTypeItemAdded:
			firstOutputSeen = true
		case dto.ResponsesOutputTypeItemDone:
			firstOutputSeen = true
			// 函数调用处理
			if streamResponse.Item != nil {
				switch streamResponse.Item.Type {
				case dto.BuildInCallWebSearchCall:
					if info != nil && info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools != nil {
						if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool != nil {
							webSearchTool.CallCount++
						}
					}
				}
			}
		}
	})

	if streamErr != nil {
		return usage, streamErr
	}
	if !terminalEventSeen {
		streamErr = types.NewOpenAIError(
			fmt.Errorf("responses stream ended before response.completed"),
			types.ErrorCodeBadResponse,
			http.StatusBadGateway,
			types.ErrOptionWithSkipRetry(),
		)
		if info.StreamStatus != nil {
			info.StreamStatus.RecordError(streamErr.Error())
		}
		return usage, streamErr
	}

	if usage.CompletionTokens == 0 {
		// 计算输出文本的 token 数量
		tempStr := responseTextBuilder.String()
		if len(tempStr) > 0 {
			// 非正常结束，使用输出文本的 token 数量
			completionTokens := service.CountTextToken(tempStr, info.UpstreamModelName)
			usage.CompletionTokens = completionTokens
		}
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.GetEstimatePromptTokens()
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	return usage, nil
}

func emitTextToolCallEvent(c *gin.Context, sr *helper.StreamResult, event dto.ResponsesStreamResponse) {
	data, err := common.Marshal(event)
	if err != nil {
		sr.Error(err)
		return
	}
	sendResponsesStreamData(c, event, string(data))
}

type textToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func parseTextToolCalls(text string, info *relaycommon.RelayInfo) ([]textToolCall, bool) {
	if info == nil || info.Request == nil || !strings.HasPrefix(text, "<TOOLCALL>") || !strings.HasSuffix(text, "</TOOLCALL>") {
		return nil, false
	}

	payload := strings.TrimSuffix(strings.TrimPrefix(text, "<TOOLCALL>"), "</TOOLCALL>")
	var toolCalls []textToolCall
	if err := common.Unmarshal([]byte(payload), &toolCalls); err != nil || len(toolCalls) == 0 {
		return nil, false
	}

	request, ok := info.Request.(*dto.OpenAIResponsesRequest)
	if !ok {
		return nil, false
	}
	allowedNames := make(map[string]struct{})
	for _, tool := range request.GetToolsMap() {
		if common.Interface2String(tool["type"]) != "function" {
			continue
		}
		if name := strings.TrimSpace(common.Interface2String(tool["name"])); name != "" {
			allowedNames[name] = struct{}{}
		}
	}

	for i := range toolCalls {
		toolCalls[i].Name = strings.TrimSpace(toolCalls[i].Name)
		if _, ok := allowedNames[toolCalls[i].Name]; !ok || len(toolCalls[i].Arguments) == 0 || !json.Valid(toolCalls[i].Arguments) {
			return nil, false
		}
		var arguments map[string]any
		if err := common.Unmarshal(toolCalls[i].Arguments, &arguments); err != nil {
			return nil, false
		}
	}
	return toolCalls, true
}
