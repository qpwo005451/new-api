#!/usr/bin/env python3
"""Patch responses_handler.go to filter image_generation tools from requests."""

FILE = "/opt/new-api/relay/responses_handler.go"

with open(FILE, "r") as f:
    content = f.read()

# Add encoding/json import if missing
if '"encoding/json"' not in content:
    content = content.replace(
        '"github.com/QuantumNous/new-api/common"',
        '"encoding/json"\n\t"github.com/QuantumNous/new-api/common"'
    )

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

# Insert the function after the import block
for marker in ["\nfunc ", "\ntype ", "\nvar "]:
    idx = content.find(marker)
    if idx != -1:
        content = content[:idx] + "\n" + FILTER_FUNC + content[idx:]
        break

# Add filter call before RemoveDisabledFields
old = "\t\tjsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)"
new = "\t\tjsonData = filterImageGenerationTool(jsonData)\n" + old
if old in content:
    content = content.replace(old, new)
    print("Added filterImageGenerationTool call")
else:
    print("WARNING: RemoveDisabledFields line not found!")

with open(FILE, "w") as f:
    f.write(content)

print("Patched successfully")
