//go:build unix

package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/modelcontrol"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func TestClaudeRunUsesFixedArgvAndPositiveEnvironment(t *testing.T) {
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "claude"), `#!/bin/sh
printf 'ARGS:'
for arg in "$@"; do printf '[%s]' "$arg"; done
printf '\nENV:\n'
env | sort
printf '\nPROMPT:'
cat
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("ANTHROPIC_API_KEY", "parent-secret")
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), ".git"))

	result := Run(context.Background(), attest(t, Request{
		Provider:  ProviderClaude,
		Worktree:  t.TempDir(),
		Objective: "Implement the provider runner safely",
		TailBytes: 32 * 1024,
	}, filepath.Join(binDir, "claude")))
	if result.Status != StatusCompleted {
		t.Fatalf("status = %q (%s); want completed", result.Status, result.Reason)
	}
	for _, fragment := range []string{"[-p]", "[--output-format]", "[stream-json]", "[--setting-sources]", "[]", "[--permission-mode]", "[dontAsk]"} {
		if !strings.Contains(result.StdoutTail, fragment) {
			t.Fatalf("argv output missing %s: %s", fragment, result.StdoutTail)
		}
	}
	if strings.Contains(result.StdoutTail, "parent-secret") || strings.Contains(result.StdoutTail, "GIT_DIR=") {
		t.Fatalf("sensitive parent environment reached child: %s", result.StdoutTail)
	}
	if !strings.Contains(result.StdoutTail, "HOME=") || !strings.Contains(result.StdoutTail, "LANG=") {
		t.Fatalf("declared environment missing: %s", result.StdoutTail)
	}
	if !strings.Contains(result.StdoutTail, "PROMPT:Implement the provider runner safely") {
		t.Fatalf("objective was not sent through stdin: %s", result.StdoutTail)
	}
}

func TestRunDrainsLargeProviderOutput(t *testing.T) {
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "codex"), `#!/bin/sh
awk 'BEGIN { for (i = 0; i < 40000; i++) print "0123456789012345678901234567890123456789" }'
exit 3
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+"/usr/bin:/bin")
	start := time.Now()
	result := Run(context.Background(), attest(t, Request{Provider: ProviderCodex, Worktree: t.TempDir(), Objective: "Run the bounded provider test", TailBytes: 256}, filepath.Join(binDir, "codex")))
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Fatalf("provider output stalled for %v", elapsed)
	}
	if result.Status != StatusFailed || result.ExitCode == nil || *result.ExitCode != 3 {
		t.Fatalf("result = %#v; want failed exit 3", result)
	}
	if !result.Truncated || result.BytesOut < 1<<20 || len(result.StdoutTail) > 256 || result.Digest == "" {
		t.Fatalf("bounded output evidence = %#v", result)
	}
}

func TestRunCancellationKillsProviderDescendant(t *testing.T) {
	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "descendant.pid")
	ready := filepath.Join(t.TempDir(), "descendant.ready")
	if err := syscall.Mkfifo(ready, 0o600); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(binDir, "claude"), `#!/bin/sh
trap '' TERM
(trap '' TERM; sleep 30 & echo $! > `+shellQuote(marker)+`; printf ready > `+shellQuote(ready)+`; wait) &
wait
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+"/usr/bin:/bin")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := make(chan Result, 1)
	go func() {
		results <- Run(ctx, attest(t, Request{Provider: ProviderClaude, Worktree: t.TempDir(), Objective: "Cancel the bounded provider test", Timeout: time.Minute, TailBytes: 4096}, filepath.Join(binDir, "claude")))
	}()
	// The FIFO is an explicit process-to-test handshake: cancellation cannot
	// happen until the grandchild PID has been written and is blocked in wait.
	type handshake struct {
		content []byte
		err     error
	}
	handshakes := make(chan handshake, 1)
	go func() {
		readyFile, openErr := os.Open(ready) // #nosec G304 -- FIFO is test-owned.
		if openErr != nil {
			handshakes <- handshake{err: openErr}
			return
		}
		content, readErr := io.ReadAll(readyFile)
		closeErr := readyFile.Close()
		if readErr != nil {
			handshakes <- handshake{err: readErr}
			return
		}
		handshakes <- handshake{content: content, err: closeErr}
	}()
	var readyResult handshake
	select {
	case readyResult = <-handshakes:
	case <-time.After(5 * time.Second): // watchdog only; it does not order cancellation.
		cancel()
		if writer, openErr := os.OpenFile(ready, os.O_WRONLY, 0); openErr == nil { // #nosec G304 -- FIFO is created in this test's temporary directory.
			_ = writer.Close()
		}
		t.Fatal("provider did not complete descendant readiness handshake")
	}
	if readyResult.err != nil || string(readyResult.content) != "ready" {
		t.Fatalf("descendant readiness handshake = %q, err=%v", readyResult.content, readyResult.err)
	}
	content, err := os.ReadFile(marker) // #nosec G304 -- marker is created under this test's temporary directory.
	if err != nil {
		t.Fatalf("provider did not create descendant marker: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		t.Fatalf("descendant pid = %q: %v", strings.TrimSpace(string(content)), err)
	}
	cancel()
	var result Result
	select {
	case result = <-results:
	case <-time.After(5 * time.Second): // watchdog only; ordering is the FIFO handshake above.
		t.Fatal("runner did not return after cancellation")
	}
	if result.Status != StatusCancelled {
		t.Fatalf("status = %q (%s); want cancelled", result.Status, result.Reason)
	}
	assertDescendantTerminated(t, pid)
}

func assertDescendantTerminated(t *testing.T, pid int) {
	t.Helper()
	if err := syscall.Kill(pid, 0); err != nil {
		return // ESRCH: the kernel has fully reaped the child.
	}
	// On Unix, kill(pid, 0) also succeeds for a zombie until init reaps it.
	// Inspect the kernel process state so the test accepts only a terminal Z
	// process, never a sleeping/running descendant that still executes code.
	output, err := exec.Command("/bin/ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output() // #nosec G204 -- fixed test-owned pid and fixed ps argv.
	if err != nil {
		return // ps reports no process after a race with reaping.
	}
	if strings.HasPrefix(strings.TrimSpace(string(output)), "Z") {
		return
	}
	t.Fatalf("descendant %d survived cancellation with state %q", pid, strings.TrimSpace(string(output)))
}

func TestRunRejectsUnknownProviderBeforeSpawn(t *testing.T) {
	result := Run(context.Background(), Request{Provider: Provider("unknown"), Worktree: t.TempDir(), Objective: "Run an unknown provider"})
	if result.Status != StatusBlocked || result.Reason == "" {
		t.Fatalf("result = %#v; want sanitized blocked refusal", result)
	}
}

func TestRunEmitsClosedRunnerTelemetry(t *testing.T) {
	base := t.TempDir()
	t.Setenv("JACU_HOME", base)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "claude"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+"/usr/bin:/bin")
	result := Run(context.Background(), attest(t, Request{
		Provider: ProviderClaude, ProjectID: "prj_0123456789abcdef", Worktree: t.TempDir(), Objective: "bounded runner test",
	}, filepath.Join(binDir, "claude")))
	if result.Status != StatusCompleted {
		t.Fatalf("runner status = %q; want completed", result.Status)
	}
	events, err := telemetry.NewStoreAt(base).ReadSince(time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatalf("read telemetry: %v", err)
	}
	if len(events) != 1 || events[0].Event != telemetry.EventFlowNode || events[0].Tool != string(ProviderClaude) || events[0].Status != StatusCompleted {
		t.Fatalf("runner telemetry = %+v", events)
	}
}

func TestRunBlocksWhenAttestationMissing(t *testing.T) {
	result := Run(context.Background(), Request{Provider: ProviderClaude, Worktree: t.TempDir(), Objective: "missing attestation"})
	if result.Status != StatusBlocked || result.Reason != "attestation incomplete" {
		t.Fatalf("result = %#v; want blocked before spawn", result)
	}
}

func attest(t *testing.T, request Request, binary string) Request {
	t.Helper()
	abs, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	request.CLI = modelcontrol.SignedCLI{
		Path: abs, SHA256: "sha256:" + hex.EncodeToString(sum[:]),
		Signer: "test-signer", Signature: "test-signature",
	}
	request.Verifier = func(modelcontrol.SignedCLI) bool { return true }
	return request
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	// #nosec G306 -- the test fixture must be executable by the provider process.
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
