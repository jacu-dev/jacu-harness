//go:build e2e

// Package e2e drives the shipped binary the way an MCP host does: a real
// process, real stdio framing, real JSON-RPC. The in-process suite in
// internal/mcpadapter already covers tool metadata and the governed flow; what
// only a spawned process can prove is that the binary a user installs speaks
// the protocol, keeps stdout protocol-pure, and leaves the repository in the
// state the envelope claims.
package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const (
	// defaultProtocolVersion is the frozen decision in PLANO.md. From this
	// version on the protocol is sessionless: there is no initialize
	// handshake, and every request declares the version in its _meta.
	defaultProtocolVersion = "2026-07-28"
	// fallbackProtocolVersion is what a host speaking the legacy initialize
	// handshake gets. The SDK caps that path deliberately, because
	// initialize is deprecated as of the default version above.
	fallbackProtocolVersion = "2025-11-25"

	metaKeyProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	metaKeyClientInfo         = "io.modelcontextprotocol/clientInfo"
	metaKeyClientCapabilities = "io.modelcontextprotocol/clientCapabilities"

	// callTimeout bounds every single JSON-RPC round trip.
	callTimeout = 90 * time.Second
	// maxLineBytes must exceed the largest envelope a tool may emit; the
	// output cap is 32 KiB on jacu_diff, and tools/list is ~14 KiB.
	maxLineBytes = 4 << 20
)

var (
	binaryOnce sync.Once
	binaryPath string
	binaryErr  error
)

// serverBinary builds cmd/jacu once per test run and returns its path. The
// binary — not the package — is the unit under test here.
func serverBinary(t *testing.T) string {
	t.Helper()
	binaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "jacu-e2e-bin-")
		if err != nil {
			binaryErr = err
			return
		}
		binaryPath = filepath.Join(dir, "jacu")
		build := exec.Command("go", "build", "-o", binaryPath, "./cmd/jacu")
		build.Dir = repoRoot(t)
		if output, err := build.CombinedOutput(); err != nil {
			binaryErr = fmt.Errorf("build jacu: %w: %s", err, output)
		}
	})
	if binaryErr != nil {
		t.Fatalf("%v", binaryErr)
	}
	return binaryPath
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id"`
	Method  string          `json:"method"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

// session is a JSON-RPC client speaking newline-delimited JSON over the
// server's stdin/stdout, exactly as the MCP stdio transport specifies.
type session struct {
	t      *testing.T
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr *bytes.Buffer

	// meta is attached to every request in the sessionless protocol and is
	// nil for a session driven through the legacy initialize handshake.
	meta map[string]any

	mu        sync.Mutex
	nextID    int
	pending   map[int]chan rpcMessage
	stdoutRaw []string
	readErr   error
	done      chan struct{}
}

// startSession spawns the binary with the project as its working directory and
// an environment of its own: HOME points at a scratch directory so worktrees
// and any toolchain state stay inside the test, never in the developer's home.
func startSession(t *testing.T, project string) *session {
	t.Helper()
	home := t.TempDir()
	cmd := exec.Command(serverBinary(t), "serve")
	cmd.Dir = project
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=" + os.Getenv("PATH"),
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}

	s := &session{
		t:       t,
		cmd:     cmd,
		stdin:   stdin,
		stderr:  stderr,
		pending: map[int]chan rpcMessage{},
		done:    make(chan struct{}),
	}
	go s.read(stdout)
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		<-s.done
	})
	return s
}

func (s *session) read(stdout io.Reader) {
	defer close(s.done)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	for scanner.Scan() {
		line := scanner.Text()
		var message rpcMessage
		decodeErr := json.Unmarshal([]byte(line), &message)

		s.mu.Lock()
		s.stdoutRaw = append(s.stdoutRaw, line)
		var waiter chan rpcMessage
		if decodeErr == nil && message.ID != nil {
			waiter = s.pending[*message.ID]
			delete(s.pending, *message.ID)
		}
		s.mu.Unlock()

		if waiter != nil {
			waiter <- message
		}
	}
	s.mu.Lock()
	s.readErr = scanner.Err()
	s.mu.Unlock()
}

func (s *session) send(payload map[string]any) {
	s.t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		s.t.Fatalf("encode request: %v", err)
	}
	if _, err := s.stdin.Write(append(encoded, '\n')); err != nil {
		s.t.Fatalf("write request: %v (stderr: %s)", err, s.stderrText())
	}
}

// call issues a request and returns its raw result, failing the test on a
// JSON-RPC error, a timeout, or a server that died mid-call.
func (s *session) call(method string, params map[string]any) json.RawMessage {
	s.t.Helper()
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	waiter := make(chan rpcMessage, 1)
	s.pending[id] = waiter
	s.mu.Unlock()

	s.send(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  s.withMeta(params),
	})

	select {
	case message := <-waiter:
		if message.Error != nil {
			s.t.Fatalf("%s returned JSON-RPC error %d: %s", method, message.Error.Code, message.Error.Message)
		}
		if message.JSONRPC != "2.0" {
			s.t.Fatalf("%s response jsonrpc = %q; want 2.0", method, message.JSONRPC)
		}
		return message.Result
	case <-s.done:
		s.t.Fatalf("server exited while waiting for %s (stderr: %s)", method, s.stderrText())
	case <-time.After(callTimeout):
		s.t.Fatalf("timeout waiting for %s (stderr: %s)", method, s.stderrText())
	}
	return nil
}

func (s *session) notify(method string, params map[string]any) {
	s.t.Helper()
	s.send(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  s.withMeta(params),
	})
}

// withMeta stamps the sessionless protocol envelope onto a request. In the
// legacy mode it returns the params untouched — sending the meta there would
// make the server reject initialize as removed from the new protocol.
func (s *session) withMeta(params map[string]any) map[string]any {
	if s.meta == nil {
		return params
	}
	stamped := make(map[string]any, len(params)+1)
	for key, value := range params {
		stamped[key] = value
	}
	stamped["_meta"] = s.meta
	return stamped
}

// useSessionlessProtocol switches the session to the default protocol version:
// no handshake, the version and the client identity travel in every request.
func (s *session) useSessionlessProtocol(clientName string) {
	s.meta = map[string]any{
		metaKeyProtocolVersion:    defaultProtocolVersion,
		metaKeyClientCapabilities: map[string]any{},
		metaKeyClientInfo:         map[string]any{"name": clientName, "version": "0"},
	}
}

// discover is the sessionless replacement for the initialize handshake.
func (s *session) discover() ([]string, map[string]any) {
	s.t.Helper()
	raw := s.call("server/discover", map[string]any{})
	var result struct {
		SupportedVersions []string       `json:"supportedVersions"`
		Capabilities      map[string]any `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		s.t.Fatalf("decode server/discover result: %v", err)
	}
	return result.SupportedVersions, result.Capabilities
}

// initializeLegacy performs the deprecated handshake a pre-2026-07-28 host
// still uses, and returns the negotiated version plus the server identity.
func (s *session) initializeLegacy(clientName string) (string, map[string]any) {
	s.t.Helper()
	raw := s.call("initialize", map[string]any{
		"protocolVersion": defaultProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": clientName, "version": "0"},
	})
	var result struct {
		ProtocolVersion string         `json:"protocolVersion"`
		ServerInfo      map[string]any `json:"serverInfo"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		s.t.Fatalf("decode initialize result: %v", err)
	}
	s.notify("notifications/initialized", map[string]any{})
	return result.ProtocolVersion, result.ServerInfo
}

// toolNames returns the advertised tool names and the byte size of the raw
// tools/list result — the surface budget that every phase report measures by
// hand today.
func (s *session) toolNames() ([]string, int) {
	s.t.Helper()
	raw := s.call("tools/list", map[string]any{})
	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		s.t.Fatalf("decode tools/list: %v", err)
	}
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	return names, len(raw)
}

// envelope is the runtime result contract every jacu tool returns.
type envelope struct {
	Status      string          `json:"status"`
	Summary     string          `json:"summary"`
	Data        json.RawMessage `json:"data"`
	Artifacts   []string        `json:"artifacts"`
	Warnings    []string        `json:"warnings"`
	NextActions []string        `json:"next_actions"`
	TraceID     string          `json:"trace_id"`
}

func (s *session) callTool(name string, arguments map[string]any) envelope {
	s.t.Helper()
	raw := s.call("tools/call", map[string]any{"name": name, "arguments": arguments})
	var result struct {
		StructuredContent json.RawMessage `json:"structuredContent"`
		IsError           bool            `json:"isError"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		s.t.Fatalf("decode tools/call result for %s: %v", name, err)
	}
	if len(result.StructuredContent) == 0 {
		s.t.Fatalf("%s returned no structuredContent: %s", name, raw)
	}
	var env envelope
	if err := json.Unmarshal(result.StructuredContent, &env); err != nil {
		s.t.Fatalf("decode %s envelope: %v", name, err)
	}
	if env.TraceID == "" {
		s.t.Fatalf("%s envelope has no trace_id: %s", name, result.StructuredContent)
	}
	return env
}

// data decodes the envelope payload into a generic map.
func (s *session) data(name string, env envelope) map[string]any {
	s.t.Helper()
	payload := map[string]any{}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		s.t.Fatalf("%s envelope has no data: status=%q summary=%q", name, env.Status, env.Summary)
	}
	if err := json.Unmarshal(env.Data, &payload); err != nil {
		s.t.Fatalf("decode %s data: %v", name, err)
	}
	return payload
}

func (s *session) stderrText() string {
	return s.stderr.String()
}

// close shuts the server down the way a host does — by closing stdin — and
// returns every line the server ever wrote to stdout.
func (s *session) close() []string {
	s.t.Helper()
	if err := s.stdin.Close(); err != nil {
		s.t.Fatalf("close stdin: %v", err)
	}
	select {
	case <-s.done:
	case <-time.After(30 * time.Second):
		s.t.Fatal("server did not exit after stdin closed")
	}
	if err := s.cmd.Wait(); err != nil {
		s.t.Fatalf("server exited with error: %v (stderr: %s)", err, s.stderrText())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readErr != nil {
		s.t.Fatalf("read stdout: %v", s.readErr)
	}
	return append([]string{}, s.stdoutRaw...)
}
