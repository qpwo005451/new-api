package openai

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	grok45ModelName                    = "grok-4.5"
	grok45ShellCommandToolName         = "shell_command"
	maxGrok45BufferedToolArgumentBytes = 1 << 20
)

type grok45ResponsesStreamEvent struct {
	response dto.ResponsesStreamResponse
	data     string
}

type grok45ToolArgumentStreamNormalizer struct {
	timeoutSchema     gjson.Result
	shellCommandIDs   map[string]struct{}
	bufferedDeltas    map[string]*strings.Builder
	passthroughIDs    map[string]struct{}
	normalizedCallIDs map[string]struct{}
}

func grok45ShellCommandTimeoutSchema(info *relaycommon.RelayInfo) (gjson.Result, bool) {
	if info == nil || info.ChannelMeta == nil || !strings.EqualFold(info.UpstreamModelName, grok45ModelName) {
		return gjson.Result{}, false
	}

	request, ok := info.Request.(*dto.OpenAIResponsesRequest)
	if !ok || len(request.Tools) == 0 {
		return gjson.Result{}, false
	}

	for _, tool := range gjson.ParseBytes(request.Tools).Array() {
		if tool.Get("type").String() != "function" || tool.Get("name").String() != grok45ShellCommandToolName {
			continue
		}
		timeoutSchema := tool.Get("parameters.properties.timeout_ms")
		timeoutType := timeoutSchema.Get("type").String()
		// Codex parses this known field as u64 even when an upstream-facing schema declares a generic number.
		if timeoutSchema.Exists() && (timeoutType == "integer" || timeoutType == "number") {
			return timeoutSchema, true
		}
	}

	return gjson.Result{}, false
}

func normalizeGrok45ShellCommandTimeout(arguments []byte, timeoutSchema gjson.Result) ([]byte, bool) {
	timeout := gjson.GetBytes(arguments, "timeout_ms")
	if !timeout.Exists() || timeout.Type != gjson.Number {
		return arguments, false
	}

	normalizedTimeout, ok := normalizeGrok45TimeoutNumber(timeout.Raw)
	if !ok {
		return arguments, false
	}
	timeoutValue, err := strconv.ParseUint(normalizedTimeout, 10, 64)
	if err != nil || !grok45TimeoutMatchesSchema(timeoutValue, timeoutSchema) {
		return arguments, false
	}

	normalizedArguments, err := sjson.SetRawBytes(arguments, "timeout_ms", []byte(normalizedTimeout))
	if err != nil {
		return arguments, false
	}
	return normalizedArguments, true
}

func normalizeGrok45TimeoutNumber(raw string) (string, bool) {
	dotIndex := strings.IndexByte(raw, '.')
	if dotIndex <= 0 || dotIndex == len(raw)-1 || strings.IndexByte(raw[dotIndex+1:], '.') >= 0 {
		return "", false
	}

	wholePart := raw[:dotIndex]
	fractionPart := raw[dotIndex+1:]
	if len(wholePart) > 1 && wholePart[0] == '0' {
		return "", false
	}
	for _, digit := range wholePart {
		if digit < '0' || digit > '9' {
			return "", false
		}
	}
	for _, digit := range fractionPart {
		if digit != '0' {
			return "", false
		}
	}

	value, err := strconv.ParseUint(wholePart, 10, 64)
	if err != nil {
		return "", false
	}
	return strconv.FormatUint(value, 10), true
}

func grok45TimeoutMatchesSchema(value uint64, timeoutSchema gjson.Result) bool {
	if minimum := timeoutSchema.Get("minimum"); minimum.Exists() {
		minimumValue, err := strconv.ParseUint(minimum.Raw, 10, 64)
		if minimum.Type != gjson.Number || err != nil || value < minimumValue {
			return false
		}
	}
	if maximum := timeoutSchema.Get("maximum"); maximum.Exists() {
		maximumValue, err := strconv.ParseUint(maximum.Raw, 10, 64)
		if maximum.Type != gjson.Number || err != nil || value > maximumValue {
			return false
		}
	}
	return true
}

func normalizeGrok45ResponsesToolArguments(responseBody []byte, info *relaycommon.RelayInfo) ([]byte, bool) {
	timeoutSchema, ok := grok45ShellCommandTimeoutSchema(info)
	if !ok {
		return responseBody, false
	}

	result := responseBody
	changed := false
	outputIndex := 0
	gjson.GetBytes(responseBody, "output").ForEach(func(_, item gjson.Result) bool {
		currentIndex := outputIndex
		outputIndex++
		if item.Get("type").String() != "function_call" || item.Get("name").String() != grok45ShellCommandToolName {
			return true
		}

		arguments := item.Get("arguments")
		if !arguments.Exists() || arguments.Type != gjson.String {
			return true
		}
		normalizedArguments, normalized := normalizeGrok45ShellCommandTimeout([]byte(arguments.String()), timeoutSchema)
		if !normalized {
			return true
		}

		next, err := sjson.SetBytes(result, "output."+strconv.Itoa(currentIndex)+".arguments", string(normalizedArguments))
		if err != nil {
			return true
		}
		result = next
		changed = true
		return true
	})

	return result, changed
}

func newGrok45ToolArgumentStreamNormalizer(info *relaycommon.RelayInfo) *grok45ToolArgumentStreamNormalizer {
	timeoutSchema, ok := grok45ShellCommandTimeoutSchema(info)
	if !ok {
		return nil
	}
	return &grok45ToolArgumentStreamNormalizer{
		timeoutSchema:     timeoutSchema,
		shellCommandIDs:   make(map[string]struct{}),
		bufferedDeltas:    make(map[string]*strings.Builder),
		passthroughIDs:    make(map[string]struct{}),
		normalizedCallIDs: make(map[string]struct{}),
	}
}

func (n *grok45ToolArgumentStreamNormalizer) transform(streamResponse dto.ResponsesStreamResponse, data string) ([]grok45ResponsesStreamEvent, bool, error) {
	original := grok45ResponsesStreamEvent{response: streamResponse, data: data}
	if n == nil {
		return []grok45ResponsesStreamEvent{original}, false, nil
	}

	itemID := responsesStreamItemID(streamResponse)
	switch streamResponse.Type {
	case dto.ResponsesOutputTypeItemAdded:
		if itemID != "" && streamResponse.Item != nil &&
			streamResponse.Item.Type == "function_call" &&
			streamResponse.Item.Name == grok45ShellCommandToolName {
			n.shellCommandIDs[itemID] = struct{}{}
		}
		return []grok45ResponsesStreamEvent{original}, false, nil
	case "response.function_call_arguments.delta":
		if _, ok := n.shellCommandIDs[itemID]; !ok {
			return []grok45ResponsesStreamEvent{original}, false, nil
		}
		if _, passthrough := n.passthroughIDs[itemID]; passthrough {
			return []grok45ResponsesStreamEvent{original}, false, nil
		}

		// Codex can parse argument deltas before item completion, so normalize only after the full JSON object is available.
		buffer := n.bufferedDeltas[itemID]
		if buffer == nil {
			buffer = &strings.Builder{}
			n.bufferedDeltas[itemID] = buffer
		}
		if buffer.Len()+len(streamResponse.Delta) > maxGrok45BufferedToolArgumentBytes {
			events := make([]grok45ResponsesStreamEvent, 0, 2)
			if buffer.Len() > 0 {
				bufferedEvent, err := grok45ArgumentsDeltaEvent(streamResponse, buffer.String())
				if err != nil {
					return nil, false, err
				}
				events = append(events, bufferedEvent)
			}
			events = append(events, original)
			delete(n.bufferedDeltas, itemID)
			n.passthroughIDs[itemID] = struct{}{}
			return events, false, nil
		}
		buffer.WriteString(streamResponse.Delta)
		return nil, false, nil
	case "response.function_call_arguments.done":
		if _, ok := n.shellCommandIDs[itemID]; !ok {
			return []grok45ResponsesStreamEvent{original}, false, nil
		}
		if _, passthrough := n.passthroughIDs[itemID]; passthrough {
			return []grok45ResponsesStreamEvent{original}, false, nil
		}
		return n.flushBufferedArguments(streamResponse, data, "arguments")
	case dto.ResponsesOutputTypeItemDone:
		if _, ok := n.shellCommandIDs[itemID]; !ok {
			return []grok45ResponsesStreamEvent{original}, false, nil
		}
		if _, passthrough := n.passthroughIDs[itemID]; passthrough {
			n.clear(itemID)
			return []grok45ResponsesStreamEvent{original}, false, nil
		}

		events, normalized, err := n.flushBufferedArguments(streamResponse, data, "item.arguments")
		n.clear(itemID)
		return events, normalized, err
	default:
		return []grok45ResponsesStreamEvent{original}, false, nil
	}
}

func (n *grok45ToolArgumentStreamNormalizer) flushBufferedArguments(streamResponse dto.ResponsesStreamResponse, data, argumentPath string) ([]grok45ResponsesStreamEvent, bool, error) {
	itemID := responsesStreamItemID(streamResponse)
	original := grok45ResponsesStreamEvent{response: streamResponse, data: data}
	buffer, buffered := n.bufferedDeltas[itemID]

	arguments := ""
	if buffered {
		arguments = buffer.String()
	} else if value := gjson.Get(data, argumentPath); value.Exists() && value.Type == gjson.String {
		arguments = value.String()
	}
	if arguments == "" {
		return []grok45ResponsesStreamEvent{original}, false, nil
	}

	normalizedArguments, normalized := normalizeGrok45ShellCommandTimeout([]byte(arguments), n.timeoutSchema)
	events := make([]grok45ResponsesStreamEvent, 0, 2)
	if buffered {
		argumentsDelta, err := grok45ArgumentsDeltaEvent(streamResponse, string(normalizedArguments))
		if err != nil {
			return nil, false, err
		}
		events = append(events, argumentsDelta)
		delete(n.bufferedDeltas, itemID)
	}

	if value := gjson.Get(data, argumentPath); value.Exists() && value.Type == gjson.String && normalized {
		updatedData, err := sjson.SetBytes([]byte(data), argumentPath, string(normalizedArguments))
		if err == nil {
			original.data = string(updatedData)
		}
	}
	events = append(events, original)

	if !normalized {
		return events, false, nil
	}
	if _, alreadyNormalized := n.normalizedCallIDs[itemID]; alreadyNormalized {
		return events, false, nil
	}
	n.normalizedCallIDs[itemID] = struct{}{}
	return events, true, nil
}

func (n *grok45ToolArgumentStreamNormalizer) clear(itemID string) {
	delete(n.shellCommandIDs, itemID)
	delete(n.bufferedDeltas, itemID)
	delete(n.passthroughIDs, itemID)
	delete(n.normalizedCallIDs, itemID)
}

func responsesStreamItemID(streamResponse dto.ResponsesStreamResponse) string {
	if streamResponse.ItemID != "" {
		return streamResponse.ItemID
	}
	if streamResponse.Item != nil {
		return streamResponse.Item.ID
	}
	return ""
}

func grok45ArgumentsDeltaEvent(source dto.ResponsesStreamResponse, arguments string) (grok45ResponsesStreamEvent, error) {
	event := dto.ResponsesStreamResponse{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: source.OutputIndex,
		ItemID:      responsesStreamItemID(source),
		Delta:       arguments,
	}
	data, err := common.Marshal(event)
	if err != nil {
		return grok45ResponsesStreamEvent{}, err
	}
	return grok45ResponsesStreamEvent{response: event, data: string(data)}, nil
}
