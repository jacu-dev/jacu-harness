package verify

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newRunner(t *testing.T) Runner {
	t.Helper()
	worktree := t.TempDir()
	return Runner{
		Worktree:      worktree,
		ToolchainHome: t.TempDir(),
		ScratchDir:    t.TempDir(),
		Timeout:       10 * time.Second,
		TailBytes:     4096,
	}
}

func TestRunReportsExitCodeAndOutput(t *testing.T) {
	runner := newRunner(t)
	runner.PathDirs = []string{"/usr/bin", "/bin"}

	result := runner.Run(context.Background(), []string{"printf", "hello"})
	if result.Status != StatusPassed {
		t.Fatalf("status = %q (%s); want passed", result.Status, result.Reason)
	}
	if result.ExitCode == nil || *result.ExitCode != 0 {
		t.Fatalf("exit code = %v; want 0", result.ExitCode)
	}
	if result.StdoutTail != "hello" {
		t.Fatalf("stdout tail = %q; want hello", result.StdoutTail)
	}
	if result.Truncated {
		t.Fatal("truncated on a tiny output")
	}
	if result.Digest == "" || !strings.HasPrefix(result.Digest, "sha256:") {
		t.Fatalf("digest = %q; want sha256 of the full output", result.Digest)
	}
}

func TestRunReportsFailureWithoutLosingStderr(t *testing.T) {
	runner := newRunner(t)
	runner.PathDirs = []string{"/usr/bin", "/bin"}

	result := runner.Run(context.Background(), []string{"false"})
	if result.Status != StatusFailed {
		t.Fatalf("status = %q; want failed", result.Status)
	}
	if result.ExitCode == nil || *result.ExitCode == 0 {
		t.Fatalf("exit code = %v; want non-zero", result.ExitCode)
	}
}

// A pipe nobody reads is a deadlock wearing a timeout's clothes. In the
// previous product a failing test suite filled the kernel buffer, the child
// blocked in write(2), and the run was reported as timed out — which downstream
// would have classified as "flaky, re-run" instead of "the tests broke".
func TestRunDrainsLargeOutputAndDigestsTheWholeThing(t *testing.T) {
	runner := newRunner(t)
	runner.PathDirs = []string{"/usr/bin", "/bin"}
	runner.TailBytes = 256

	script := filepath.Join(t.TempDir(), "flood")
	writeExecutable(t, script, "#!/bin/sh\nawk 'BEGIN{for(i=0;i<40000;i++) print \"0123456789012345678901234567890123456789\"}'\nexit 3\n")
	runner.PathDirs = append([]string{filepath.Dir(script)}, runner.PathDirs...)

	start := time.Now()
	result := runner.Run(context.Background(), []string{"flood"})
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Fatalf("took %v; a large output must not stall the executor", elapsed)
	}
	if result.Status != StatusFailed {
		t.Fatalf("status = %q (%s); want failed, not timed out", result.Status, result.Reason)
	}
	if result.ExitCode == nil || *result.ExitCode != 3 {
		t.Fatalf("exit code = %v; want 3", result.ExitCode)
	}
	if !result.Truncated {
		t.Fatal("truncated = false on a flood")
	}
	if len(result.StdoutTail) > runner.TailBytes {
		t.Fatalf("tail is %d bytes; cap is %d", len(result.StdoutTail), runner.TailBytes)
	}
	if result.BytesOut < 1<<20 {
		t.Fatalf("bytes_out = %d; the executor must count the whole stream", result.BytesOut)
	}
}

func TestRunTimesOutAndKillsTheProcessGroup(t *testing.T) {
	runner := newRunner(t)
	runner.PathDirs = []string{"/usr/bin", "/bin"}
	runner.Timeout = 700 * time.Millisecond

	marker := filepath.Join(t.TempDir(), "grandchild.pid")
	script := filepath.Join(t.TempDir(), "stubborn")
	writeExecutable(t, script, "#!/bin/sh\ntrap '' TERM\n(trap '' TERM; sleep 30 & echo $! > "+marker+"; wait) &\nwait\n")
	runner.PathDirs = append([]string{filepath.Dir(script)}, runner.PathDirs...)

	result := runner.Run(context.Background(), []string{"stubborn"})
	if result.Status != StatusTimedOut {
		t.Fatalf("status = %q; want timed_out", result.Status)
	}
	if result.DurationMs < 700 {
		t.Fatalf("duration = %dms; want at least the timeout", result.DurationMs)
	}
	// The grandchild is the point: SIGTERM is ignored, so only killing the
	// whole group ends it. Killing just the direct child leaks the sleep.
	if pid := readPID(t, marker); pid > 0 && processExists(pid) {
		t.Fatalf("grandchild %d survived the timeout; the group was not killed", pid)
	}
}

// A run cancelled before the spawn must not spawn. Proven by side effect: the
// command would create a file, and the file must not exist.
func TestRunDoesNotSpawnWhenAlreadyCancelled(t *testing.T) {
	runner := newRunner(t)
	runner.PathDirs = []string{"/usr/bin", "/bin"}
	marker := filepath.Join(t.TempDir(), "spawned")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := runner.Run(ctx, []string{"touch", marker})

	if result.Status != StatusNotRun {
		t.Fatalf("status = %q; want not_run", result.Status)
	}
	if result.Reason != "cancelled" {
		t.Fatalf("reason = %q; want cancelled", result.Reason)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("the command ran despite the cancelled context (stat err = %v)", err)
	}
}

// The canary needs both halves. Asserting only that a secret does not leak
// would pass against an executor that fails to pass any environment at all.
func TestRunPassesOnlyTheDeclaredEnvironment(t *testing.T) {
	runner := newRunner(t)
	runner.PathDirs = []string{"/usr/bin", "/bin"}
	t.Setenv("JACU_TEST_SECRET", "top-secret-value")

	leaked := runner.Run(context.Background(), []string{"printenv", "JACU_TEST_SECRET"})
	if leaked.Status == StatusPassed {
		t.Fatalf("the parent secret reached the child: %q", leaked.StdoutTail)
	}

	declared := runner.Run(context.Background(), []string{"printenv", "HOME"})
	if declared.Status != StatusPassed {
		t.Fatalf("HOME status = %q; the declared environment must reach the child", declared.Status)
	}
	if got := strings.TrimSpace(declared.StdoutTail); got != runner.ToolchainHome {
		t.Fatalf("HOME = %q; want the synthetic home %q", got, runner.ToolchainHome)
	}
	if got := strings.TrimSpace(runner.Run(context.Background(), []string{"printenv", "TMPDIR"}).StdoutTail); got != runner.ScratchDir {
		t.Fatalf("TMPDIR = %q; want the run scratch %q", got, runner.ScratchDir)
	}
}

// Names that turn "inherit a variable" into "execute an arbitrary binary as the
// compiler" never enter the environment, not even from project config.
func TestRunNeverPassesToolchainHijackVariables(t *testing.T) {
	runner := newRunner(t)
	runner.PathDirs = []string{"/usr/bin", "/bin"}
	for _, name := range []string{"RUSTC_WRAPPER", "CARGO_BUILD_RUSTC_WRAPPER", "GOFLAGS", "LD_PRELOAD", "NODE_OPTIONS"} {
		t.Setenv(name, "/tmp/evil")
		result := runner.Run(context.Background(), []string{"printenv", name})
		if result.Status == StatusPassed {
			t.Fatalf("%s reached the child: %q", name, result.StdoutTail)
		}
	}
}

func TestRunUsesTheWorktreeAsWorkingDirectory(t *testing.T) {
	runner := newRunner(t)
	runner.PathDirs = []string{"/usr/bin", "/bin"}
	result := runner.Run(context.Background(), []string{"pwd"})
	if result.Status != StatusPassed {
		t.Fatalf("status = %q", result.Status)
	}
	got, err := filepath.EvalSymlinks(strings.TrimSpace(result.StdoutTail))
	if err != nil {
		t.Fatalf("resolve reported cwd: %v", err)
	}
	want, err := filepath.EvalSymlinks(runner.Worktree)
	if err != nil {
		t.Fatalf("resolve worktree: %v", err)
	}
	if got != want {
		t.Fatalf("cwd = %q; want the worktree %q", got, want)
	}
}

// exec.Command resolves through the PATH of the parent process, at construction
// time, not through the PATH placed in cmd.Env. A naive port resolves the
// binary on the host and only then runs it with a sanitised environment — which
// is exactly the hijack the reconstructed PATH exists to prevent.
func TestRunResolvesAgainstTheMinimalPathNotTheParentPath(t *testing.T) {
	hostile := t.TempDir()
	sentinel := filepath.Join(t.TempDir(), "hijacked")
	writeExecutable(t, filepath.Join(hostile, "printf"), "#!/bin/sh\ntouch "+sentinel+"\n")
	t.Setenv("PATH", hostile+string(os.PathListSeparator)+os.Getenv("PATH"))

	runner := newRunner(t)
	runner.PathDirs = []string{"/usr/bin", "/bin"}
	result := runner.Run(context.Background(), []string{"printf", "hello"})

	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("the parent PATH won and the fake printf ran (stat err = %v)", err)
	}
	if result.Status != StatusPassed || result.StdoutTail != "hello" {
		t.Fatalf("result = %#v; want the real printf", result)
	}
}

// The worktree is writable by the model. A verifier supplied by the very thing
// being verified is not verification.
func TestRunRefusesAProgramInsideTheWorktree(t *testing.T) {
	runner := newRunner(t)
	writeExecutable(t, filepath.Join(runner.Worktree, "mytool"), "#!/bin/sh\nexit 0\n")
	runner.PathDirs = []string{runner.Worktree, "/usr/bin", "/bin"}

	result := runner.Run(context.Background(), []string{"mytool"})
	if result.Status != StatusBlocked {
		t.Fatalf("status = %q; want blocked", result.Status)
	}
	if !strings.Contains(result.Reason, "worktree") {
		t.Fatalf("reason = %q; want it to name the worktree", result.Reason)
	}
}

func TestRunBlocksAnUnresolvableProgram(t *testing.T) {
	runner := newRunner(t)
	runner.PathDirs = []string{"/usr/bin", "/bin"}
	result := runner.Run(context.Background(), []string{"definitely-not-a-real-binary-xyz"})
	if result.Status != StatusBlocked {
		t.Fatalf("status = %q; want blocked", result.Status)
	}
	if !strings.Contains(result.Reason, "definitely-not-a-real-binary-xyz") {
		t.Fatalf("reason = %q; it has to name the program that was not found", result.Reason)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	// #nosec G306 -- a test fixture the executor is meant to run.
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	content, err := os.ReadFile(path) // #nosec G304 -- fixture under the test's temp dir.
	if err != nil {
		return 0
	}
	pid := 0
	for _, char := range strings.TrimSpace(string(content)) {
		if char < '0' || char > '9' {
			return 0
		}
		pid = pid*10 + int(char-'0')
	}
	return pid
}
