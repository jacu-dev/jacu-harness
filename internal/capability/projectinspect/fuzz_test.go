package projectinspect

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
)

func FuzzInputDecode(f *testing.F) {
	root := f.TempDir()
	writeFixture(f, root, "go.mod", "module example.com/project\n\ngo 1.26\n")
	capability := capabilityruntime.Capability{
		Spec: capabilityruntime.ToolSpec{
			Name:           ToolName,
			Version:        "1",
			Risk:           capabilityruntime.RiskSafe,
			ReadOnly:       true,
			Idempotent:     true,
			Timeout:        10 * time.Second,
			MaxInputBytes:  256 * 1024,
			MaxOutputBytes: 16 * 1024,
		},
		Handler: inspectHandler(root),
	}

	f.Add([]byte(`{}`))
	f.Add([]byte(`{"max_files":-1}`))
	f.Add([]byte("{\"include\":[\"\x00\"]}"))
	f.Add(bytes.Repeat([]byte("x"), 1024*1024))

	f.Fuzz(func(t *testing.T, input []byte) {
		result := capabilityruntime.Execute(context.Background(), capability, json.RawMessage(input))
		if !slices.Contains([]string{"ok", "partial", "blocked", "failed"}, result.Status) {
			t.Fatalf("status inválido: %q", result.Status)
		}
		if result.Summary == "" {
			t.Fatal("summary vazio")
		}
		if !strings.HasPrefix(result.TraceID, "tr_") {
			t.Fatalf("trace_id inválido: %q", result.TraceID)
		}
		if _, err := json.Marshal(result); err != nil {
			t.Fatalf("envelope inválido: %v", err)
		}
	})
}
