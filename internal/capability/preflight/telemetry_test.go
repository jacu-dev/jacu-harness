package preflight

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func TestPreflightTelemetryUsesClosedFieldsOnly(t *testing.T) {
	report := Report{Verdict: "block", Findings: []Finding{{Class: ClassCredentialAbsent, Target: "credential", Detail: "model-controlled secret text"}}}
	events, err := preflightTelemetryEvents(report)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Event != telemetry.EventPreflightCheck || events[1].Event != telemetry.EventMissionInterruption {
		t.Fatalf("unexpected preflight telemetry: %+v", events)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "model-controlled") || strings.Contains(string(encoded), "secret text") {
		t.Fatalf("free text reached telemetry: %s", encoded)
	}
}

func TestPreflightTelemetryPreservesEveryFailureClass(t *testing.T) {
	report := Report{Verdict: "block", Findings: []Finding{
		{Class: ClassCredentialAbsent}, {Class: ClassPathMissing}, {Class: ClassCredentialAbsent},
	}}
	events, err := preflightTelemetryEvents(report)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatalf("event count = %d; want two check and two interruption events", len(events))
	}
	seen := map[string]int{}
	for _, event := range events {
		if event.FailureClass != "" {
			seen[event.FailureClass]++
		}
	}
	if seen[ClassCredentialAbsent] != 2 || seen[ClassPathMissing] != 2 {
		t.Fatalf("failure classes = %#v; want both classes on check and interruption events", seen)
	}
}

func FuzzPreflightTelemetryNeverWritesModelText(f *testing.F) {
	f.Add("model supplied command with spaces")
	f.Add("credential value; $(cat secret)")
	f.Fuzz(func(t *testing.T, text string) {
		events, err := preflightTelemetryEvents(Report{Verdict: "block", Findings: []Finding{{Class: ClassDocMissing, Target: "doc", Detail: text}}})
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(events)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), text) && text != "" {
			t.Fatalf("fuzz text reached telemetry: %q", text)
		}
	})
}
