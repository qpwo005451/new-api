package common

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	appcommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

const responsesDiagnosticsContextKey = "responses_diagnostics"

type ResponsesDiagnostics struct {
	Request *ResponsesRequestDiagnostics `json:"request,omitempty"`
	Stream  *ResponsesStreamDiagnostics  `json:"stream,omitempty"`
}

type ResponsesRequestDiagnostics struct {
	InputJSONType                  string          `json:"input_json_type,omitempty"`
	InputBytes                     int             `json:"input_bytes,omitempty"`
	InputSHA256                    string          `json:"input_sha256,omitempty"`
	InputItemCount                 int             `json:"input_item_count,omitempty"`
	InputItemTypes                 []string        `json:"input_item_types,omitempty"`
	PreviousResponseIDPresent      bool            `json:"previous_response_id_present"`
	PreviousResponseIDLength       int             `json:"previous_response_id_length,omitempty"`
	PreviousResponseIDSHA256       string          `json:"previous_response_id_sha256,omitempty"`
	EncryptedContentCount          int             `json:"encrypted_content_count,omitempty"`
	EncryptedContentBytes          int             `json:"encrypted_content_bytes,omitempty"`
	EncryptedContentSHA256         []string        `json:"encrypted_content_sha256,omitempty"`
	FunctionCallOutputCount        int             `json:"function_call_output_count,omitempty"`
	FunctionOutputPresentCount     int             `json:"function_output_present_count,omitempty"`
	FunctionOutputStringCount      int             `json:"function_output_string_count,omitempty"`
	FunctionOutputBytes            int             `json:"function_output_bytes,omitempty"`
	FunctionOutputJSONTypes        map[string]int  `json:"function_output_json_types,omitempty"`
	TypeCounts                     map[string]int  `json:"type_counts,omitempty"`
	SensitiveContentCapturePolicy  string          `json:"sensitive_content_capture_policy"`
	TopLevelFieldPresence          map[string]bool `json:"top_level_field_presence,omitempty"`
	TopLevelRawFieldByteLengths    map[string]int  `json:"top_level_raw_field_byte_lengths,omitempty"`
	EncryptedContentContainerTypes map[string]int  `json:"encrypted_content_container_types,omitempty"`
}

type ResponsesStreamDiagnostics struct {
	EventCount                    int                              `json:"event_count,omitempty"`
	FirstEvents                   []ResponsesStreamEventDiagnostic `json:"first_events,omitempty"`
	ErrorEventCount               int                              `json:"error_event_count,omitempty"`
	ResponseFailedEventCount      int                              `json:"response_failed_event_count,omitempty"`
	TerminalEventSeen             bool                             `json:"terminal_event_seen"`
	SensitiveContentCapturePolicy string                           `json:"sensitive_content_capture_policy"`
}

type ResponsesStreamEventDiagnostic struct {
	ChannelID   int    `json:"channel_id,omitempty"`
	Type        string `json:"type,omitempty"`
	OutputIndex *int   `json:"output_index,omitempty"`
	ItemType    string `json:"item_type,omitempty"`
	ItemIDHash  string `json:"item_id_sha256,omitempty"`
	HasResponse bool   `json:"has_response,omitempty"`
	ErrorCode   string `json:"error_code,omitempty"`
	MessageLen  int    `json:"message_length,omitempty"`
}

func CaptureResponsesRequestDiagnostics(c *gin.Context, req *dto.OpenAIResponsesRequest) {
	if c == nil || req == nil {
		return
	}
	diag := getOrCreateResponsesDiagnostics(c)
	diag.Request = summarizeResponsesRequest(req)
}

func RecordResponsesStreamDiagnostic(c *gin.Context, info *RelayInfo, event dto.ResponsesStreamResponse) {
	if c == nil {
		return
	}
	diag := getOrCreateResponsesDiagnostics(c)
	if diag.Stream == nil {
		diag.Stream = &ResponsesStreamDiagnostics{
			SensitiveContentCapturePolicy: "metadata_only_no_prompt_no_tool_output_no_encrypted_content",
		}
	}
	stream := diag.Stream
	stream.EventCount++
	switch event.Type {
	case "error":
		stream.ErrorEventCount++
	case "response.failed", "response.error":
		stream.ResponseFailedEventCount++
	case "response.completed", "response.done", "response.incomplete":
		stream.TerminalEventSeen = true
	}
	if len(stream.FirstEvents) >= 8 {
		return
	}
	itemType := ""
	itemIDHash := ""
	if event.Item != nil {
		itemType = event.Item.Type
		itemIDHash = hashString(event.Item.ID)
	}
	channelID := 0
	if info != nil && info.ChannelMeta != nil {
		channelID = info.ChannelId
	}
	errorCode := ""
	if event.Code != nil {
		errorCode = fmt.Sprintf("%v", event.Code)
	}
	stream.FirstEvents = append(stream.FirstEvents, ResponsesStreamEventDiagnostic{
		ChannelID:   channelID,
		Type:        event.Type,
		OutputIndex: event.OutputIndex,
		ItemType:    itemType,
		ItemIDHash:  itemIDHash,
		HasResponse: event.Response != nil,
		ErrorCode:   errorCode,
		MessageLen:  len(event.Message),
	})
}

func AppendResponsesDiagnosticsAdminInfo(c *gin.Context, err *types.NewAPIError, adminInfo map[string]interface{}) {
	if c == nil || err == nil || adminInfo == nil || !isEncryptedContentError(err) {
		return
	}
	value, exists := c.Get(responsesDiagnosticsContextKey)
	if !exists {
		return
	}
	diag, ok := value.(*ResponsesDiagnostics)
	if !ok || diag == nil {
		return
	}
	adminInfo["responses_diagnostics"] = diag
}

func getOrCreateResponsesDiagnostics(c *gin.Context) *ResponsesDiagnostics {
	if value, exists := c.Get(responsesDiagnosticsContextKey); exists {
		if diag, ok := value.(*ResponsesDiagnostics); ok && diag != nil {
			return diag
		}
	}
	diag := &ResponsesDiagnostics{}
	c.Set(responsesDiagnosticsContextKey, diag)
	return diag
}

func summarizeResponsesRequest(req *dto.OpenAIResponsesRequest) *ResponsesRequestDiagnostics {
	diag := &ResponsesRequestDiagnostics{
		InputJSONType:                 appcommon.GetJsonType(req.Input),
		InputBytes:                    len(req.Input),
		InputSHA256:                   hashBytes(req.Input),
		PreviousResponseIDPresent:     strings.TrimSpace(req.PreviousResponseID) != "",
		PreviousResponseIDLength:      len(req.PreviousResponseID),
		PreviousResponseIDSHA256:      hashString(req.PreviousResponseID),
		TypeCounts:                    map[string]int{},
		FunctionOutputJSONTypes:       map[string]int{},
		SensitiveContentCapturePolicy: "metadata_only_no_prompt_no_tool_output_no_encrypted_content",
		TopLevelFieldPresence: map[string]bool{
			"conversation":         len(req.Conversation) > 0,
			"instructions":         len(req.Instructions) > 0,
			"metadata":             len(req.Metadata) > 0,
			"previous_response_id": strings.TrimSpace(req.PreviousResponseID) != "",
			"prompt":               len(req.Prompt) > 0,
			"tools":                len(req.Tools) > 0,
		},
		TopLevelRawFieldByteLengths: map[string]int{
			"conversation": len(req.Conversation),
			"instructions": len(req.Instructions),
			"metadata":     len(req.Metadata),
			"prompt":       len(req.Prompt),
			"tools":        len(req.Tools),
		},
		EncryptedContentContainerTypes: map[string]int{},
	}
	if len(req.Input) == 0 {
		return diag
	}
	var input any
	if err := appcommon.Unmarshal(req.Input, &input); err != nil {
		return diag
	}
	items, ok := input.([]any)
	if !ok {
		summarizeResponsesInputValue(input, diag, "")
		return diag
	}
	diag.InputItemCount = len(items)
	for _, item := range items {
		itemType := responsesItemType(item)
		if itemType == "" {
			itemType = appcommon.GetJsonType(mustMarshal(item))
		}
		diag.InputItemTypes = append(diag.InputItemTypes, itemType)
		if itemType != "" {
			diag.TypeCounts[itemType]++
		}
		if itemType == "function_call_output" {
			diag.FunctionCallOutputCount++
			if output, ok := mapValue(item, "output"); ok {
				outputBytes := mustMarshal(output)
				outputType := appcommon.GetJsonType(outputBytes)
				diag.FunctionOutputPresentCount++
				diag.FunctionOutputBytes += len(outputBytes)
				diag.FunctionOutputJSONTypes[outputType]++
				if _, ok := output.(string); ok {
					diag.FunctionOutputStringCount++
				}
			}
		}
		summarizeResponsesInputValue(item, diag, itemType)
	}
	return diag
}

func summarizeResponsesInputValue(value any, diag *ResponsesRequestDiagnostics, containerType string) {
	switch typed := value.(type) {
	case map[string]any:
		if itemType := stringValue(typed["type"]); itemType != "" {
			containerType = itemType
		}
		for key, child := range typed {
			if key == "encrypted_content" {
				contentBytes := len(mustMarshal(child))
				diag.EncryptedContentCount++
				diag.EncryptedContentBytes += contentBytes
				if len(diag.EncryptedContentSHA256) < 8 {
					diag.EncryptedContentSHA256 = append(diag.EncryptedContentSHA256, hashBytes(mustMarshal(child)))
				}
				if containerType == "" {
					containerType = "unknown"
				}
				diag.EncryptedContentContainerTypes[containerType]++
				continue
			}
			summarizeResponsesInputValue(child, diag, containerType)
		}
	case []any:
		for _, child := range typed {
			summarizeResponsesInputValue(child, diag, containerType)
		}
	}
}

func responsesItemType(value any) string {
	if typed, ok := value.(map[string]any); ok {
		return stringValue(typed["type"])
	}
	return ""
}

func mapValue(value any, key string) (any, bool) {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	child, exists := typed[key]
	return child, exists
}

func stringValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

func isEncryptedContentError(err *types.NewAPIError) bool {
	code := strings.ToLower(string(err.GetErrorCode()))
	message := strings.ToLower(err.Error())
	return code == "invalid_encrypted_content" ||
		strings.Contains(message, "encrypted function output content could not be decrypted or decoded") ||
		strings.Contains(message, "encrypted_content")
}

func hashString(value string) string {
	if value == "" {
		return ""
	}
	return hashBytes([]byte(value))
}

func hashBytes(value []byte) string {
	if len(value) == 0 {
		return ""
	}
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func mustMarshal(value any) []byte {
	data, err := appcommon.Marshal(value)
	if err != nil {
		return nil
	}
	return data
}
