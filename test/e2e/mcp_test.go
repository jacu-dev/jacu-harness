//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"github.com/jacu-dev/jacu-harness/internal/testgit"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// expectedTools is the full advertised surface. It is a literal list, not a
// count: a phase that adds a tool has to come here and say which one, and a
// phase that renames one cannot pass by accident.
var expectedTools = []string{
	"jacu_apply",
	"jacu_diff",
	"jacu_discard",
	"jacu_memory_recall",
	"jacu_memory_save",
	"jacu_mission_compile",
	"jacu_project_inspect",
	"jacu_report",
	"jacu_verify",
	"jacu_flow_run",
	"jacu_status",
	"jacu_workspace_open",
	"jacu_workspace_status",
}

// maxToolsListBytes is the surface budget: a ratchet, not a physical limit.
//
// The first version of this cap was 16 KiB, on the reasoning that the whole
// catalogue should fit the same cap a single tool answer gets. That reasoning
// was wrong. PLANO.md sets 16 KiB as the inline cap for one tool *answer*;
// docs/hygiene.md asks that the catalogue size be recorded and every increase
// be matched by approved capability. Those are different things — the answer
// cap protects one call, the catalogue is sent once per session.
//
// The catalogue budget is measured on every run; E1 adds the approved report
// capability and keeps the same 20 KiB cap.
// per tool, dominated by output schemas. Descriptions are 4% of the weight, so
// trimming prose buys nothing; the repetition is the envelope, identical across
// every tool. Raising this number is allowed when a phase adds an approved tool,
// and only then — that is what makes it a ratchet and not a rubber band.
const maxToolsListBytes = 20 * 1024

// TestGovernedChangeReachesGitThroughTheShippedBinary drives one full mission
// through the built binary over stdio and then asks Git — not the envelope —
// what actually happened.
func TestGovernedChangeReachesGitThroughTheShippedBinary(t *testing.T) {
	project := newProjectRepo(t)
	baseHead := git(t, project, "rev-parse", "HEAD")

	s := startSession(t, project)
	s.useSessionlessProtocol("jacu-e2e")

	supported, _ := s.discover()
	for _, want := range []string{defaultProtocolVersion, fallbackProtocolVersion} {
		if !contains(supported, want) {
			t.Fatalf("server/discover advertises %v; want it to include %q", supported, want)
		}
	}
	assertDoctorAgreesWithTheWire(t, supported)

	names, listBytes := s.toolNames()
	assertSameTools(t, names, expectedTools)
	if listBytes > maxToolsListBytes {
		t.Fatalf("tools/list = %d bytes; budget is %d", listBytes, maxToolsListBytes)
	}
	// docs/hygiene.md asks every phase to record the tool count and the
	// serialized size of tools/list. Reporting it here puts the number on the
	// pull request instead of in a human's clipboard.
	report(t, fmt.Sprintf("MCP surface: %d tools, %d bytes (cap %d)", len(names), listBytes, maxToolsListBytes))

	inspect := s.callTool("jacu_project_inspect", map[string]any{})
	if inspect.Status != "ok" {
		t.Fatalf("inspect status = %q (%s)", inspect.Status, inspect.Summary)
	}
	projectID, _ := s.data("jacu_project_inspect", inspect)["project_id"].(string)
	if projectID == "" {
		t.Fatal("inspect returned no project_id")
	}

	missionInput := map[string]any{
		"objective": "Fix the greeting in the project readme",
		"context":   map[string]any{"project_id": projectID},
		"acceptance_criteria": []string{
			"README greets the reader",
			"Git history records the reviewed change",
			"No unrelated file changes",
		},
		"allowed_paths": []string{"README.md"},
		"risk_hint":     "write",
	}
	compile := s.callTool("jacu_mission_compile", missionInput)
	if compile.Status != "ok" {
		t.Fatalf("compile status = %q (%s)", compile.Status, compile.Summary)
	}
	missionID, _ := s.data("jacu_mission_compile", compile)["mission_id"].(string)
	if missionID == "" {
		t.Fatal("compile returned no mission_id")
	}

	open := s.callTool("jacu_workspace_open", map[string]any{
		"mission_input": missionInput,
		"mission_id":    missionID,
	})
	if open.Status != "ok" {
		t.Fatalf("open status = %q (%s)", open.Status, open.Summary)
	}
	openData := s.data("jacu_workspace_open", open)
	runID, _ := openData["run_id"].(string)
	branch, _ := openData["branch"].(string)
	worktree, _ := openData["worktree_path"].(string)
	if runID == "" || branch == "" || worktree == "" {
		t.Fatalf("open data = %#v; want run_id, branch and worktree_path", openData)
	}
	if within(worktree, project) {
		t.Fatalf("worktree %q is inside the project %q; isolation is the whole point", worktree, project)
	}

	// First gate: a run whose diff was never reviewed cannot be applied. The
	// reason is asserted, not just the status — every later gate would also
	// answer "blocked", so a test that only reads the status cannot tell which
	// door is actually locked.
	premature := s.callTool("jacu_apply", map[string]any{"run_id": runID})
	assertBlocked(t, "apply before diff", premature, "diff not reviewed")
	if head := git(t, project, "rev-parse", "HEAD"); head != baseHead {
		t.Fatalf("refused apply moved HEAD from %s to %s", baseHead, head)
	}

	writeFile(t, filepath.Join(worktree, "README.md"), "# Fixture\n\nHello, reviewer.\n")

	diff := s.callTool("jacu_diff", map[string]any{"run_id": runID})
	if diff.Status != "ok" {
		t.Fatalf("diff status = %q (%s)", diff.Status, diff.Summary)
	}
	diffData := s.data("jacu_diff", diff)
	digest, _ := diffData["digest"].(string)
	if !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("diff digest = %q; want sha256 prefix", digest)
	}
	if files, ok := diffData["files"].([]any); !ok || len(files) != 1 || files[0] != "README.md" {
		t.Fatalf("diff files = %#v; want exactly README.md", diffData["files"])
	}

	// Second gate, and the one the whole design rests on: what gets committed
	// is what was reviewed. Changing the worktree after the review has to
	// invalidate the approval.
	writeFile(t, filepath.Join(worktree, "SNEAKED.md"), "not reviewed by anyone\n")
	stale := s.callTool("jacu_apply", map[string]any{"run_id": runID})
	assertBlocked(t, "apply after the worktree changed", stale, "worktree changed after review")
	if head := git(t, project, "rev-parse", "HEAD"); head != baseHead {
		t.Fatalf("refused apply moved HEAD from %s to %s", baseHead, head)
	}
	if err := os.Remove(filepath.Join(worktree, "SNEAKED.md")); err != nil {
		t.Fatalf("remove sneaked file: %v", err)
	}
	if reviewed := s.callTool("jacu_diff", map[string]any{"run_id": runID}); reviewed.Status != "ok" {
		t.Fatalf("second diff status = %q (%s)", reviewed.Status, reviewed.Summary)
	}

	apply := s.callTool("jacu_apply", map[string]any{"run_id": runID})
	if apply.Status != "ok" {
		t.Fatalf("apply status = %q (%s)", apply.Status, apply.Summary)
	}
	applyData := s.data("jacu_apply", apply)
	commitSHA, _ := applyData["commit_sha"].(string)
	if commitSHA == "" {
		t.Fatalf("apply data = %#v; want commit_sha", applyData)
	}

	// Everything below asks Git, not the envelope.
	if head := git(t, project, "rev-parse", "HEAD"); head != baseHead {
		t.Fatalf("apply moved the checked-out branch from %s to %s; the machine must never write there", baseHead, head)
	}
	if head := git(t, project, "rev-parse", branch); head != commitSHA {
		t.Fatalf("branch %s points at %s; apply reported %s", branch, head, commitSHA)
	}
	if parent := git(t, project, "rev-parse", commitSHA+"^"); parent != baseHead {
		t.Fatalf("commit parent = %s; want the base %s", parent, baseHead)
	}
	body := git(t, project, "show", "-s", "--format=%B", commitSHA)
	for _, trailer := range []string{"Jacu-Run: " + runID, "Jacu-Mission: " + missionID, "Assisted-by: jacu-e2e"} {
		if !strings.Contains(body, trailer) {
			t.Fatalf("commit message is missing %q:\n%s", trailer, body)
		}
	}
	content := git(t, project, "show", commitSHA+":README.md")
	if !strings.Contains(content, "Hello, reviewer.") {
		t.Fatalf("committed README = %q; want the reviewed content", content)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree %q survived a successful apply (stat err = %v)", worktree, err)
	}

	status := s.callTool("jacu_status", map[string]any{})
	if status.Status != "ok" {
		t.Fatalf("status = %q (%s)", status.Status, status.Summary)
	}
	runs, _ := s.data("jacu_status", status)["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("status runs = %#v; want exactly the applied run", runs)
	}
	if entry, ok := runs[0].(map[string]any); !ok || entry["run_id"] != runID || entry["status"] != "applied" {
		t.Fatalf("status run = %#v; want %s applied", runs[0], runID)
	}

	// Memory round trip through the same process.
	save := s.callTool("jacu_memory_save", map[string]any{
		"project_id": projectID,
		"kind":       "decision",
		"title":      "Greeting lives in the readme",
		"body":       "The reviewed change put the greeting in README.md instead of a banner file.",
		"source":     "human",
	})
	if save.Status != "ok" {
		t.Fatalf("memory_save status = %q (%s)", save.Status, save.Summary)
	}
	recall := s.callTool("jacu_memory_recall", map[string]any{
		"project_id": projectID,
		"query":      "where does the greeting live",
	})
	if recall.Status != "ok" {
		t.Fatalf("memory_recall status = %q (%s)", recall.Status, recall.Summary)
	}
	results, _ := s.data("jacu_memory_recall", recall)["results"].([]any)
	if len(results) == 0 {
		t.Fatal("memory_recall returned no results for a record just saved")
	}

	lines := s.close()
	assertProtocolPureStdout(t, lines)
	if logs := s.stderrText(); !strings.Contains(logs, "capability execution") {
		t.Fatalf("stderr has no capability logs; logging may have moved to stdout:\n%s", logs)
	}
}

// TestLegacyInitializeNegotiatesTheDocumentedFallback covers the other half of
// the frozen protocol decision: a host still speaking the deprecated
// initialize handshake gets 2025-11-25, and stdout carries exactly one framed
// message for that single request. That line count is the cheapest smoke there
// is — it catches a stray print, a banner, or a log handler pointed at stdout,
// which no in-process test can see.
func TestLegacyInitializeNegotiatesTheDocumentedFallback(t *testing.T) {
	project := newProjectRepo(t)
	s := startSession(t, project)

	negotiated, serverInfo := s.initializeLegacy("jacu-e2e-smoke")
	if negotiated != fallbackProtocolVersion {
		t.Fatalf("legacy handshake negotiated %q; PLANO.md documents %q as the fallback", negotiated, fallbackProtocolVersion)
	}
	if name, _ := serverInfo["name"].(string); name != "jacu" {
		t.Fatalf("serverInfo.name = %v; want jacu", serverInfo["name"])
	}

	names, listBytes := s.toolNames()
	assertSameTools(t, names, expectedTools)
	if listBytes > maxToolsListBytes {
		t.Fatalf("tools/list = %d bytes; budget is %d", listBytes, maxToolsListBytes)
	}

	lines := s.close()
	assertProtocolPureStdout(t, lines)
	if len(lines) != 2 {
		t.Fatalf("stdout carried %d lines for initialize + tools/list:\n%s", len(lines), strings.Join(lines, "\n"))
	}
}

// assertDoctorAgreesWithTheWire ties the CLI's claim to the server's behavior.
// `jacu doctor` prints a protocol list that hosts and humans read when
// something does not connect; nothing else checks that it matches what the
// server actually advertises.
func assertDoctorAgreesWithTheWire(t *testing.T, supported []string) {
	t.Helper()
	cmd := exec.Command(serverBinary(t), "doctor")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doctor: %v: %s", err, output)
	}
	line := ""
	for _, candidate := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(candidate, "protocols ") {
			line = strings.TrimPrefix(candidate, "protocols ")
		}
	}
	if line == "" {
		t.Fatalf("doctor printed no protocols line:\n%s", output)
	}
	for _, claimed := range strings.Split(line, ", ") {
		if !contains(supported, claimed) {
			t.Fatalf("doctor claims protocol %q that the server does not advertise (%v)", claimed, supported)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// assertProtocolPureStdout requires every stdout line to be a JSON-RPC 2.0
// message. stdout is the transport; anything else on it corrupts the session.
func assertProtocolPureStdout(t *testing.T, lines []string) {
	t.Helper()
	if len(lines) == 0 {
		t.Fatal("server wrote nothing to stdout")
	}
	for index, line := range lines {
		var message map[string]any
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatalf("stdout line %d is not JSON: %q", index, line)
		}
		if message["jsonrpc"] != "2.0" {
			t.Fatalf("stdout line %d is not JSON-RPC 2.0: %q", index, line)
		}
	}
}

// assertBlocked requires both the refusal and its stated reason. Pinning the
// reason is what makes the assertion specific to one gate.
func assertBlocked(t *testing.T, what string, env envelope, wantReason string) {
	t.Helper()
	if env.Status != "blocked" {
		t.Fatalf("%s = %q (%s); want blocked", what, env.Status, env.Summary)
	}
	if !strings.Contains(env.Summary, wantReason) {
		t.Fatalf("%s was blocked by the wrong gate: %q; want a refusal mentioning %q", what, env.Summary, wantReason)
	}
}

func assertSameTools(t *testing.T, got, want []string) {
	t.Helper()
	present := map[string]bool{}
	for _, name := range got {
		present[name] = true
	}
	missing := []string{}
	for _, name := range want {
		if !present[name] {
			missing = append(missing, name)
		}
		delete(present, name)
	}
	unexpected := make([]string, 0, len(present))
	for name := range present {
		unexpected = append(unexpected, name)
	}
	if len(missing) > 0 || len(unexpected) > 0 {
		t.Fatalf("advertised tools differ: missing %v, unexpected %v", missing, unexpected)
	}
}

// newProjectRepo builds a small but realistic Go project with one commit.
func newProjectRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.name", "jacu e2e")
	git(t, root, "config", "user.email", "e2e@jacu.invalid")
	git(t, root, "config", "commit.gpgsign", "false")
	writeFile(t, filepath.Join(root, "README.md"), "# Fixture\n")
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/fixture\n\ngo 1.26\n")
	writeFile(t, filepath.Join(root, "main.go"), "package main\n\nfunc main() {}\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-m", "initial fixture")
	return root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = testgit.Env()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

// within reports whether path sits inside root.
func within(path, root string) bool {
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolvedPath = path
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		resolvedRoot = root
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
