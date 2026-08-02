package common

import (
	"strconv"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

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
