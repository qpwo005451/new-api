package ollama

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

const maxOllamaToolCalls = 128

type ollamaChatStreamChunk struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at"`
	// chat
	Message *struct {
		Role      string           `json:"role"`
		Content   string           `json:"content"`
		Thinking  json.RawMessage  `json:"thinking"`
		ToolCalls []OllamaToolCall `json:"tool_calls"`
	} `json:"message"`
	// generate
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	DoneReason         string `json:"done_reason"`
	TotalDuration      int64  `json:"total_duration"`
	LoadDuration       int64  `json:"load_duration"`
	PromptEvalCount    int    `json:"prompt_eval_count"`
	EvalCount          int    `json:"eval_count"`
	PromptEvalDuration int64  `json:"prompt_eval_duration"`
	EvalDuration       int64  `json:"eval_duration"`
}

func ollamaToolCallsToOpenAI(toolCalls []OllamaToolCall, startIndex int, includeIndex bool) ([]dto.ToolCallResponse, int) {
	if len(toolCalls) == 0 {
		return nil, startIndex
	}
	result := make([]dto.ToolCallResponse, 0, len(toolCalls))
	for _, tc := range toolCalls {
		var argBytes []byte
		var err error
		if tc.Function.Arguments == nil {
			argBytes = []byte("{}")
		} else {
			argBytes, err = common.Marshal(tc.Function.Arguments)
			if err != nil || len(argBytes) == 0 {
				argBytes = []byte("{}")
			}
		}
		toolCallID := tc.ID
		if toolCallID == "" {
			toolCallID = fmt.Sprintf("call_%d", startIndex)
		}
		tr := dto.ToolCallResponse{
			ID:   toolCallID,
			Type: "function",
			Function: dto.FunctionResponse{
				Name:      tc.Function.Name,
				Arguments: string(argBytes),
			},
		}
		if includeIndex {
			tr.SetIndex(startIndex)
		}
		startIndex++
		result = append(result, tr)
	}
	return result, startIndex
}

func toUnix(ts string) int64 {
	if ts == "" {
		return time.Now().Unix()
	}
	// try time.RFC3339 or with nanoseconds
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t2, err2 := time.Parse(time.RFC3339, ts)
		if err2 == nil {
			return t2.Unix()
		}
		return time.Now().Unix()
	}
	return t.Unix()
}

// dedupeOllamaToolCalls collapses repeated ollama tool calls that share an id,
// or share a name and arguments without an id, so cumulative upstream frames
// cannot turn one tool call into hundreds. The returned flag reports that the
// remaining budget forced a truncation.
func dedupeOllamaToolCalls(calls []OllamaToolCall, toolCallIndex int, seenToolCalls map[string]struct{}) ([]OllamaToolCall, bool) {
	unique := make([]OllamaToolCall, 0, len(calls))
	for _, tc := range calls {
		args, _ := common.Marshal(tc.Function.Arguments)
		key := tc.ID + "\x00" + tc.Function.Name + "\x00" + string(args)
		if _, exists := seenToolCalls[key]; exists {
			continue
		}
		seenToolCalls[key] = struct{}{}
		unique = append(unique, tc)
	}
	truncated := false
	if room := maxOllamaToolCalls - toolCallIndex; len(unique) > room {
		unique = unique[:room]
		truncated = true
	}
	return unique, truncated
}

// buildOllamaChatStreamDelta converts one streamed ollama chat frame into the
// matching OpenAI chat-completions chunk. toolCallIndex tracks the running
// tool-call index across frames; the returned value is the updated index.
// truncated reports that the tool-call limit cut this frame short. Repeated
// tool calls with the same id, or the same name and arguments without an id,
// are collapsed so cumulative upstream frames cannot turn one tool call into
// hundreds.
func buildOllamaChatStreamDelta(chunk ollamaChatStreamChunk, model, responseId string, created int64, toolCallIndex int, seenToolCalls map[string]struct{}) (dto.ChatCompletionsStreamResponse, int, bool) {
	// delta content
	var content string
	if chunk.Message != nil {
		content = chunk.Message.Content
	} else {
		content = chunk.Response
	}
	delta := dto.ChatCompletionsStreamResponse{
		Id:      responseId,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Index: 0,
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Role: "assistant"},
		}},
	}
	if content != "" {
		delta.Choices[0].Delta.SetContentString(content)
	}
	if chunk.Message != nil && len(chunk.Message.Thinking) > 0 {
		raw := strings.TrimSpace(string(chunk.Message.Thinking))
		if raw != "" && raw != "null" {
			// Unmarshal the JSON string to get the actual content without quotes
			var thinkingContent string
			if err := common.Unmarshal(chunk.Message.Thinking, &thinkingContent); err == nil {
				delta.Choices[0].Delta.SetReasoningContent(thinkingContent)
			} else {
				// Fallback to raw string if it's not a JSON string
				delta.Choices[0].Delta.SetReasoningContent(raw)
			}
		}
	}
	// tool calls
	truncated := false
	if chunk.Message != nil && len(chunk.Message.ToolCalls) > 0 {
		var unique []OllamaToolCall
		unique, truncated = dedupeOllamaToolCalls(chunk.Message.ToolCalls, toolCallIndex, seenToolCalls)
		delta.Choices[0].Delta.ToolCalls, toolCallIndex = ollamaToolCallsToOpenAI(unique, toolCallIndex, true)
	}
	return delta, toolCallIndex, truncated
}

func ollamaStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("empty response"), types.ErrorCodeBadResponse, http.StatusBadRequest)
	}
	defer service.CloseResponseBodyGracefully(resp)

	helper.SetEventStreamHeaders(c)
	scanner := helper.NewStreamScanner(resp.Body)
	usage := &dto.Usage{}
	var model = info.UpstreamModelName
	var responseId = common.GetUUID()
	var created = time.Now().Unix()
	var toolCallIndex int
	seenToolCalls := make(map[string]struct{})
	start := helper.GenerateStartEmptyResponse(responseId, created, model, nil)
	if data, err := common.Marshal(start); err == nil {
		_ = helper.StringData(c, string(data))
	}

	truncated := false
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var chunk ollamaChatStreamChunk
		if err := common.Unmarshal([]byte(line), &chunk); err != nil {
			logger.LogError(c, "ollama stream json decode error: "+err.Error()+" line="+line)
			return usage, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if chunk.Model != "" {
			model = chunk.Model
		}
		created = toUnix(chunk.CreatedAt)

		if !chunk.Done {
			if truncated {
				// Drain remaining frames so the done frame still provides usage
				// for billing; nothing further is forwarded to the client.
				continue
			}
			delta, nextToolCallIndex, limitHit := buildOllamaChatStreamDelta(chunk, model, responseId, created, toolCallIndex, seenToolCalls)
			toolCallIndex = nextToolCallIndex
			if data, err := common.Marshal(delta); err == nil {
				_ = helper.StringData(c, string(data))
			}
			if limitHit {
				truncated = true
				logger.LogWarn(c, fmt.Sprintf("ollama response contains too many tool calls (limit %d); stream truncated", maxOllamaToolCalls))
				if stop := helper.GenerateStopResponse(responseId, created, model, constant.FinishReasonToolCalls); stop != nil {
					if data, err := common.Marshal(stop); err == nil {
						_ = helper.StringData(c, string(data))
					}
				}
				if final := helper.GenerateFinalUsageResponse(responseId, created, model, *usage); final != nil {
					if data, err := common.Marshal(final); err == nil {
						_ = helper.StringData(c, string(data))
					}
				}
				helper.Done(c)
			}
			continue
		}
		// done frame
		// finalize once and break loop
		usage.PromptTokens = chunk.PromptEvalCount
		usage.CompletionTokens = chunk.EvalCount
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
		if truncated {
			break
		}
		finishReason := chunk.DoneReason
		if finishReason == "" {
			finishReason = "stop"
		}
		if finishReason == "stop" && toolCallIndex > 0 {
			finishReason = constant.FinishReasonToolCalls
		}
		// emit stop delta
		if stop := helper.GenerateStopResponse(responseId, created, model, finishReason); stop != nil {
			if data, err := common.Marshal(stop); err == nil {
				_ = helper.StringData(c, string(data))
			}
		}
		// emit usage frame
		if final := helper.GenerateFinalUsageResponse(responseId, created, model, *usage); final != nil {
			if data, err := common.Marshal(final); err == nil {
				_ = helper.StringData(c, string(data))
			}
		}
		// send [DONE]
		helper.Done(c)
		break
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		logger.LogError(c, "ollama stream scan error: "+err.Error())
	}
	return usage, nil
}

// non-stream handler for chat/generate
func ollamaChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	if common.DebugEnabled {
		println("ollama non-stream raw resp:", string(body))
	}

	full, usage, truncated, err := parseOllamaChatResponse(body, info)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if truncated {
		logger.LogWarn(c, fmt.Sprintf("ollama response contains too many tool calls (limit %d); response truncated", maxOllamaToolCalls))
	}
	out, _ := common.Marshal(full)
	service.IOCopyBytesGracefully(c, resp, out)
	return usage, nil
}

// parseOllamaChatResponse aggregates an ollama chat/generate response body —
// either a single JSON object or newline-delimited stream frames — into the
// equivalent OpenAI chat-completions response. The returned truncated flag
// reports that the tool-call limit cut the aggregated tool calls short.
func parseOllamaChatResponse(body []byte, info *relaycommon.RelayInfo) (*dto.OpenAITextResponse, *dto.Usage, bool, error) {
	lines := strings.Split(string(body), "\n")
	var (
		aggContent       strings.Builder
		reasoningBuilder strings.Builder
		lastChunk        ollamaChatStreamChunk
		parsedAny        bool
		toolCallIndex    int
		toolCalls        []dto.ToolCallResponse
		truncated        bool
		seenToolCalls    = make(map[string]struct{})
	)
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var ck ollamaChatStreamChunk
		if err := common.Unmarshal([]byte(ln), &ck); err != nil {
			if len(lines) == 1 {
				return nil, nil, false, err
			}
			continue
		}
		parsedAny = true
		lastChunk = ck
		if ck.Message != nil && len(ck.Message.Thinking) > 0 {
			raw := strings.TrimSpace(string(ck.Message.Thinking))
			if raw != "" && raw != "null" {
				// Unmarshal the JSON string to get the actual content without quotes
				var thinkingContent string
				if err := common.Unmarshal(ck.Message.Thinking, &thinkingContent); err == nil {
					reasoningBuilder.WriteString(thinkingContent)
				} else {
					// Fallback to raw string if it's not a JSON string
					reasoningBuilder.WriteString(raw)
				}
			}
		}
		if ck.Message != nil && ck.Message.Content != "" {
			aggContent.WriteString(ck.Message.Content)
		} else if ck.Response != "" {
			aggContent.WriteString(ck.Response)
		}
		if ck.Message != nil && len(ck.Message.ToolCalls) > 0 {
			var unique []OllamaToolCall
			unique, truncated = dedupeOllamaToolCalls(ck.Message.ToolCalls, toolCallIndex, seenToolCalls)
			var converted []dto.ToolCallResponse
			converted, toolCallIndex = ollamaToolCallsToOpenAI(unique, toolCallIndex, false)
			toolCalls = append(toolCalls, converted...)
		}
	}

	if !parsedAny {
		var single ollamaChatStreamChunk
		if err := common.Unmarshal(body, &single); err != nil {
			return nil, nil, false, err
		}
		lastChunk = single
		if single.Message != nil {
			if len(single.Message.Thinking) > 0 {
				raw := strings.TrimSpace(string(single.Message.Thinking))
				if raw != "" && raw != "null" {
					// Unmarshal the JSON string to get the actual content without quotes
					var thinkingContent string
					if err := common.Unmarshal(single.Message.Thinking, &thinkingContent); err == nil {
						reasoningBuilder.WriteString(thinkingContent)
					} else {
						// Fallback to raw string if it's not a JSON string
						reasoningBuilder.WriteString(raw)
					}
				}
			}
			aggContent.WriteString(single.Message.Content)
			if len(single.Message.ToolCalls) > 0 {
				var unique []OllamaToolCall
				unique, truncated = dedupeOllamaToolCalls(single.Message.ToolCalls, toolCallIndex, seenToolCalls)
				var converted []dto.ToolCallResponse
				converted, toolCallIndex = ollamaToolCallsToOpenAI(unique, toolCallIndex, false)
				toolCalls = append(toolCalls, converted...)
			}
		} else {
			aggContent.WriteString(single.Response)
		}
	}

	model := lastChunk.Model
	if model == "" {
		model = info.UpstreamModelName
	}
	created := toUnix(lastChunk.CreatedAt)
	usage := &dto.Usage{PromptTokens: lastChunk.PromptEvalCount, CompletionTokens: lastChunk.EvalCount, TotalTokens: lastChunk.PromptEvalCount + lastChunk.EvalCount}
	content := aggContent.String()
	finishReason := lastChunk.DoneReason
	if finishReason == "" {
		finishReason = "stop"
	}
	if finishReason == "stop" && len(toolCalls) > 0 {
		finishReason = constant.FinishReasonToolCalls
	}

	msg := dto.Message{Role: "assistant", Content: contentPtr(content)}
	if len(toolCalls) > 0 {
		if rawToolCalls, err := common.Marshal(toolCalls); err == nil {
			msg.ToolCalls = rawToolCalls
		}
	}
	if rc := reasoningBuilder.String(); rc != "" {
		msg.ReasoningContent = &rc
	}
	full := dto.OpenAITextResponse{
		Id:      common.GetUUID(),
		Model:   model,
		Object:  "chat.completion",
		Created: created,
		Choices: []dto.OpenAITextResponseChoice{{
			Index:        0,
			Message:      msg,
			FinishReason: finishReason,
		}},
		Usage: *usage,
	}
	return &full, usage, truncated, nil
}

func contentPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
