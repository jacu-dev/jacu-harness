package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVersionString(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
}

// The usage line is the first thing a new installer sees when they type the
// command wrong, and for five phases it told them the server did not exist yet.
// Shipped output that is false about the product is a defect, not cosmetics.
func TestUsageNamesTheSubcommandsThatExist(t *testing.T) {
	binary := buildBinary(t)

	for _, args := range [][]string{{}, {"nonsense"}} {
		label := "no arguments"
		if len(args) > 0 {
			label = args[0]
		}
		command := exec.Command(binary, args...)
		command.Env = isolatedUserStateEnv(t)
		output, err := command.CombinedOutput()
		if err == nil {
			t.Errorf("%s: exited 0; a usage error must fail", label)
		}
		text := string(output)

		if !strings.Contains(text, "usage: jacu") {
			t.Errorf("%s: usage does not name the command jacu: %q", label, text)
		}
		if strings.Contains(text, "jacu-mcp") {
			t.Errorf("%s: usage still names the retired command jacu-mcp: %q", label, text)
		}
		for _, subcommand := range []string{"serve", "doctor", "init", "version", "report", "statusline", "stats", "provenance"} {
			if !strings.Contains(text, subcommand) {
				t.Errorf("%s: usage does not name %q: %q", label, subcommand, text)
			}
		}
		// The message this replaces promised the entrypoint for a phase that
		// closed in 2026. Any promise of a future phase here is that bug again.
		if strings.Contains(text, "fase 01") || strings.Contains(text, "lands in") {
			t.Errorf("%s: usage still promises the entrypoint as future work: %q", label, text)
		}
	}

	// An unknown subcommand says which one, so the reader sees their own typo.
	unknown := exec.Command(binary, "nonsense")
	unknown.Env = isolatedUserStateEnv(t)
	output, _ := unknown.CombinedOutput()
	if !strings.Contains(string(output), "nonsense") {
		t.Errorf("usage does not name the rejected subcommand: %q", output)
	}
}

func TestParseSinceArgsAcceptsDaysAndRejectsInvalidValues(t *testing.T) {
	duration, err := parseSinceArgs([]string{"--since", "30d"})
	if err != nil || duration != 30*24*time.Hour {
		t.Fatalf("parseSinceArgs 30d = %v, %v", duration, err)
	}
	if _, err := parseSinceArgs([]string{"--since", "wat"}); err == nil {
		t.Fatal("invalid --since was accepted")
	}
	if _, err := parseSinceArgs([]string{"--since"}); err == nil {
		t.Fatal("missing --since value was accepted")
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "jacu")
	if output, err := exec.Command("go", "build", "-buildvcs=false", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, output)
	}
	return binary
}

func isolatedUserStateEnv(t *testing.T) []string {
	t.Helper()
	env := os.Environ()
	filtered := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if strings.HasPrefix(entry, "JACU_HOME=") {
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered, "JACU_HOME="+t.TempDir())
}
