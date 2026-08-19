package telemetry_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func FuzzVerifyDenialNeverWritesProgramName(f *testing.F) {
	for _, seed := range []string{"go", "curl", "npm;cat secrets", "gо", "program\x00name"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, refusedProgram string) {
		refusedProgram = "__refused_program__" + refusedProgram
		event, err := telemetry.NewEvent(telemetry.EventInput{
			Timestamp: time.Now().UTC(), ProjectID: "prj_0123456789abcdef", TraceID: "tr_0123456789abcdef",
			Module: "verify", Stage: "denial", Event: telemetry.EventVerifyDenial, Status: "blocked",
			Reason: "not_in_allowlist", ProgramKnown: false,
		})
		if err != nil {
			t.Fatalf("construct denial: %v", err)
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal denial: %v", err)
		}
		if refusedProgram != "" && strings.Contains(string(encoded), refusedProgram) {
			t.Fatalf("refused program leaked into telemetry: %q", refusedProgram)
		}
	})
}
