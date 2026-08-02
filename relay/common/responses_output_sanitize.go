package common

import (
	"bytes"
	"encoding/json"
)

// StripResponsesEncryptedContent removes every encrypted_content field from a
// Responses API JSON payload (a stream event line or a complete response body).
//
// Some upstreams wrap reasoning detail and agent messages in encrypted_content
// that clients on custom providers (e.g. Codex Desktop pointed at this relay)
// cannot decrypt. The client then aborts the whole stream with
// "Encrypted function output content could not be decrypted or decoded."
// The plaintext counterparts (reasoning summary, input_text items) are
// preserved, so stripping the field keeps the stream usable for any client.
//
// Containers that held only encrypted content (e.g. a content entry of type
// "encrypted_content") are removed entirely.
//
// Returns the input unchanged when the payload contains no encrypted_content
// or is not valid JSON.
func StripResponsesEncryptedContent(jsonData []byte) []byte {
	if !bytes.Contains(jsonData, []byte("encrypted_content")) {
		return jsonData
	}
	var value interface{}
	if err := json.Unmarshal(jsonData, &value); err != nil {
		return jsonData
	}
	stripped, changed := stripEncryptedContent(value)
	if !changed {
		return jsonData
	}
	result, err := json.Marshal(stripped)
	if err != nil {
		return jsonData
	}
	return result
}

// stripEncryptedContent removes encrypted_content keys in place and reports
// whether anything changed. Values reduced to an "encrypted_content" type
// marker (or to nothing) are removed from their parent container.
func stripEncryptedContent(value interface{}) (interface{}, bool) {
	switch v := value.(type) {
	case map[string]interface{}:
		changed := false
		for key, child := range v {
			if key == "encrypted_content" {
				delete(v, key)
				changed = true
				continue
			}
			strippedChild, childChanged := stripEncryptedContent(child)
			if childChanged {
				changed = true
			}
			if strippedChild == nil {
				delete(v, key)
				continue
			}
			v[key] = strippedChild
		}
		// A container reduced to its "encrypted_content" type marker (or to
		// nothing) is an empty shell; let the parent drop it.
		if changed && (len(v) == 0 || (len(v) == 1 && v["type"] == "encrypted_content")) {
			return nil, true
		}
		return v, changed
	case []interface{}:
		changed := false
		filtered := make([]interface{}, 0, len(v))
		for _, child := range v {
			strippedChild, childChanged := stripEncryptedContent(child)
			if childChanged {
				changed = true
			}
			if strippedChild == nil {
				changed = true
				continue
			}
			filtered = append(filtered, strippedChild)
		}
		return filtered, changed
	}
	return value, false
}
