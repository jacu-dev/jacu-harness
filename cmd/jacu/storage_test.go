package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestStorageCLIRejectsApplyAndDryRunTogether(t *testing.T) {
	binary := buildBinary(t)
	// #nosec G204 -- binary is built by buildBinary and receives fixed test argv.
	command := exec.Command(binary, "storage", "prune", "--apply", "--dry-run")
	command.Dir = t.TempDir()
	command.Env = isolatedUserStateEnv(t)
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("storage accepted conflicting flags: %s", output)
	}
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 2 {
		t.Fatalf("storage exit = %v; want exit 2; output=%q", err, output)
	}
	if !strings.Contains(string(output), "--apply and --dry-run cannot be combined") {
		t.Fatalf("storage did not explain conflicting flags: %q", output)
	}
}
