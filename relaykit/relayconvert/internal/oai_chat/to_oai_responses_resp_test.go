package oaichat

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionsResponseToResponsesPreservesTextToolCallsAndUsage(t *testing.T) {
	chat := &dto.OpenAITextResponse{
		Id:      "chatcmpl_1",
		Model:   "gpt-test",
		Created: 456,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Message:      assistantMessageWithTool("I will call.", "call_1", "lookup", `{"q":"x"}`),
				FinishReason: "tool_calls",
			},
		},
		Usage: dto.Usage{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8},
	}

	resp, usage, err := ChatCompletionsResponseToResponsesResponse(chat, "resp_1")
	require.NoError(t, err)
	require.NotNil(t, usage)

	assert.Equal(t, "resp_1", resp.ID)
	assert.Equal(t, "response", resp.Object)
	assert.Equal(t, `"completed"`, string(resp.Status))
	assert.Equal(t, 3, resp.Usage.InputTokens)
	assert.Equal(t, 5, resp.Usage.OutputTokens)
	require.Len(t, resp.Output, 2)
	assert.Equal(t, responsesOutputTypeMessage, resp.Output[0].Type)
	assert.Equal(t, "I will call.", resp.Output[0].Content[0].Text)
	assert.Equal(t, responsesOutputTypeFunctionCall, resp.Output[1].Type)
	assert.True(t, strings.HasPrefix(resp.Output[1].ID, "fc_"))
	assert.Equal(t, "call_1", resp.Output[1].CallId)
	assert.Equal(t, "lookup", resp.Output[1].Name)
	assert.Equal(t, `"{\"q\":\"x\"}"`, string(resp.Output[1].Arguments))
}

func TestChatCompletionsResponseToResponsesPreservesReasoningTextAlias(t *testing.T) {
	var chat dto.OpenAITextResponse
	err := kitutil.Unmarshal([]byte(`{"id":"chatcmpl_1","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","reasoning_text":"thinking","content":"answer"},"finish_reason":"stop"}]}`), &chat)
	require.NoError(t, err)

	resp, _, err := ChatCompletionsResponseToResponsesResponse(&chat, "resp_1")
	require.NoError(t, err)
	require.Len(t, resp.Output, 2)
	assert.Equal(t, responsesOutputTypeMessage, resp.Output[0].Type)
	assert.Equal(t, responsesOutputTypeReasoning, resp.Output[1].Type)
	require.Len(t, resp.Output[1].Content, 1)
	assert.Equal(t, "thinking", resp.Output[1].Content[0].Text)
}

func TestResponsesFunctionCallItemIDIsStableAndBounded(t *testing.T) {
	callID := "call/with spaces"
	itemID := responsesFunctionCallItemID(callID)

	assert.True(t, strings.HasPrefix(itemID, "fc_"))
	assert.LessOrEqual(t, len(itemID), 64)
	assert.Equal(t, itemID, responsesFunctionCallItemID(callID))
}

func TestChatCompletionsResponseToResponsesPreservesCustomToolItemID(t *testing.T) {
	output, err := chatToolCallToResponsesOutput(dto.ToolCallRequest{
		ID:     "custom_call_1",
		Type:   dto.CustomType,
		Custom: []byte(`{"input":"x"}`),
	}, "resp_1", 0, "completed")
	require.NoError(t, err)

	assert.Equal(t, "custom_call_1", output.ID)
	assert.Equal(t, "custom_call_1", output.CallId)
}

func TestChatCompletionsStreamToResponsesPreservesReasoningTextAlias(t *testing.T) {
	var chunk dto.ChatCompletionsStreamResponse
	err := kitutil.Unmarshal([]byte(`{"choices":[{"index":0,"delta":{"reasoning_text":"thinking"}}]}`), &chunk)
	require.NoError(t, err)

	events := mustResponsesEventsFromChatChunk(t, NewChatToResponsesStreamState("resp_1", "deepseek-v4-flash"), &chunk)
	require.Len(t, events, 3)
	assert.Equal(t, responsesEventCreated, events[0].Type)
	assert.Equal(t, responsesEventOutputItemAdded, events[1].Type)
	assert.Equal(t, responsesOutputTypeReasoning, events[1].Payload.Item.Type)
	assert.Equal(t, responsesEventReasoningSummaryDelta, events[2].Type)
	assert.Equal(t, "thinking", events[2].Payload.Delta)
}

func TestChatCompletionsStreamToResponsesDefersToolUntilCallID(t *testing.T) {
	state := NewChatToResponsesStreamState("resp_1", "gpt-test")
	toolIndex := 0

	first := mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{{
				Index: &toolIndex,
				Function: dto.FunctionResponse{
					Name:      "lookup",
					Arguments: `{"q":"x"}`,
				},
			}}},
		}},
	})
	require.Len(t, first, 1)
	assert.Equal(t, responsesEventCreated, first[0].Type)

	second := mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{{
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{{
				Index: &toolIndex,
				ID:    "call_actual",
			}}},
		}},
	})
	require.Len(t, second, 2)
	require.NotNil(t, second[0].Payload.Item)
	assert.Equal(t, responsesEventOutputItemAdded, second[0].Type)
	assert.Equal(t, "call_actual", second[0].Payload.Item.CallId)
	assert.True(t, strings.HasPrefix(second[0].Payload.Item.ID, "fc_"))
	assert.Equal(t, responsesEventFunctionArgsDelta, second[1].Type)
	assert.Equal(t, `{"q":"x"}`, second[1].Payload.Delta)
}

func TestChatCompletionsResponseToResponsesNormalizesLongFunctionCallID(t *testing.T) {
	longCallID := "call_" + strings.Repeat("x", 80)
	output, err := chatToolCallToResponsesOutput(dto.ToolCallRequest{
		ID:   longCallID,
		Type: "function",
	}, "resp_1", 0, "completed")
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(output.ID, "fc_"))
	assert.True(t, strings.HasPrefix(output.CallId, "call_"))
	assert.LessOrEqual(t, len(output.ID), 64)
	assert.LessOrEqual(t, len(output.CallId), 64)
}

func TestChatCompletionsResponseToResponsesMapsIncompleteFinishReasons(t *testing.T) {
	tests := []struct {
		name         string
		finishReason string
		wantReason   string
	}{
		{name: "length", finishReason: "length", wantReason: responsesIncompleteReasonMaxTokens},
		{name: "content filter", finishReason: "content_filter", wantReason: responsesIncompleteReasonContentFilter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, _, err := ChatCompletionsResponseToResponsesResponse(&dto.OpenAITextResponse{
				Id:    "chatcmpl_1",
				Model: "gpt-test",
				Choices: []dto.OpenAITextResponseChoice{
					{
						Message:      dto.Message{Role: "assistant", Content: "partial"},
						FinishReason: tt.finishReason,
					},
				},
			}, "resp_1")
			require.NoError(t, err)

			assert.Equal(t, `"incomplete"`, string(resp.Status))
			require.NotNil(t, resp.IncompleteDetails)
			assert.Equal(t, tt.wantReason, resp.IncompleteDetails.Reason)
			require.Len(t, resp.Output, 1)
			assert.Equal(t, "incomplete", resp.Output[0].Status)
		})
	}
}

func TestChatCompletionsStreamToResponsesEventsAggregatesUsageAndToolArgs(t *testing.T) {
	state := NewChatToResponsesStreamState("resp_1", "gpt-test")
	state.Created = 123
	toolIndex := 0

	var events []ChatToResponsesStreamEvent
	events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
		Id:      "chatcmpl_1",
		Model:   "gpt-test",
		Created: 123,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{Index: 0, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Role: "assistant"}},
		},
	})...)
	events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{Index: 0, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: lo.ToPtr("hello")}},
		},
	})...)
	events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{Index: 0, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{
				{Index: &toolIndex, ID: "call_1", Type: "function", Function: dto.FunctionResponse{Name: "lookup"}},
			}}},
		},
	})...)
	events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{Index: 0, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{
				{Index: &toolIndex, Function: dto.FunctionResponse{Arguments: `{"q":"x"}`}},
			}}},
		},
	})...)
	finishReason := "tool_calls"
	events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{Index: 0, FinishReason: &finishReason},
		},
	})...)
	events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
		Usage: &dto.Usage{PromptTokens: 2, CompletionTokens: 4, TotalTokens: 6},
	})...)
	events = append(events, FinalizeChatCompletionsStreamToResponses(state)...)

	require.Len(t, events, 10)
	require.NotNil(t, events[3].Payload.Item)
	assert.Equal(t, "fc_call_1", events[3].Payload.ItemID)
	assert.Equal(t, "fc_call_1", events[3].Payload.Item.ID)
	assert.Equal(t, "call_1", events[3].Payload.Item.CallId)
	var functionArgsDone *ChatToResponsesStreamEvent
	for i := range events {
		if events[i].Type == responsesEventFunctionArgsDone {
			functionArgsDone = &events[i]
			break
		}
	}
	require.NotNil(t, functionArgsDone)
	assert.Equal(t, "fc_call_1", functionArgsDone.Payload.ItemID)
	require.NotNil(t, functionArgsDone.Payload.Arguments)
	assert.Equal(t, `{"q":"x"}`, *functionArgsDone.Payload.Arguments)
	assert.Equal(t, responsesEventCreated, events[0].Type)
	assert.Equal(t, responsesEventOutputTextDelta, events[2].Type)
	assert.Equal(t, "hello", events[2].Payload.Delta)
	assert.Equal(t, responsesEventFunctionArgsDelta, events[4].Type)
	assert.Equal(t, `{"q":"x"}`, events[4].Payload.Delta)
	var functionItemDone *ChatToResponsesStreamEvent
	for i := range events {
		if events[i].Type == responsesEventOutputItemDone && events[i].Payload.Item != nil && events[i].Payload.Item.Type == responsesOutputTypeFunctionCall {
			functionItemDone = &events[i]
			break
		}
	}
	require.NotNil(t, functionItemDone)
	assert.Equal(t, "fc_call_1", functionItemDone.Payload.Item.ID)
	assert.Equal(t, "call_1", functionItemDone.Payload.Item.CallId)
	assert.Equal(t, responsesEventCompleted, events[9].Type)
	require.NotNil(t, events[9].Payload.Response)
	assert.Equal(t, 6, events[9].Payload.Response.Usage.TotalTokens)
	require.Len(t, events[9].Payload.Response.Output, 2)
	assert.Equal(t, "hello", events[9].Payload.Response.Output[0].Content[0].Text)
	assert.Equal(t, "fc_call_1", events[9].Payload.Response.Output[1].ID)
	assert.Equal(t, "call_1", events[9].Payload.Response.Output[1].CallId)
	assert.Equal(t, `"{\"q\":\"x\"}"`, string(events[9].Payload.Response.Output[1].Arguments))
}

func mustResponsesEventsFromChatChunk(t *testing.T, state *ChatToResponsesStreamState, chunk *dto.ChatCompletionsStreamResponse) []ChatToResponsesStreamEvent {
	t.Helper()
	events, err := ChatCompletionsStreamChunkToResponsesEvents(chunk, state)
	require.NoError(t, err)
	return events
}
