#!/usr/bin/env python3
"""Patch responses_handler.go to filter image_generation tools from requests."""

import sys

FILE = "/opt/new-api/relay/responses_handler.go"
IMPORT_ANCHOR = '"github.com/QuantumNous/new-api/common"'
IMPORT_MARKER = '"encoding/json"'
FUNCTION_MARKER = "func filterImageGenerationTool("
FUNCTION_INSERTION_MARKERS = ["\nfunc ", "\ntype ", "\nvar "]
CALL_MARKER = "filterImageGenerationTool(jsonData)"
REMOVE_DISABLED_FIELDS_CALL = (
    "\t\tjsonData, err = relaycommon.RemoveDisabledFields("
    "jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)"
)
CALL_INJECTION = "\t\tjsonData = filterImageGenerationTool(jsonData)\n" + REMOVE_DISABLED_FIELDS_CALL


def fail(message: str) -> None:
    print(f"ERROR: {message}", file=sys.stderr)
    raise SystemExit(1)


def find_function_insertion_index(content: str) -> int:
    for marker in FUNCTION_INSERTION_MARKERS:
        idx = content.find(marker)
        if idx != -1:
            return idx
    return -1

with open(FILE, "r") as f:
    content = f.read()

needs_import = IMPORT_MARKER not in content
needs_function = FUNCTION_MARKER not in content
needs_call = CALL_MARKER not in content

if not needs_import and not needs_function and not needs_call:
    print("No changes needed")
    raise SystemExit(0)

if needs_import and IMPORT_ANCHOR not in content:
    fail(f"cannot insert {IMPORT_MARKER}: missing import anchor {IMPORT_ANCHOR!r}")

if needs_function and find_function_insertion_index(content) == -1:
    fail("cannot insert filterImageGenerationTool: no function/type/var marker found")

if needs_call and REMOVE_DISABLED_FIELDS_CALL not in content:
    fail("cannot insert filterImageGenerationTool call: RemoveDisabledFields anchor not found")

# Filter function to add
FILTER_FUNC = '''
// filterImageGenerationTool removes image_generation tools from the tools array
// in the request body to avoid 403 errors from upstream providers.
func filterImageGenerationTool(jsonData []byte) []byte {
	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return jsonData
	}
	toolsRaw, ok := data["tools"]
	if !ok {
		return jsonData
	}
	tools, ok := toolsRaw.([]interface{})
	if !ok {
		return jsonData
	}
	filtered := make([]interface{}, 0, len(tools))
	for _, tool := range tools {
		toolMap, ok := tool.(map[string]interface{})
		if !ok {
			filtered = append(filtered, tool)
			continue
		}
		if toolType, exists := toolMap["type"]; exists {
			if typeStr, ok := toolType.(string); ok && typeStr == "image_generation" {
				continue // skip image_generation tool
			}
		}
		filtered = append(filtered, tool)
	}
	if len(filtered) == len(tools) {
		return jsonData // no change
	}
	data["tools"] = filtered
	result, err := json.Marshal(data)
	if err != nil {
		return jsonData
	}
	return result
}
'''

updated = content
changed = False

if needs_import:
    updated = updated.replace(
        IMPORT_ANCHOR,
        '"encoding/json"\n\t"github.com/QuantumNous/new-api/common"',
        1,
    )
    changed = True
    print("Added encoding/json import")

if needs_function:
    idx = find_function_insertion_index(updated)
    if idx == -1:
        fail("cannot insert filterImageGenerationTool after import update: no function/type/var marker found")
    updated = updated[:idx] + "\n" + FILTER_FUNC + updated[idx:]
    changed = True
    print("Added filterImageGenerationTool function")

if needs_call:
    updated = updated.replace(REMOVE_DISABLED_FIELDS_CALL, CALL_INJECTION, 1)
    changed = True
    print("Added filterImageGenerationTool call")

if not changed:
    fail("internal error: patch was required but no changes were applied")

if IMPORT_MARKER not in updated:
    fail(f"patch incomplete: missing {IMPORT_MARKER} after update")

if FUNCTION_MARKER not in updated:
    fail("patch incomplete: missing filterImageGenerationTool after update")

if CALL_MARKER not in updated:
    fail("patch incomplete: missing filterImageGenerationTool call after update")

if changed:
    with open(FILE, "w") as f:
        f.write(updated)
    print("Patched successfully")
