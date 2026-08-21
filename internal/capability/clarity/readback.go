package clarity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	CodeProse         = "prose_readback"
	CodeUnknownField  = "unknown_field"
	CodeMalformed     = "malformed_readback"
	CodeSpecGrew      = "spec_bytes_delta"
	CodeVariance      = "variance_runs"
	CodeDivergence    = "divergence"
	FieldWriteScope   = "write_scope"
	FieldForbidden    = "forbidden_paths"
	FieldRequirements = "requirements"
	FieldOutOfScope   = "out_of_scope"
	FieldTasks        = "tasks"
)

var identifierField = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// Readback is the only accepted probe answer. Unknown JSON keys are refused.
type Readback struct {
	WriteScope     []string `json:"write_scope"`
	ForbiddenPaths []string `json:"forbidden_paths"`
	Requirements   []string `json:"requirements"`
	OutOfScope     []string `json:"out_of_scope"`
	Tasks          []string `json:"tasks"`
}

// Error is a typed finding. It never carries the rejected payload.
type Error struct {
	Code  string `json:"code"`
	Field string `json:"field,omitempty"`
	Path  string `json:"path,omitempty"`
}

func (err Error) Error() string {
	if err.Field != "" && err.Path != "" {
		return err.Code + ":" + err.Field + ":" + err.Path
	}
	if err.Field != "" {
		return err.Code + ":" + err.Field
	}
	return err.Code
}

func Ingest(raw []byte) (Readback, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return Readback{}, Error{Code: CodeProse}
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var readback Readback
	if err := decoder.Decode(&readback); err != nil {
		if field, ok := unknownJSONField(err); ok {
			return Readback{}, Error{Code: CodeUnknownField, Field: field}
		}
		return Readback{}, Error{Code: CodeMalformed}
	}
	if decoder.More() {
		return Readback{}, Error{Code: CodeMalformed}
	}
	return Normalize(readback), nil
}

func Normalize(readback Readback) Readback {
	return Readback{
		WriteScope:     normalizeList(readback.WriteScope),
		ForbiddenPaths: normalizeList(readback.ForbiddenPaths),
		Requirements:   normalizeList(readback.Requirements),
		OutOfScope:     normalizeList(readback.OutOfScope),
		Tasks:          normalizeList(readback.Tasks),
	}
}

func normalizeList(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
		if value == "" {
			continue
		}
		unique[value] = struct{}{}
	}
	out := make([]string, 0, len(unique))
	for value := range unique {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func unknownJSONField(err error) (string, bool) {
	const prefix = "json: unknown field "
	message := err.Error()
	if !strings.HasPrefix(message, prefix) {
		return "", false
	}
	field := strings.Trim(strings.TrimPrefix(message, prefix), `"`)
	if !identifierField.MatchString(field) {
		return "", true
	}
	return field, true
}

func equalLists(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func Schema() map[string]any {
	stringArray := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{FieldWriteScope, FieldForbidden, FieldRequirements, FieldOutOfScope, FieldTasks},
		"properties": map[string]any{
			FieldWriteScope:   stringArray,
			FieldForbidden:    stringArray,
			FieldRequirements: stringArray,
			FieldOutOfScope:   stringArray,
			FieldTasks:        stringArray,
		},
	}
}

func ProbePrompt() string {
	return fmt.Sprintf("Return JSON only with keys %s, %s, %s, %s, %s. No prose.",
		FieldWriteScope, FieldForbidden, FieldRequirements, FieldOutOfScope, FieldTasks)
}
