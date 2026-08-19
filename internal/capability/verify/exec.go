package verify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/userstate"
)

// Per-command outcome. The verdict is a struct, never a formatted string: the
// previous product decided error by comparing the summary to a literal phrase,
// so adding the duration to the formatter would have silently inverted the
// verdict of every successful command.
const (
	StatusPassed   = "passed"
	StatusFailed   = "failed"
	StatusTimedOut = "timed_out"
	StatusBlocked  = "blocked"
	StatusNotRun   = "not_run"
)

// Result is one command's outcome. not_run is a first-class state: a cancelled
// run that reports N failures produces N phantom remediations downstream.
type Result struct {
	ArgV       []string `json:"argv"`
	Status     string   `json:"status"`
	ExitCode   *int     `json:"exit_code,omitempty"`
	DurationMs int64    `json:"duration_ms"`
	StdoutTail string   `json:"stdout_tail"`
	StderrTail string   `json:"stderr_tail"`
	Truncated  bool     `json:"truncated"`
	BytesOut   int64    `json:"bytes_out"`
	Digest     string   `json:"digest"`
	Reason     string   `json:"reason,omitempty"`
}

// Runner executes one allowlisted command under the phase limits.
type Runner struct {
	// Worktree is the working directory and the boundary: a program resolved
	// inside it is refused, because a verifier supplied by the thing being
	// verified is not verification.
	Worktree string
	// PathDirs are the project-declared directories, highest precedence in the
	// reconstructed PATH.
	PathDirs []string
	// ToolchainHome is the synthetic HOME. Dropping HOME entirely breaks every
	// toolchain that needs a cache; passing the real one hands the command
	// ~/.aws and ~/.config/gh.
	ToolchainHome string
	// ScratchDir becomes TMPDIR, outside the worktree.
	ScratchDir string
	Timeout    time.Duration
	TailBytes  int
}

const (
	defaultTimeout   = 120 * time.Second
	defaultTailBytes = 4096
)

// Run executes argv. It never returns an error: every outcome, including a
// refusal, is a Result — the caller aggregates verdicts, it does not handle
// exceptions.
func (r Runner) Run(ctx context.Context, argv []string) Result {
	result := Result{ArgV: append([]string{}, argv...), Status: StatusBlocked}
	if len(argv) == 0 {
		result.Reason = "missing program"
		return result
	}
	if ctx.Err() != nil {
		// Checked before resolving anything: a cancelled run should not pay for
		// a probe, and must not spawn.
		result.Status = StatusNotRun
		result.Reason = "cancelled"
		return result
	}

	binary, err := r.resolveProgram(argv[0])
	if err != nil {
		result.Reason = err.Error()
		return result
	}

	started := time.Now()
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// #nosec G204 -- binary is resolved against the reconstructed PATH and the
	// argv already passed the allowlist; that is the whole point of this package.
	command := exec.CommandContext(commandCtx, binary, argv[1:]...)
	command.Dir = r.Worktree
	command.Env = r.environment()
	configureProcessGroup(command)

	stdout, err := command.StdoutPipe()
	if err != nil {
		result.Reason = err.Error()
		return result
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		result.Reason = err.Error()
		return result
	}

	if err := command.Start(); err != nil {
		result.Reason = err.Error()
		return result
	}

	// A pipe nobody reads is a deadlock that looks like a timeout. Each pipe
	// gets its own reader, keeping the tail, counting every byte, and hashing
	// the complete stream — the digest covers what was produced, not what was
	// kept.
	hasher := sha256.New()
	var mutex sync.Mutex
	outTail := newTailBuffer(r.tailBytes())
	errTail := newTailBuffer(r.tailBytes())
	var waiting sync.WaitGroup
	waiting.Add(2)
	go drain(&waiting, stdout, outTail, hasher, &mutex)
	go drain(&waiting, stderr, errTail, hasher, &mutex)

	// Drain before Wait, never after: Wait closes the pipes, and reading from a
	// closed pipe loses whatever the command wrote. The readers see EOF when the
	// process exits, so this ordering is also what makes Wait return promptly.
	waiting.Wait()
	waitErr := command.Wait()
	stopProcessGroup(command)
	result.DurationMs = time.Since(started).Milliseconds()

	result.StdoutTail = outTail.String()
	result.StderrTail = errTail.String()
	result.BytesOut = outTail.total + errTail.total
	result.Truncated = outTail.truncated || errTail.truncated
	result.Digest = "sha256:" + hex.EncodeToString(hasher.Sum(nil))

	switch {
	case errors.Is(commandCtx.Err(), context.DeadlineExceeded):
		result.Status = StatusTimedOut
		result.Reason = "timeout after " + timeout.String()
	case ctx.Err() != nil:
		result.Status = StatusNotRun
		result.Reason = "cancelled"
	case waitErr == nil:
		result.Status = StatusPassed
		result.ExitCode = intPointer(0)
	default:
		result.Status = StatusFailed
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			result.ExitCode = intPointer(exitErr.ExitCode())
		} else {
			result.Reason = waitErr.Error()
		}
	}
	return result
}

func (r Runner) tailBytes() int {
	if r.TailBytes > 0 {
		return r.TailBytes
	}
	return defaultTailBytes
}

func intPointer(value int) *int { return &value }

func drain(waiting *sync.WaitGroup, reader io.Reader, tail *tailBuffer, hasher io.Writer, mutex *sync.Mutex) {
	defer waiting.Done()
	buffer := make([]byte, 32*1024)
	for {
		read, err := reader.Read(buffer)
		if read > 0 {
			mutex.Lock()
			_, _ = hasher.Write(buffer[:read])
			mutex.Unlock()
			tail.Write(buffer[:read])
		}
		if err != nil {
			return
		}
	}
}

// tailBuffer keeps the last N bytes and counts everything that went by.
type tailBuffer struct {
	limit     int
	data      []byte
	total     int64
	truncated bool
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit, data: make([]byte, 0, limit)}
}

func (t *tailBuffer) Write(chunk []byte) {
	t.total += int64(len(chunk))
	if len(chunk) >= t.limit {
		t.data = append(t.data[:0], chunk[len(chunk)-t.limit:]...)
		t.truncated = true
		return
	}
	if len(t.data)+len(chunk) > t.limit {
		overflow := len(t.data) + len(chunk) - t.limit
		t.data = t.data[overflow:]
		t.truncated = true
	}
	t.data = append(t.data, chunk...)
}

func (t *tailBuffer) String() string { return string(t.data) }

// environment is built from an explicit allowlist, never by inheritance. Names
// that turn "inherit a variable" into "execute an arbitrary binary as the
// compiler" — RUSTC_WRAPPER, CARGO_BUILD_RUSTC_WRAPPER, GOFLAGS, LD_PRELOAD,
// NODE_OPTIONS — are absent by construction, not by a denylist.
func (r Runner) environment() []string {
	return []string{
		"PATH=" + strings.Join(r.searchPath(), string(os.PathListSeparator)),
		"HOME=" + r.ToolchainHome,
		"TMPDIR=" + r.ScratchDir,
		"LANG=C.UTF-8",
	}
}

// searchPath reconstructs the PATH. Three system directories are not enough:
// go usually lives in /usr/local/go/bin or a runner toolcache, cargo under
// ~/.cargo/bin, and on macOS almost everything under /opt/homebrew/bin. Order
// matters as much as membership — toolchain before system, or /usr/bin/go wins
// over the toolchain the project actually uses.
//
// The directories are discovered from the parent environment; the PATH itself
// is never inherited. That distinction is the whole security property.
func (r Runner) searchPath() []string {
	candidates := append([]string{}, r.PathDirs...)
	if goroot := os.Getenv("GOROOT"); goroot != "" {
		candidates = append(candidates, filepath.Join(goroot, "bin"))
	}
	candidates = append(candidates,
		"/usr/local/go/bin",
		"/opt/homebrew/bin",
		"/home/linuxbrew/.linuxbrew/bin",
	)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".cargo", "bin"),
			filepath.Join(home, ".local", "bin"),
		)
	}
	candidates = append(candidates, "/usr/local/bin", "/usr/bin", "/bin")

	seen := make(map[string]struct{}, len(candidates))
	path := make([]string, 0, len(candidates))
	for _, dir := range candidates {
		if dir == "" || dir == "." || !filepath.IsAbs(dir) {
			continue
		}
		if _, duplicate := seen[dir]; duplicate {
			continue
		}
		seen[dir] = struct{}{}
		// #nosec G703 -- the taint is the point: these directories are discovered
		// from the parent environment on purpose (see the comment above), and the
		// probe is a read-only stat on a directory, never an open of a file path.
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			path = append(path, dir)
		}
	}
	return path
}

// SearchPath exposes the verifier's reconstructed search path to read-only
// preflight checks so they predict the environment in which verification runs.
func SearchPath(pathDirs []string) []string {
	return (Runner{PathDirs: append([]string{}, pathDirs...)}).searchPath()
}

// resolveProgram looks the program up by hand against the reconstructed PATH
// and returns an absolute path.
//
// exec.Command resolves through the PATH of the parent process, at construction
// time, and not through the PATH placed in cmd.Env. A naive port resolves the
// binary on the host and only then runs it with a sanitised environment — which
// is precisely the hijack the reconstructed PATH exists to prevent.
func (r Runner) resolveProgram(program string) (string, error) {
	if strings.ContainsAny(program, `/\`) {
		return "", errors.New("program path blocked; use a bare program name")
	}
	searched := r.searchPath()
	for _, dir := range searched {
		candidate := filepath.Join(dir, program)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			continue
		}
		resolved, err := filepath.EvalSymlinks(candidate)
		if err != nil {
			continue
		}
		if err := r.outsideTheWorktree(resolved); err != nil {
			return "", err
		}
		return resolved, nil
	}
	return "", fmt.Errorf("program %q not found in %s", program, strings.Join(searched, string(os.PathListSeparator)))
}

// outsideTheWorktree refuses a binary the run itself could have written.
func (r Runner) outsideTheWorktree(binary string) error {
	for label, root := range map[string]string{"worktree": r.Worktree, "jacu state directory": jacuStateDir()} {
		if root == "" {
			continue
		}
		resolvedRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			resolvedRoot = root
		}
		relative, err := filepath.Rel(resolvedRoot, binary)
		if err != nil {
			continue
		}
		if relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("program resolved inside the %s (%s); it cannot supply its own verifier", label, binary)
		}
	}
	return nil
}

func jacuStateDir() string {
	return userstate.DirOrLocal()
}
