package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
	"github.com/jacu-dev/jacu-harness/internal/userstate"
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
	capability := capabilityruntime.Capability{
		ProjectID: telemetry.ProjectID(root),
		Spec: capabilityruntime.ToolSpec{
			Name: SaveToolName, Version: "1", Risk: capabilityruntime.RiskWrite,
			ReadOnly: false, Idempotent: true, OpenWorld: false,
			Timeout: 10 * time.Second, MaxInputBytes: 64 * 1024, MaxOutputBytes: 16 * 1024,
		},
		Handler: saveHandler(root),
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        SaveToolName,
		Description: "Save memory.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &destructive, IdempotentHint: true, OpenWorldHint: &openWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, envelope[SaveResult], error) {
		raw, err := json.Marshal(input)
		if err != nil {
			return nil, envelope[SaveResult]{}, err
		}
		result := capabilityruntime.Execute(ctx, capability, raw)
		data, _ := result.Data.(SaveResult)
		return nil, envelope[SaveResult]{Status: result.Status, Summary: result.Summary, Data: data,
			Artifacts: nonNilStrings(result.Artifacts), Warnings: nonNilStrings(result.Warnings), NextActions: nonNilStrings(result.NextActions), TraceID: result.TraceID}, nil
	})
}

func registerRecallTool(server *mcp.Server, root string) {
	destructive, openWorld := false, false
	capability := capabilityruntime.Capability{
		ProjectID: telemetry.ProjectID(root),
		Spec: capabilityruntime.ToolSpec{
			Name: RecallToolName, Version: "1", Risk: capabilityruntime.RiskSafe,
			ReadOnly: true, Idempotent: true, OpenWorld: false,
			Timeout: 10 * time.Second, MaxInputBytes: 16 * 1024, MaxOutputBytes: maxRecallOutputBytes,
		},
		Handler: recallHandler(),
	}
	mcp.AddTool(server, &mcp.Tool{
		Name:        RecallToolName,
		Description: "Recall memory.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &destructive, IdempotentHint: true, OpenWorldHint: &openWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input RecallInput) (*mcp.CallToolResult, envelope[RecallResult], error) {
		raw, err := json.Marshal(input)
		if err != nil {
			return nil, envelope[RecallResult]{}, err
		}
		result := capabilityruntime.Execute(ctx, capability, raw)
		data, _ := result.Data.(RecallResult)
		return nil, envelope[RecallResult]{Status: result.Status, Summary: result.Summary, Data: data,
			Artifacts: nonNilStrings(result.Artifacts), Warnings: nonNilStrings(result.Warnings), NextActions: nonNilStrings(result.NextActions), TraceID: result.TraceID}, nil
	})
}

func saveHandler(root string) capabilityruntime.Handler {
	return func(_ context.Context, raw json.RawMessage) (capabilityruntime.Result, error) {
		var input Input
		if err := json.Unmarshal(raw, &input); err != nil {
			return capabilityruntime.Result{}, err
		}
		normalized := normalize(input)
		lints := lint(root, normalized)
		data := SaveResult{Lints: nonNilLints(lints)}
		for _, item := range lints {
			if item.Level == "BLOCK" {
				return capabilityruntime.Result{Status: "blocked", Summary: "Memory save blocked by lint.", Data: data, Artifacts: []string{}, Warnings: []string{}, NextActions: []string{}}, nil
			}
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		store := NewFileStore(memoryHome())
		rec := Record{MemoryID: memoryID(normalized), ProjectID: normalized.ProjectID, Kind: normalized.Kind,
			Title: normalized.Title, Body: normalized.Body, Evidence: normalized.Evidence, Source: normalized.Source,
			Status: "active", SupersededBy: "", CreatedAt: now, UpdatedAt: now}
		if existing, ok := store.Get(rec.MemoryID); ok {
			rec.CreatedAt = existing.CreatedAt
		}
		if err := store.Save(rec, normalized.Supersedes); err != nil {
			return capabilityruntime.Result{}, fmt.Errorf("save memory: %w", err)
		}
		data.MemoryID, data.Record = rec.MemoryID, rec
		warnings := []string{}
		if normalized.ProjectID != "" && (normalized.Kind == "convention" || normalized.Supersedes != "") {
			if _, err := SyncProjectAgents(store, normalized.ProjectID, filepath.Join(root, "AGENTS.md")); err != nil {
				warnings = append(warnings, "AGENTS.md bridge not updated: "+err.Error())
			}
		}
		return capabilityruntime.Result{Status: "ok", Summary: "Memory saved.", Data: data, Artifacts: []string{}, Warnings: warnings, NextActions: []string{}}, nil
	}
}

func recallHandler() capabilityruntime.Handler {
	return func(_ context.Context, raw json.RawMessage) (capabilityruntime.Result, error) {
		var input RecallInput
		if err := json.Unmarshal(raw, &input); err != nil {
			return capabilityruntime.Result{}, err
		}
		q := SearchQuery(input)
		if q.K <= 0 {
			q.K = defaultRecallK
		}
		results := NewFileStore(memoryHome()).Search(q)
		result := capabilityruntime.Result{Status: "ok", Summary: "Memory recall completed.", Data: RecallResult{Results: nonNilScored(results)}, Artifacts: []string{}, Warnings: []string{}, NextActions: []string{}}
		if err := fitRecallResultOutput(&result, maxRecallOutputBytes); err != nil {
			return capabilityruntime.Result{}, err
		}
		return result, nil
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

func memoryHome() string {
	return userstate.DirOrLocal()
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
