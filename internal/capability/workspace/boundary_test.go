package workspace

import (
	"reflect"
	"testing"
)

func TestWorkspaceInternalImportBoundary(t *testing.T) {
	got, err := workspaceDirectInternalImports()
	if err != nil {
		t.Fatalf("inspect workspace imports: %v", err)
	}
	want := []string{
		"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile",
		"github.com/jacu-dev/jacu-harness/internal/capability/verify",
		"github.com/jacu-dev/jacu-harness/internal/gitx",
		"github.com/jacu-dev/jacu-harness/internal/project",
		"github.com/jacu-dev/jacu-harness/internal/runner",
		"github.com/jacu-dev/jacu-harness/internal/runstate",
		"github.com/jacu-dev/jacu-harness/internal/runtime",
		"github.com/jacu-dev/jacu-harness/internal/scope",
		"github.com/jacu-dev/jacu-harness/internal/telemetry",
		"github.com/jacu-dev/jacu-harness/internal/userstate",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("workspace internal imports = %v; want %v", got, want)
	}
}
