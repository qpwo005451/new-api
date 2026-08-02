package common

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// RemoveResponsesInputEncryptedContent strips encrypted upstream state from the
// input array before forwarding a Responses request. Custom-provider clients
// may replay ciphertext from an earlier response that a different upstream
// instance cannot decrypt.
func RemoveResponsesInputEncryptedContent(jsonData []byte) ([]byte, bool, error) {
	input := gjson.GetBytes(jsonData, "input")
	if !input.Exists() || !bytes.Contains([]byte(input.Raw), []byte("encrypted_content")) {
		return jsonData, false, nil
	}

	decoder := json.NewDecoder(bytes.NewReader([]byte(input.Raw)))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return jsonData, false, err
	}
	stripped, changed := stripEncryptedContent(value)
	if !changed {
		return jsonData, false, nil
	}
	strippedInput, err := json.Marshal(stripped)
	if err != nil {
		return jsonData, false, err
	}
	result, err := sjson.SetRawBytes(jsonData, "input", strippedInput)
	if err != nil {
		return jsonData, false, err
	}
	return result, true, nil
}

// RemoveResponsesInputItemStatus removes response-only item status fields before
// forwarding a create request to strict Responses-compatible upstreams.
func RemoveResponsesInputItemStatus(jsonData []byte) ([]byte, bool, error) {
	input := gjson.GetBytes(jsonData, "input")
	if !input.IsArray() {
		return jsonData, false, nil
	}

	result := jsonData
	changed := false
	var removeErr error
	index := 0
	input.ForEach(func(_, item gjson.Result) bool {
		currentIndex := index
		index++
		if !item.Get("status").Exists() {
			return true
		}

		next, err := sjson.DeleteBytes(result, "input."+strconv.Itoa(currentIndex)+".status")
		if err != nil {
			removeErr = err
			return false
		}
		result = next
		changed = true
		return true
	})
	if removeErr != nil {
		return jsonData, false, removeErr
	}
	return result, changed, nil
}
