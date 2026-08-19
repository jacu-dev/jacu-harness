//go:build hosteval

package hosteval

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Host is one coding CLI the harness can drive headlessly.
//
// The argv templates below are the headless invocation of each CLI as of
// 2026-08-14. They are the part of this file most likely to rot: every host
// renames its non-interactive flag eventually. When one breaks, the failure is
// loud (Probe reports the exact argv) rather than silent, and only this table
// changes.
type Host struct {
	Name string
	Bin  string
	// Argv returns the full argv for a prompt. It never interpolates the
	// prompt into a shell string: the prompt is model-adjacent text and a
	// shell would be a metacharacter injection surface for no benefit.
	Argv func(prompt string) []string
}

// Hosts is the execution matrix. Gemini is present because the owner named it
// a target on 2026-08-14; it has never been validated as an MCP host in this
// repository, so expect it to report unavailable until that is done.
func Hosts() []Host {
	return []Host{
		{Name: "claude", Bin: "claude", Argv: func(p string) []string { return []string{"-p", p} }},
		{Name: "codex", Bin: "codex", Argv: func(p string) []string { return []string{"exec", p} }},
		{Name: "opencode", Bin: "opencode", Argv: func(p string) []string { return []string{"run", p} }},
		{Name: "gemini", Bin: "gemini", Argv: func(p string) []string { return []string{"-p", p} }},
	}
}

// HostByName looks up a host in the matrix.
func HostByName(name string) (Host, bool) {
	for _, h := range Hosts() {
		if h.Name == name {
			return h, true
		}
	}
	return Host{}, false
}

// ErrHostUnavailable means the CLI is not installed or not on PATH. It is
// reported as `skipped`, never as `pass`: an absent host proves nothing, and
// counting it green is exactly how a matrix comes to claim coverage it does
// not have.
var ErrHostUnavailable = errors.New("host unavailable")

// Probe resolves the binary and returns its absolute path.
func (h Host) Probe() (string, error) {
	path, err := exec.LookPath(h.Bin)
	if err != nil {
		return "", fmt.Errorf("%w: %s not on PATH", ErrHostUnavailable, h.Bin)
	}
	return path, nil
}

// Run sends the prompt to the host in workdir and returns its combined output.
// A non-zero exit is returned with the output attached, not swallowed: a host
// that refused the prompt is a result the report has to show.
func (h Host) Run(ctx context.Context, workdir, prompt string, timeout time.Duration) (string, error) {
	path, err := h.Probe()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, h.Argv(prompt)...) //nolint:gosec // argv is a fixed template plus one prompt argument
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), "JACU_TELEMETRY=on")
	out, runErr := cmd.CombinedOutput()

	if ctx.Err() != nil {
		return string(out), fmt.Errorf("host %s timed out after %s", h.Name, timeout)
	}
	if runErr != nil {
		return string(out), fmt.Errorf("host %s exited with error: %w", h.Name, runErr)
	}
	return string(out), nil
}

// TruncationWarning reports whether the host said it shortened skill
// descriptions to fit its budget. This is the 2026-08-13 Codex finding: the
// warning is the mechanism behind the 4.1 routing failure, so a case that
// passes while the warning is present passed for the wrong reason and the
// report has to say so.
func TruncationWarning(hostOutput string) bool {
	lower := strings.ToLower(hostOutput)
	for _, needle := range []string{
		"descriptions were shortened",
		"description were shortened",
		"skills context budget",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}
