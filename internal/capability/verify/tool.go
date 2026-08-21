package verify

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	ToolName = "jacu_verify"
)

type envelope[T any] struct {
	Status      string   `json:"status"`
	Summary     string   `json:"summary"`
	Data        T        `json:"data"`
	Artifacts   []string `json:"artifacts"`
	Warnings    []string `json:"warnings"`
	NextActions []string `json:"next_actions"`
	TraceID     string   `json:"trace_id"`
}

// Spec limits are runtime policy, not tool parameters. The object being
// governed does not get to choose its own timeout or its own output cap.
const (
	toolTimeout    = 630 * time.Second
	maxOutputBytes = 16 * 1024
)

func RegisterTool(server *mcp.Server, root string) {
	manager, err := NewTaskManager(root)
	if err != nil {
		panic("verify: initialize task manager: " + err.Error())
	}
	RegisterToolWithTaskManager(server, root, manager)
}

func RegisterToolWithTaskManager(server *mcp.Server, root string, manager *TaskManager) {
	if manager == nil {
		panic("verify: task manager is nil")
	}
	registerVerify(server, root, manager)
}

func registerVerify(server *mcp.Server, root string, manager *TaskManager) {
	destructive := false
	openWorld := false
	tool := &mcp.Tool{
		Name:         ToolName,
		Description:  "Run checks.",
		OutputSchema: outputSchema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: &destructive,
			IdempotentHint:  false,
			OpenWorldHint:   &openWorld,
		},
	}
	tool.InputSchema = verifyInputSchema()
	mcp.AddTool(server, tool, func(ctx context.Context, _ *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, envelope[Data], error) {
		result := RunWithManager(ctx, root, input, manager)
		data, _ := result.Data.(Data)
		return nil, envelope[Data]{
			Status:      result.Status,
			Summary:     result.Summary,
			Data:        data,
			Artifacts:   result.Artifacts,
			Warnings:    result.Warnings,
			NextActions: result.NextActions,
			TraceID:     result.TraceID,
		}, nil
	})
}

func verifyInputSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"run_id":  {Type: "string"},
			"argv":    {Type: "array", Items: &jsonschema.Schema{Type: "string"}},
			"async":   {Type: "boolean"},
			"task_id": {Type: "string"},
			"cancel":  {Type: "boolean"},
		},
	}
}

// outputSchema announces the verdict enum. A host that has to branch on the
// verdict should learn the five values from the contract, not by observation —
// and "not run" existing at all is the part nobody guesses.
func outputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[envelope[Data]](nil)
	if err != nil {
		panic("verify: infer output schema: " + err.Error())
	}
	data, ok := schema.Properties["data"]
	if !ok {
		panic("verify: output schema lost data")
	}
	verdict, ok := data.Properties["verdict"]
	if !ok {
		panic("verify: output schema lost verdict")
	}
	verdict.Enum = []any{VerdictPass, VerdictFail, VerdictTimeout, VerdictBlocked, VerdictNotRun}
	// Task metadata is already a versioned sub-contract and is also projected
	// by jacu_status. Keep its nested schema opaque here so the same command
	// evidence is not duplicated in tools/list.
	data.Properties["task"] = &jsonschema.Schema{Types: []string{"null", "object"}}
	return schema
}

// fitOutputCap keeps the answer inside the inline cap by dropping evidence in
// priority order, instead of letting the runtime zero the whole payload on
// overflow. The tails of commands that passed go first — nobody reads the
// output of a test that succeeded — then the tails of the ones that did not.
// argv, status, exit code, duration and the digests are never dropped: they are
// what the verdict and the receipt are made of.
func fitOutputCap(data Data, warnings []string) (Data, []string) {
	if encodedSize(data) <= maxOutputBytes {
		return data, warnings
	}
	dropped := 0
	for index := range data.Commands {
		if data.Commands[index].Status == StatusPassed {
			data.Commands[index].StdoutTail = ""
			data.Commands[index].StderrTail = ""
			data.Commands[index].Truncated = true
			dropped++
		}
	}
	if encodedSize(data) > maxOutputBytes {
		for index := range data.Commands {
			data.Commands[index].StdoutTail = ""
			data.Commands[index].StderrTail = ""
			data.Commands[index].Truncated = true
		}
		dropped = len(data.Commands)
	}
	if dropped > 0 {
		warnings = append(warnings,
			"output tails dropped to fit the inline cap; the evidence digest still covers the full output")
	}
	return data, warnings
}

func encodedSize(data Data) int {
	encoded, err := json.Marshal(data)
	if err != nil {
		return maxOutputBytes + 1
	}
	return len(encoded)
}
