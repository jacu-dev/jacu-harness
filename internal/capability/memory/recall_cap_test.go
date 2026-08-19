package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
)

func TestRecallTruncatesLowestScoreResultsBeforeRuntimeOutputCap(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JACU_HOME", home)
	store := NewFileStore(home)
	for index := 0; index < defaultRecallK; index++ {
		body := strings.Repeat(fmt.Sprintf("body-%02d-", index), 260)
		if index < defaultRecallK/2 {
			body = "priority " + body
		}
		record := testRecord(
			fmt.Sprintf("mem_%016x", index+1),
			testProjectID,
			"gotcha",
			"priority record",
			body,
		)
		if err := store.Save(record, ""); err != nil {
			t.Fatalf("Save record %d: %v", index, err)
		}
	}

	input := RecallInput{ProjectID: testProjectID, Query: "priority gotcha"}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	wantAll := store.Search(SearchQuery{ProjectID: input.ProjectID, Query: input.Query, K: defaultRecallK})
	uncapped := capabilityruntime.Result{
		Status: "ok", Summary: "Memory recall completed.", Data: RecallResult{Results: wantAll},
		Artifacts: []string{}, Warnings: []string{}, NextActions: []string{},
	}
	uncapped.TraceID = "tr_0000000000000000"
	uncappedEnvelope, err := json.Marshal(uncapped)
	if err != nil {
		t.Fatalf("marshal uncapped result: %v", err)
	}
	t.Logf("uncapped recall envelope bytes=%d", len(uncappedEnvelope))
	if len(uncappedEnvelope) <= 32*1024 {
		t.Fatalf("fixture envelope = %d bytes; want over 32KB", len(uncappedEnvelope))
	}
	capability := capabilityruntime.Capability{
		Spec: capabilityruntime.ToolSpec{
			Name: RecallToolName, Version: "1", Risk: capabilityruntime.RiskSafe,
			ReadOnly: true, Idempotent: true, OpenWorld: false,
			Timeout: 10 * time.Second, MaxInputBytes: 16 * 1024, MaxOutputBytes: 32 * 1024,
		},
		Handler: recallHandler(),
	}
	result := capabilityruntime.Execute(context.Background(), capability, raw)
	if result.Status != "ok" {
		t.Fatalf("status = %q, data = %#v, warnings = %v; want ok with retained results", result.Status, result.Data, result.Warnings)
	}
	data, ok := result.Data.(RecallResult)
	if !ok {
		t.Fatalf("data type = %T; want RecallResult", result.Data)
	}
	if len(data.Results) == 0 || len(data.Results) >= len(wantAll) {
		t.Fatalf("retained results = %d; want between 1 and %d", len(data.Results), len(wantAll)-1)
	}
	if !reflect.DeepEqual(data.Results, wantAll[:len(data.Results)]) {
		t.Fatalf("retained results are not the highest-score prefix")
	}
	if !containsString(result.Warnings, "recall results truncated to fit 32KB encoded output limit") {
		t.Fatalf("warnings = %v; want recall truncation warning", result.Warnings)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal capped result: %v", err)
	}
	if len(encoded) > 32*1024 {
		t.Fatalf("capped result = %d bytes; want at most 32KB", len(encoded))
	}
}

func containsString(items []string, wanted string) bool {
	for _, item := range items {
		if item == wanted {
			return true
		}
	}
	return false
}
