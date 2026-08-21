package mcpadapter

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const listToolsMethod = "tools/list"

func compactToolsListMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		result, err := next(ctx, method, req)
		if err != nil || method != listToolsMethod {
			return result, err
		}
		listed, ok := result.(*mcp.ListToolsResult)
		if !ok || listed == nil {
			return result, err
		}
		compacted := *listed
		tools := make([]*mcp.Tool, 0, len(listed.Tools))
		for _, tool := range listed.Tools {
			if tool == nil {
				continue
			}
			cloned := *tool
			if cloned.OutputSchema != nil {
				cloned.OutputSchema = compactOutputSchema(cloned.OutputSchema)
			}
			if cloned.InputSchema != nil {
				cloned.InputSchema = compactInputSchema(cloned.InputSchema)
			}
			tools = append(tools, &cloned)
		}
		compacted.Tools = tools
		return &compacted, nil
	}
}

func compactOutputSchema(schema any) map[string]any {
	data := map[string]any{"type": "object"}
	if extracted := extractDataSchema(schema); extracted != nil {
		compactSchemaTree(extracted)
		data = extracted
	}
	defs := map[string]any{
		"s": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
	}
	if merged, ok := data["$defs"].(map[string]any); ok {
		for key, value := range merged {
			if key == "s" || key == "e" {
				continue
			}
			defs[key] = value
		}
		delete(data, "$defs")
	}
	defs["e"] = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status":       map[string]any{"type": "string"},
			"summary":      map[string]any{"type": "string"},
			"data":         data,
			"artifacts":    map[string]any{"$ref": "#/$defs/s"},
			"warnings":     map[string]any{"$ref": "#/$defs/s"},
			"next_actions": map[string]any{"$ref": "#/$defs/s"},
			"trace_id":     map[string]any{"type": "string"},
		},
	}
	return map[string]any{
		"$defs": defs,
		"$ref":  "#/$defs/e",
	}
}

func compactInputSchema(schema any) map[string]any {
	parsed := asObject(schema)
	if parsed == nil {
		return map[string]any{"type": "object"}
	}
	compactSchemaTree(parsed)
	return parsed
}

func compactSchemaTree(root map[string]any) {
	stripNoise(root)
	collapseNullableTypes(root)
	replaced := replaceStringArrays(root)
	if replaced > 0 {
		attachStringArrayDef(root)
	}
}

func attachStringArrayDef(root map[string]any) {
	defs, _ := root["$defs"].(map[string]any)
	if defs == nil {
		defs = map[string]any{}
		root["$defs"] = defs
	}
	defs["s"] = map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "string"},
	}
}

func extractDataSchema(schema any) map[string]any {
	parsed := asObject(schema)
	if parsed == nil {
		return nil
	}
	properties, _ := parsed["properties"].(map[string]any)
	if properties == nil {
		return nil
	}
	data, _ := properties["data"].(map[string]any)
	return data
}

func asObject(schema any) map[string]any {
	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	var parsed map[string]any
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		return nil
	}
	return parsed
}

func stripNoise(value any) {
	switch typed := value.(type) {
	case map[string]any:
		delete(typed, "additionalProperties")
		delete(typed, "$schema")
		delete(typed, "title")
		delete(typed, "default")
		delete(typed, "examples")
		delete(typed, "description")
		delete(typed, "required")
		for _, nested := range typed {
			stripNoise(nested)
		}
	case []any:
		for _, nested := range typed {
			stripNoise(nested)
		}
	}
}

func collapseNullableTypes(value any) {
	switch typed := value.(type) {
	case map[string]any:
		if collapsed, ok := nonNullType(typed["type"]); ok {
			typed["type"] = collapsed
		}
		for _, nested := range typed {
			collapseNullableTypes(nested)
		}
	case []any:
		for _, nested := range typed {
			collapseNullableTypes(nested)
		}
	}
}

func nonNullType(typeVal any) (string, bool) {
	list, ok := typeVal.([]any)
	if !ok {
		return "", false
	}
	found := ""
	nulls := 0
	for _, item := range list {
		name, ok := item.(string)
		if !ok {
			return "", false
		}
		if name == "null" {
			nulls++
			continue
		}
		if found != "" {
			return "", false
		}
		found = name
	}
	if found == "" || nulls == 0 {
		return "", false
	}
	return found, true
}

func replaceStringArrays(value any) int {
	object, ok := value.(map[string]any)
	if !ok {
		list, isList := value.([]any)
		if !isList {
			return 0
		}
		replaced := 0
		for _, nested := range list {
			replaced += replaceStringArrays(nested)
		}
		return replaced
	}
	if isStringArray(object) {
		for key := range object {
			delete(object, key)
		}
		object["$ref"] = "#/$defs/s"
		return 1
	}
	replaced := 0
	for _, nested := range object {
		replaced += replaceStringArrays(nested)
	}
	return replaced
}

func isStringArray(schema map[string]any) bool {
	if schema["type"] != "array" {
		return false
	}
	items, ok := schema["items"].(map[string]any)
	if !ok {
		return false
	}
	return items["type"] == "string" && len(items) == 1
}
