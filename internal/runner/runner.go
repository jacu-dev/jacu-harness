// Package runner contains the bounded, provider-specific process adapter used
// by the headless E2 command. It deliberately has no shell or credential store.
package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/modelcontrol"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderCodex  Provider = "codex"
)

const (
	PermissionReadOnly = "read-only"
	PermissionWorktree = "workspace-write"
	StatusCompleted    = "completed"
	StatusFailed       = "failed"
	StatusTimedOut     = "timed_out"
	StatusCancelled    = "cancelled"
	StatusBlocked      = "blocked"
	defaultTimeout     = 10 * time.Minute
	defaultTailBytes   = 16 * 1024
	maximumTailBytes   = 64 * 1024
	terminationGrace   = 200 * time.Millisecond
)

type Request struct {
	ProjectID  string
	Provider   Provider
	Worktree   string
	Objective  string
	Model      string
	Permission string
	Timeout    time.Duration
	TailBytes  int
	CLI        modelcontrol.SignedCLI
	Verifier   modelcontrol.SignatureVerifier
}

// HarnessProcess owns one bounded provider process. Keeping the request on
// this value makes the spawn boundary explicit and gives future providers one
// lifecycle owner instead of one-off exec.Command call sites.
type HarnessProcess struct {
	request Request
}

func NewHarnessProcess(request Request) HarnessProcess {
	return HarnessProcess{request: request}
}

func (process HarnessProcess) Run(ctx context.Context) Result {
	return run(ctx, process.request)
}

type Result struct {
	Provider   Provider `json:"provider"`
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

type invocation struct {
	binary string
	args   []string
}

// Run executes one provider invocation. It returns a typed result for all
// expected outcomes, including validation refusal, so callers never infer
// governance from an error string or child stderr.
func Run(ctx context.Context, request Request) Result {
	return NewHarnessProcess(request).Run(ctx)
}

func run(ctx context.Context, request Request) (result Result) {
	started := time.Now()
	result = Result{Provider: request.Provider, Status: StatusBlocked}
	defer func() {
		if request.ProjectID == "" {
			return
		}
		telemetry.EmitBestEffortInput(telemetry.EventInput{
			Timestamp: time.Now().UTC(), ProjectID: request.ProjectID, TraceID: telemetry.NewTraceID(),
			Event: telemetry.EventFlowNode, Tool: string(request.Provider), Status: result.Status,
			Duration: time.Since(started), ExitReason: runnerExitReason(result.Status),
		})
	}()
	if err := validateRequest(request); err != nil {
		result.Reason = err.Error()
		return result
	}
	if err := modelcontrol.ValidateSignedCLI(request.CLI, request.Verifier); err != nil {
		result.Reason = "attestation incomplete"
		return result
	}
	actual, digestErr := fileSHA256(request.CLI.Path)
	if digestErr != nil || actual != request.CLI.SHA256 {
		result.Reason = "attestation incomplete"
		return result
	}
	if err := ctx.Err(); err != nil {
		result.Status = StatusCancelled
		result.Reason = "cancelled before spawn"
		return result
	}

	binary := request.CLI.Path
	call, err := buildInvocation(request, binary)
	if err != nil {
		result.Reason = err.Error()
		return result
	}

	timeout := request.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// #nosec G204 -- binary is resolved from the two-provider allowlist and the
	// arguments are built by buildInvocation; no shell parses them.
	command := exec.CommandContext(commandCtx, call.binary, call.args...)
	command.Dir = request.Worktree
	command.Env = cleanEnvironment()
	command.Stdin = strings.NewReader(request.Objective + "\n")
	configureProcessGroup(command)

	stdout, err := command.StdoutPipe()
	if err != nil {
		result.Reason = "provider stdout setup failed"
		return result
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		result.Reason = "provider stderr setup failed"
		return result
	}
	if err := command.Start(); err != nil {
		result.Reason = "provider failed to start"
		return result
	}

	out := newCapture(request.TailBytes)
	errOut := newCapture(request.TailBytes)
	var readers sync.WaitGroup
	readers.Add(2)
	go drain(&readers, stdout, out)
	go drain(&readers, stderr, errOut)
	// The readers must finish before Wait: this drains pipe data without the
	// deadlock caused by waiting on a child whose output pipe is full.
	readers.Wait()
	waitErr := command.Wait()
	stopProcessGroup(command)

	result.DurationMs = time.Since(started).Milliseconds()
	result.StdoutTail = out.tail.String()
	result.StderrTail = errOut.tail.String()
	result.BytesOut = out.bytes + errOut.bytes
	result.Truncated = out.tail.truncated || errOut.tail.truncated
	result.Digest = digestCaptures(out, errOut)

	switch {
	case errors.Is(commandCtx.Err(), context.DeadlineExceeded):
		result.Status = StatusTimedOut
		result.Reason = "provider timeout exceeded"
	case ctx.Err() != nil:
		result.Status = StatusCancelled
		result.Reason = "provider cancelled"
	case waitErr == nil:
		result.Status = StatusCompleted
		result.ExitCode = intPointer(0)
	default:
		result.Status = StatusFailed
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			result.ExitCode = intPointer(exitErr.ExitCode())
		} else {
			result.Reason = "provider process failed"
		}
	}
	return result
}

func runnerExitReason(status string) string {
	switch status {
	case StatusCompleted:
		return "completed"
	case StatusTimedOut:
		return "timed_out"
	case StatusCancelled:
		return "cancelled"
	case StatusBlocked:
		return "blocked"
	default:
		return "failed"
	}
}

func validateRequest(request Request) error {
	if request.Provider != ProviderClaude && request.Provider != ProviderCodex {
		return errors.New("provider is not allowlisted")
	}
	if strings.TrimSpace(request.Objective) == "" {
		return errors.New("objective is required")
	}
	if !filepath.IsAbs(request.Worktree) {
		return errors.New("worktree must be absolute")
	}
	info, err := os.Stat(request.Worktree)
	if err != nil || !info.IsDir() {
		return errors.New("worktree is unavailable")
	}
	if request.Model != "" && !validOpaque(request.Model) {
		return errors.New("model id is invalid")
	}
	if request.Permission != "" && request.Permission != PermissionReadOnly && request.Permission != PermissionWorktree {
		return errors.New("permission profile is invalid")
	}
	if request.Timeout < 0 || request.Timeout > 30*time.Minute {
		return errors.New("timeout exceeds the runner limit")
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func buildInvocation(request Request, binary string) (invocation, error) {
	permission := request.Permission
	if permission == "" {
		permission = PermissionWorktree
	}
	switch request.Provider {
	case ProviderClaude:
		allowed := []string{"Read", "Glob", "Grep"}
		if permission == PermissionWorktree {
			allowed = append(allowed, "Edit", "Write", "Bash")
		}
		settings, err := json.Marshal(map[string]any{
			"permissions": map[string]any{"deny": []string{"Read(../**)", "Glob(../**)", "Grep(../**)", "Read(.ssh/**)", "Read(.claude/**)", "Read(.aws/**)", "Read(.config/**)"}},
			"sandbox": map[string]any{
				"enabled": true, "failIfUnavailable": true, "allowUnsandboxedCommands": false,
				"filesystem": map[string]any{"denyRead": []string{"/", "~/"}, "allowRead": []string{request.Worktree}},
				"network":    map[string]any{"allowedDomains": []string{}},
			},
		})
		if err != nil {
			return invocation{}, errors.New("provider settings could not be encoded")
		}
		args := []string{
			"-p", "--safe-mode", "--strict-mcp-config", "--output-format", "stream-json", "--verbose",
			"--permission-mode", "dontAsk", "--settings", string(settings), "--setting-sources", "",
			"--tools", strings.Join(allowed, ","), "--disallowedTools", "Read(../**),Glob(../**),Grep(../**),Read(.ssh/**),Read(.claude/**),Read(.aws/**),Read(.config/**)",
			"--allowedTools", strings.Join(allowed, ","),
		}
		if request.Model != "" {
			args = append(args, "--model", request.Model)
		}
		return invocation{binary: binary, args: args}, nil
	case ProviderCodex:
		sandbox := "workspace-write"
		if permission == PermissionReadOnly {
			sandbox = "read-only"
		}
		args := []string{"--ask-for-approval", "untrusted", "--sandbox", sandbox, "--cd", request.Worktree, "exec", "--json"}
		if request.Model != "" {
			args = append(args, "--model", request.Model)
		}
		return invocation{binary: binary, args: append(args, "-")}, nil
	default:
		return invocation{}, errors.New("provider is not allowlisted")
	}
}

func cleanEnvironment() []string {
	allowed := map[string]struct{}{
		"PATH": {}, "HOME": {}, "CODEX_HOME": {}, "LANG": {}, "LC_ALL": {}, "LC_CTYPE": {}, "TMPDIR": {}, "TMP": {}, "TEMP": {},
	}
	environment := make([]string, 0, len(allowed))
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, keep := allowed[name]; keep {
				environment = append(environment, entry)
			}
		}
	}
	return environment
}

func validOpaque(value string) bool {
	if value == "" || len(value) > 256 || value == "." || value == ".." || strings.Contains(value, "..") || strings.HasPrefix(value, "-") {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == '-' || char == '.' {
			continue
		}
		return false
	}
	return true
}

type capture struct {
	tail  *tailBuffer
	hash  [32]byte
	bytes int64
}

func newCapture(limit int) *capture {
	if limit <= 0 {
		limit = defaultTailBytes
	}
	if limit > maximumTailBytes {
		limit = maximumTailBytes
	}
	return &capture{tail: &tailBuffer{limit: limit}}
}

func drain(readers *sync.WaitGroup, reader io.Reader, capture *capture) {
	defer readers.Done()
	hasher := sha256.New()
	buffer := make([]byte, 32*1024)
	for {
		read, err := reader.Read(buffer)
		if read > 0 {
			capture.bytes += int64(read)
			_, _ = hasher.Write(buffer[:read])
			capture.tail.Write(buffer[:read])
		}
		if err != nil {
			copy(capture.hash[:], hasher.Sum(nil))
			return
		}
	}
}

func digestCaptures(stdout, stderr *capture) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("stdout\x00"))
	_, _ = hasher.Write(stdout.hash[:])
	_, _ = hasher.Write([]byte("stderr\x00"))
	_, _ = hasher.Write(stderr.hash[:])
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil))
}

type tailBuffer struct {
	limit     int
	data      []byte
	truncated bool
}

func (t *tailBuffer) Write(chunk []byte) {
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

func intPointer(value int) *int { return &value }
