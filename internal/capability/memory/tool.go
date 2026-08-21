package memory

import (
	"context"
	"encoding/json"
	"errors"

	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	SaveToolName                   = "jacu_memory_save"
	RecallToolName                 = "jacu_memory_recall"
	defaultRecallK                 = 20
	maxRecallOutputBytes     int64 = 32 * 1024
	recallOutputLimitWarning       = "recall results truncated to fit 32KB encoded output limit"
	recallTraceIDBudget            = "tr_0000000000000000"
)

var errRecallMetadataExceedsOutputLimit = errors.New("memory recall metadata exceeds 32KB output limit")

type envelope[T any] struct {
	Status      string   `json:"status"`
	Summary     string   `json:"summary"`
	Data        T        `json:"data"`
	Artifacts   []string `json:"artifacts"`
	Warnings    []string `json:"warnings"`
	NextActions []string `json:"next_actions"`
	TraceID     string   `json:"trace_id"`
}

type SaveResult struct {
	MemoryID string `json:"memory_id"`
	Record   Record `json:"record"`
	Lints    []Lint `json:"lints"`
}

type RecallInput struct {
	ProjectID         string   `json:"project_id"`
	Query             string   `json:"query,omitempty"`
	Kinds             []string `json:"kinds,omitempty"`
	IncludeSuperseded bool     `json:"include_superseded,omitempty"`
	K                 int      `json:"k,omitempty"`
}

type RecallResult struct {
	Results []Scored `json:"results"`
}

func RegisterTool(server *mcp.Server, root string) {
	registerSaveTool(server, root)
	registerRecallTool(server, root)
}

func registerSaveTool(server *mcp.Server, root string) {
	destructive, openWorld := false, false
	mcp.AddTool(server, &mcp.Tool{
		Name:        SaveToolName,
		Description: "Save memory.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &destructive, IdempotentHint: true, OpenWorldHint: &openWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, envelope[SaveResult], error) {
		return nil, toEnvelope[SaveResult](RunSave(ctx, root, input)), nil
	})
}

func registerRecallTool(server *mcp.Server, root string) {
	destructive, openWorld := false, false
	mcp.AddTool(server, &mcp.Tool{
		Name:        RecallToolName,
		Description: "Recall memory.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &destructive, IdempotentHint: true, OpenWorldHint: &openWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input RecallInput) (*mcp.CallToolResult, envelope[RecallResult], error) {
		return nil, toEnvelope[RecallResult](RunRecall(ctx, root, input)), nil
	})
}

func toEnvelope[T any](result capabilityruntime.Result) envelope[T] {
	data, _ := result.Data.(T)
	return envelope[T]{
		Status: result.Status, Summary: result.Summary, Data: data,
		Artifacts: nonNilStrings(result.Artifacts), Warnings: nonNilStrings(result.Warnings),
		NextActions: nonNilStrings(result.NextActions), TraceID: result.TraceID,
	}
}

func fitRecallResultOutput(result *capabilityruntime.Result, maxBytes int64) error {
	encodedBytes, err := encodedRecallResultBytes(*result)
	if err != nil {
		return err
	}
	if encodedBytes <= maxBytes {
		return nil
	}

	data, ok := result.Data.(RecallResult)
	if !ok {
		return errRecallMetadataExceedsOutputLimit
	}
	result.Warnings = append(result.Warnings, recallOutputLimitWarning)
	allResults := data.Results
	data.Results = []Scored{}
	result.Data = data
	encodedBytes, err = encodedRecallResultBytes(*result)
	if err != nil {
		return err
	}
	if encodedBytes > maxBytes {
		return errRecallMetadataExceedsOutputLimit
	}

	best := data.Results
	for low, high := 0, len(allResults); low <= high; {
		middle := low + (high-low)/2
		candidate := append([]Scored{}, allResults[:middle]...)
		data.Results = candidate
		result.Data = data
		encodedBytes, err = encodedRecallResultBytes(*result)
		if err != nil {
			return err
		}
		if encodedBytes <= maxBytes {
			best = candidate
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	data.Results = best
	result.Data = data
	return nil
}

func encodedRecallResultBytes(result capabilityruntime.Result) (int64, error) {
	result.TraceID = recallTraceIDBudget
	encoded, err := json.Marshal(result)
	return int64(len(encoded)), err
}

func nonNilLints(items []Lint) []Lint {
	if items == nil {
		return []Lint{}
	}
	return items
}

func nonNilScored(items []Scored) []Scored {
	if items == nil {
		return []Scored{}
	}
	return items
}

func nonNilStrings(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}
