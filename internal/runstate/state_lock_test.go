package runstate

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const (
	lockHelperMode  = "JACU_RUNSTATE_LOCK_HELPER_MODE"
	lockHelperRepo  = "JACU_RUNSTATE_LOCK_HELPER_REPO"
	lockHelperReady = "JACU_RUNSTATE_LOCK_HELPER_READY"
)

// TestRunstateLockHelper is executed in two child processes by
// TestSaveSerializesAcrossProcesses. Keeping the helper in the test binary
// makes the RED test exercise the real file lock and Save path, not a mock.
func TestRunstateLockHelper(t *testing.T) {
	mode := os.Getenv(lockHelperMode)
	if mode == "" {
		return
	}
	repo := os.Getenv(lockHelperRepo)
	state, err := Load(repo, "run_0000000000000001")
	if err != nil {
		t.Fatalf("helper Load: %v", err)
	}

	switch mode {
	case "holder":
		err = WithLock(repo, func() error {
			// #nosec G703 -- the parent test supplies a path in t.TempDir.
			if writeErr := os.WriteFile(os.Getenv(lockHelperReady), []byte("ready\n"), 0o600); writeErr != nil {
				return writeErr
			}
			time.Sleep(250 * time.Millisecond)
			state.Status = StatusReviewed
			return SaveLocked(repo, state)
		})
	case "worker":
		state.Status = StatusApplied
		err = Save(repo, state)
	default:
		t.Fatalf("unknown helper mode %q", mode)
	}
	if err != nil {
		t.Fatalf("helper %s: %v", mode, err)
	}
}

func TestSaveSerializesAcrossProcesses(t *testing.T) {
	repo := newStateRepo(t)
	initial := fixtureRun("run_0000000000000001", time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC))
	if err := Save(repo, initial); err != nil {
		t.Fatalf("initial Save: %v", err)
	}
	ready := filepath.Join(t.TempDir(), "ready")

	// #nosec -- this is the current test binary and the flag is a fixed test
	// selector; no repository or user input becomes a command.
	holder := exec.Command(os.Args[0], "-test.run=TestRunstateLockHelper")
	holder.Env = append(os.Environ(),
		lockHelperMode+"=holder",
		lockHelperRepo+"="+repo,
		lockHelperReady+"="+ready,
	)
	if err := holder.Start(); err != nil {
		t.Fatalf("start holder: %v", err)
	}
	waitForFile(t, ready)

	// #nosec -- see the holder process above.
	worker := exec.Command(os.Args[0], "-test.run=TestRunstateLockHelper")
	worker.Env = append(os.Environ(),
		lockHelperMode+"=worker",
		lockHelperRepo+"="+repo,
		lockHelperReady+"="+ready,
	)
	if err := worker.Start(); err != nil {
		t.Fatalf("start worker: %v", err)
	}

	if err := holder.Wait(); err != nil {
		t.Fatalf("holder: %v", err)
	}
	if err := worker.Wait(); err != nil {
		t.Fatalf("worker did not observe holder's persisted transition: %v", err)
	}

	got, err := Load(repo, initial.RunID)
	if err != nil {
		t.Fatalf("final Load: %v", err)
	}
	if got.Status != StatusApplied {
		t.Fatalf("final status = %q; want serialized reviewed -> applied transition", got.Status)
	}
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("helper did not acquire lock before timeout")
}
