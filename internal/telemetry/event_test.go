package telemetry

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validEventInput() EventInput {
	return EventInput{
		Timestamp: time.Date(2026, time.August, 12, 3, 0, 0, 0, time.UTC),
		ProjectID: "prj_0123456789abcdef",
		TraceID:   "tr_0123456789abcdef",
		RunID:     "run_0123456789abcdef",
		MissionID: "msn_0123456789abcdef",
		Event:     EventToolCall,
		Tool:      "jacu_report",
		Status:    "ok",
		Duration:  42 * time.Millisecond,
	}
}

func TestEventRejectsUnknownFields(t *testing.T) {
	_, err := DecodeEvent([]byte(`{"ts":"2026-08-12T03:00:00Z","project_id":"prj_0123456789abcdef","trace_id":"tr_0123456789abcdef","event":"tool_call","status":"ok","prompt":"do not retain"}`))
	if err == nil {
		t.Fatal("unknown event field was accepted")
	}
}

func TestEventConstructionAllowsOnlyClosedValues(t *testing.T) {
	input := validEventInput()
	input.ExitReason = "mission prompt: secret"
	if _, err := NewEvent(input); err == nil {
		t.Fatal("free-text exit_reason was accepted")
	}

	input = validEventInput()
	input.Tool = "/Users/erick/private/project"
	if _, err := NewEvent(input); err == nil {
		t.Fatal("file path was accepted as tool")
	}
}

func TestEventRoundTripUsesOnlyAllowlistedKeys(t *testing.T) {
	event, err := NewEvent(validEventInput())
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	decoded, err := DecodeEvent(encoded)
	if err != nil {
		t.Fatalf("DecodeEvent: %v", err)
	}
	if decoded.Event != EventToolCall || decoded.Tool != "jacu_report" || decoded.DurationMs != 42 {
		t.Fatalf("decoded event = %+v", decoded)
	}
	if strings.Contains(string(encoded), "prompt") || strings.Contains(string(encoded), "private") {
		t.Fatalf("encoded event contains forbidden content: %s", encoded)
	}
}

func TestV2EnvelopeIsPresentAndV1ReadsAsNoData(t *testing.T) {
	v2 := validEventInput()
	v2.SchemaVersion = "2"
	v2.Level = "user"
	v2.Module = "runtime"
	v2.Stage = "tool_call"
	event, err := NewEvent(v2)
	if err != nil {
		t.Fatalf("NewEvent v2: %v", err)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal v2 event: %v", err)
	}
	for _, field := range []string{`"schema_version":"2"`, `"level":"user"`, `"module":"runtime"`, `"stage":"tool_call"`} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("v2 event lacks %s: %s", field, encoded)
		}
	}

	v1, err := DecodeEvent([]byte(`{"ts":"2026-08-12T03:00:00Z","project_id":"prj_0123456789abcdef","trace_id":"tr_0123456789abcdef","event":"tool_call","status":"ok"}`))
	if err != nil {
		t.Fatalf("DecodeEvent v1: %v", err)
	}
	if v1.SchemaVersion != "1" || v1.Level != "no-data" || v1.Module != "no-data" || v1.Stage != "no-data" || v1.Measurement != "no-data" {
		t.Fatalf("v1 compatibility fields = %+v; want schema 1 and no-data markers", v1)
	}
}

func TestFullEventConstructorHasNoContentSurface(t *testing.T) {
	event, err := NewFullEvent(validEventInput())
	if err != nil {
		t.Fatalf("NewFullEvent: %v", err)
	}
	if event.Level != LevelFull {
		t.Fatalf("full event level = %q; want %q", event.Level, LevelFull)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal full event: %v", err)
	}
	for _, forbidden := range []string{"prompt", "diff", "output", "path", "free_text"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("full event contains forbidden content field %q: %s", forbidden, encoded)
		}
	}
}

func FuzzEventConstructionDoesNotAcceptMissionText(f *testing.F) {
	f.Add("prompt with /private/path and diff output")
	f.Add("secret")
	f.Fuzz(func(t *testing.T, missionText string) {
		// Mission text is intentionally not a parameter of the constructor. The
		// fuzz value models arbitrary caller content that must stay outside events.
		event, err := NewToolCallEvent("prj_0123456789abcdef", "tr_0123456789abcdef", "jacu_report", "ok", 1)
		if err != nil {
			t.Fatalf("NewToolCallEvent: %v", err)
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal event: %v", err)
		}
		if missionText != "" && strings.Contains(string(encoded), missionText) {
			t.Fatalf("mission text leaked into event: %q", missionText)
		}
	})
}
