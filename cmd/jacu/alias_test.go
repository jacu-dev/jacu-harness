package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeprecatedAlias(t *testing.T) {
	binary := buildBinary(t)
	dir := filepath.Dir(binary)
	alias := filepath.Join(dir, "jacu-mcp")
	if err := os.Symlink(filepath.Base(binary), alias); err != nil {
		t.Fatalf("create alias symlink: %v", err)
	}

	const deprecation = "jacu-mcp is deprecated; use jacu"

	var aliasStdout, aliasStderr bytes.Buffer
	aliasCommand := exec.Command(alias, "version") // #nosec G204 -- alias is a symlink this test just created next to the built binary.
	aliasCommand.Env = isolatedUserStateEnv(t)
	aliasCommand.Stdout = &aliasStdout
	aliasCommand.Stderr = &aliasStderr
	if err := aliasCommand.Run(); err != nil {
		t.Fatalf("run deprecated alias: %v", err)
	}
	if got, want := strings.TrimSpace(aliasStdout.String()), "jacu "+Version; got != want {
		t.Errorf("alias stdout = %q, want %q", got, want)
	}
	if got := strings.TrimSpace(aliasStderr.String()); got != deprecation {
		t.Errorf("alias stderr = %q, want %q", got, deprecation)
	}

	var binaryStderr bytes.Buffer
	binaryCommand := exec.Command(binary, "version") // #nosec G204 -- binary is the path returned by buildBinary.
	binaryCommand.Env = isolatedUserStateEnv(t)
	binaryCommand.Stderr = &binaryStderr
	if err := binaryCommand.Run(); err != nil {
		t.Fatalf("run canonical binary: %v", err)
	}
	if strings.Contains(binaryStderr.String(), deprecation) {
		t.Errorf("canonical stderr contains deprecation notice: %q", binaryStderr.String())
	}
}
