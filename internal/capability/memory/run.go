package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
	"github.com/jacu-dev/jacu-harness/internal/userstate"
)

func RunSave(ctx context.Context, root string, in Input) capabilityruntime.Result {
	return capabilityruntime.ExecuteInput(ctx, saveCapability(root), in)
}

func RunRecall(ctx context.Context, root string, in RecallInput) capabilityruntime.Result {
	return capabilityruntime.ExecuteInput(ctx, recallCapability(), in)
}

func saveCapability(root string) capabilityruntime.Capability {
	return capabilityruntime.Capability{
		ProjectID: telemetry.ProjectID(root),
		Spec: capabilityruntime.ToolSpec{
			Name: SaveToolName, Version: "1", Risk: capabilityruntime.RiskWrite,
			ReadOnly: false, Idempotent: true, OpenWorld: false,
			Timeout: 10 * time.Second, MaxInputBytes: 64 * 1024, MaxOutputBytes: 16 * 1024,
		},
		Handler: saveHandler(root),
	}
}

func recallCapability() capabilityruntime.Capability {
	return capabilityruntime.Capability{
		Spec: capabilityruntime.ToolSpec{
			Name: RecallToolName, Version: "1", Risk: capabilityruntime.RiskSafe,
			ReadOnly: true, Idempotent: true, OpenWorld: false,
			Timeout: 10 * time.Second, MaxInputBytes: 16 * 1024, MaxOutputBytes: maxRecallOutputBytes,
		},
		Handler: recallHandler(),
	}
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

func memoryHome() string {
	return userstate.DirOrLocal()
}
