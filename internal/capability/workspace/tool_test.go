package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestWorkspaceToolCapabilitiesHaveBoundedSpecs(t *testing.T) {
	const kib = int64(1024)
	want := []struct {
		name       string
		capability capabilityruntime.Capability
		risk       capabilityruntime.RiskLevel
		readOnly   bool
		idempotent bool
		openWorld  bool
		timeout    time.Duration
		output     int64
	}{
		{"jacu_workspace_open", workspaceOpenCapability(t.TempDir()), capabilityruntime.RiskWrite, false, false, false, time.Minute, 16 * kib},
		{"jacu_status", workspaceStatusCapability(t.TempDir()), capabilityruntime.RiskSafe, true, true, false, 30 * time.Second, 16 * kib},
		{"jacu_diff", workspaceDiffCapability(t.TempDir()), capabilityruntime.RiskWrite, false, true, false, time.Minute, 32 * kib},
		{"jacu_apply", workspaceApplyCapability(t.TempDir(), "test-host"), capabilityruntime.RiskWrite, false, false, true, 10 * time.Minute, 16 * kib},
		{"jacu_discard", workspaceDiscardCapability(t.TempDir()), capabilityruntime.RiskWrite, false, false, false, 2 * time.Minute, 16 * kib},
	}
	for _, item := range want {
		t.Run(item.name, func(t *testing.T) {
			spec := item.capability.Spec
			if spec.Name != item.name || spec.Version != "1" {
				t.Fatalf("identity = %q@%q; want %q@1", spec.Name, spec.Version, item.name)
			}
			if spec.Risk != item.risk || spec.ReadOnly != item.readOnly || spec.Idempotent != item.idempotent {
				t.Fatalf("risk/readOnly/idempotent = %q/%v/%v; want %q/%v/%v", spec.Risk, spec.ReadOnly, spec.Idempotent, item.risk, item.readOnly, item.idempotent)
			}
			if spec.OpenWorld != item.openWorld {
				t.Fatalf("OpenWorld = %v; want %v", spec.OpenWorld, item.openWorld)
			}
			if spec.Timeout != item.timeout {
				t.Fatalf("Timeout = %v; want %v", spec.Timeout, item.timeout)
			}
			if spec.MaxInputBytes != 256*kib || spec.MaxOutputBytes != item.output {
				t.Fatalf("limits = %d/%d; want %d/%d", spec.MaxInputBytes, spec.MaxOutputBytes, 256*kib, item.output)
			}
			if err := spec.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}
		})
	}
}

func TestRequestHostNameHasDeterministicFallback(t *testing.T) {
	if got := requestHostName(nil); got != "unknown-mcp-client" {
		t.Fatalf("requestHostName(nil) = %q; want unknown-mcp-client", got)
	}
}

func TestCanonicalHostNameIsSafeForCommitTrailer(t *testing.T) {
	if got := canonicalHostName("Claude Code"); got != "Claude Code" {
		t.Fatalf("ordinary host = %q; want exact name", got)
	}
	got := canonicalHostName("build-agent\r\nInjected-Trailer: yes\x00")
	if strings.ContainsAny(got, "\r\n") {
		t.Fatalf("canonical host contains line break: %q", got)
	}
	for _, character := range got {
		if unicode.IsControl(character) {
			t.Fatalf("canonical host contains control %U: %q", character, got)
		}
	}
	if got != "build-agent Injected-Trailer: yes" {
		t.Fatalf("canonical host = %q; want safe single line", got)
	}
	if got := canonicalHostName(" \r\n\t "); got != "unknown-mcp-client" {
		t.Fatalf("empty canonical host = %q; want fallback", got)
	}
	if got := canonicalHostName("Agent\t\u2028\u2029\u200bName"); got != "Agent Name" {
		t.Fatalf("unicode separator canonical host = %q; want Agent Name", got)
	}
}

func TestCanonicalHostNameLimitsMultibyteNameTo128Runes(t *testing.T) {
	got := canonicalHostName(strings.Repeat("界", 140))
	if !utf8.ValidString(got) {
		t.Fatalf("canonical host is invalid UTF-8: %q", got)
	}
	if runeCount := utf8.RuneCountInString(got); runeCount != 128 {
		t.Fatalf("canonical host runes = %d; want 128", runeCount)
	}
	if got != strings.Repeat("界", 128) {
		t.Fatalf("canonical host was not truncated deterministically: %q", got)
	}
}

func TestCanonicalHostNameControlsOnlyUsesDefault(t *testing.T) {
	got := canonicalHostName("\x00\t\r\n\u0085\u2028\u2029\u200b")
	if got != defaultHostName {
		t.Fatalf("controls-only canonical host = %q; want %q", got, defaultHostName)
	}
}

func TestWorkspaceDiffMCPTruncatesOnlyEncodedDiffAndPreservesReviewedMetadata(t *testing.T) {
	repo, opened := openFixture(t)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	writeWorktreeFile(t, opened.WorktreePath, "README.md", strings.Repeat("<>&\"\\\té界\n", 3000))
	fullDiff := workspaceFullDiff(t, opened)
	if len(fullDiff) <= maxInlineDiffBytes || !strings.Contains(fullDiff, "é界") {
		t.Fatalf("large fixture does not exercise raw and multibyte boundaries: bytes=%d", len(fullDiff))
	}

	output, encoded := callWorkspaceDiffMCP(t, repo, opened.RunID)
	maxOutput := workspaceDiffCapability(repo).Spec.MaxOutputBytes
	if int64(len(encoded)) > maxOutput {
		t.Fatalf("encoded structured output = %d bytes; max = %d", len(encoded), maxOutput)
	}
	wantDigest := diffDigest(fullDiff)
	if output.Status != "ok" || output.Summary != "Workspace diff reviewed." {
		t.Fatalf("status/summary = %q/%q; want reviewed success", output.Status, output.Summary)
	}
	if output.Data.Digest != wantDigest {
		t.Fatalf("review digest = %q; want %q", output.Data.Digest, wantDigest)
	}
	if !containsString(output.Data.Files, "README.md") || !containsString(output.Data.InScope, "README.md") || len(output.Data.OutOfScope) != 0 {
		t.Fatalf("structured paths lost: files=%v in_scope=%v out_of_scope=%v", output.Data.Files, output.Data.InScope, output.Data.OutOfScope)
	}
	if output.Data.Added == 0 || output.Data.Deleted == 0 {
		t.Fatalf("structured counts lost: added=%d deleted=%d", output.Data.Added, output.Data.Deleted)
	}
	if output.Data.Diff == "" || output.Data.Diff == fullDiff || !strings.Contains(output.Data.Diff, "diff truncated") ||
		!strings.Contains(output.Data.Diff, "é界") || !utf8.ValidString(output.Data.Diff) {
		t.Fatalf("diff was not selectively truncated: bytes=%d", len(output.Data.Diff))
	}
	if !containsString(output.Warnings, "diff truncated to fit 32KB encoded output limit") {
		t.Fatalf("encoded-output truncation warning missing: %v", output.Warnings)
	}
	if !strings.HasPrefix(output.TraceID, "tr_") || output.Artifacts == nil || output.NextActions == nil {
		t.Fatalf("envelope metadata lost: trace=%q artifacts=%v next_actions=%v", output.TraceID, output.Artifacts, output.NextActions)
	}

	run, err := runstate.Load(repo, opened.RunID)
	if err != nil {
		t.Fatalf("Load reviewed run: %v", err)
	}
	if run.Status != runstate.StatusReviewed || run.ReviewedDigest != output.Data.Digest || run.ReviewedAt.IsZero() {
		t.Fatalf("persisted review disagrees with output: status=%q digest=%q reviewed_at=%v output_digest=%q", run.Status, run.ReviewedDigest, run.ReviewedAt, output.Data.Digest)
	}
}

func TestWorkspaceDiffMCPSmallOutputRemainsUnchanged(t *testing.T) {
	repo, opened := openFixture(t)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	writeWorktreeFile(t, opened.WorktreePath, "README.md", "<small> \"quoted\" \\ café 界\n")
	fullDiff := workspaceFullDiff(t, opened)

	output, encoded := callWorkspaceDiffMCP(t, repo, opened.RunID)
	if int64(len(encoded)) > workspaceDiffCapability(repo).Spec.MaxOutputBytes {
		t.Fatalf("small encoded structured output = %d bytes", len(encoded))
	}
	if output.Status != "ok" || output.Summary != "Workspace diff reviewed." {
		t.Fatalf("status/summary = %q/%q; want reviewed success", output.Status, output.Summary)
	}
	if output.Data.Digest != diffDigest(fullDiff) {
		t.Fatalf("small review digest = %q", output.Data.Digest)
	}
	if output.Data.Diff != fullDiff {
		t.Fatalf("small diff changed:\ngot:  %q\nwant: %q", output.Data.Diff, fullDiff)
	}
	if containsString(output.Warnings, "diff truncated to fit 32KB encoded output limit") || strings.Contains(output.Data.Diff, "diff truncated") {
		t.Fatalf("small diff reported truncation: warnings=%v", output.Warnings)
	}
	if !strings.HasPrefix(output.TraceID, "tr_") || output.Artifacts == nil || output.NextActions == nil {
		t.Fatalf("small envelope metadata lost: trace=%q artifacts=%v next_actions=%v", output.TraceID, output.Artifacts, output.NextActions)
	}

	run, err := runstate.Load(repo, opened.RunID)
	if err != nil {
		t.Fatalf("Load reviewed run: %v", err)
	}
	if run.Status != runstate.StatusReviewed || run.ReviewedDigest != output.Data.Digest {
		t.Fatalf("small persisted review disagrees with output: status=%q digest=%q output_digest=%q", run.Status, run.ReviewedDigest, output.Data.Digest)
	}
}

func TestWorkspaceDiffMCPRejectsUnrepresentableMetadataBeforeReview(t *testing.T) {
	repo, opened := openFixture(t)
	defer cleanupWorktree(t, repo, opened.WorktreePath)
	for index := 0; index < 250; index++ {
		name := fmt.Sprintf("unexpected/path-%03d-with-long-review-metadata-name.txt", index)
		writeWorktreeFile(t, opened.WorktreePath, name, "changed\n")
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "diff-metadata-limit-test", Version: "1"}, nil)
	RegisterTool(server, repo)
	session, ctx := connectWorkspaceTestServer(t, server)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: DiffToolName, Arguments: map[string]any{"run_id": opened.RunID}})
	if err != nil {
		t.Fatalf("CallTool jacu_diff: %v", err)
	}
	if result.IsError {
		t.Fatalf("metadata-only overflow changed MCP error semantics: %v", result.GetError())
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal rejected review output: %v", err)
	}
	var output envelope[mcpDiffOutputData]
	if unmarshalErr := json.Unmarshal(encoded, &output); unmarshalErr != nil {
		t.Fatalf("decode rejected review output: %v", unmarshalErr)
	}
	if output.Status != "failed" || output.Summary == "Workspace diff reviewed." {
		t.Fatalf("metadata-only overflow claimed review: status=%q summary=%q", output.Status, output.Summary)
	}

	run, err := runstate.Load(repo, opened.RunID)
	if err != nil {
		t.Fatalf("Load run after rejected review: %v", err)
	}
	if run.Status != runstate.StatusOpen || run.ReviewedDigest != "" || !run.ReviewedAt.IsZero() {
		t.Fatalf("unrepresentable review was persisted: status=%q digest=%q reviewed_at=%v", run.Status, run.ReviewedDigest, run.ReviewedAt)
	}
}

func TestAllRegisteredWorkspaceToolsUseSharedOperationGate(t *testing.T) {
	gate := newObservedOperationGate()
	server := mcp.NewServer(&mcp.Implementation{Name: "gate-test", Version: "1"}, nil)
	registerWorkspaceTools(server, t.TempDir(), gate)
	session, ctx := connectWorkspaceTestServer(t, server)

	calls := []struct {
		name      string
		arguments map[string]any
	}{
		{WorkspaceOpenToolName, map[string]any{
			"mission_input": map[string]any{"objective": "Invalid gate fixture"},
			"mission_id":    "msn_invalid",
		}},
		{WorkspaceStatusToolName, map[string]any{}},
		{DiffToolName, map[string]any{"run_id": "run_0000000000000000"}},
		{ApplyToolName, map[string]any{"run_id": "run_0000000000000000"}},
		{DiscardToolName, map[string]any{"run_id": "run_0000000000000000"}},
	}
	for _, call := range calls {
		done := callWorkspaceTestTool(session, ctx, call.name, call.arguments)
		awaitSignal(t, ctx, gate.attempts, call.name+" gate attempt")
		awaitSignal(t, ctx, gate.acquired, call.name+" gate acquisition")
		if err := awaitSignal(t, ctx, done, call.name+" MCP result"); err != nil {
			t.Fatalf("call %s: %v", call.name, err)
		}
	}
}

func TestWorkspaceOperationGateSerializesConcurrentMCPCalls(t *testing.T) {
	gate := newObservedOperationGate()
	releaseFirst := make(chan struct{})
	events := make(chan string, 3)
	first := gateTestCapability("jacu_gate_first", func(ctx context.Context) (capabilityruntime.Result, error) {
		events <- "first-enter"
		select {
		case <-releaseFirst:
		case <-ctx.Done():
			return capabilityruntime.Result{}, ctx.Err()
		}
		events <- "first-exit"
		return gateTestResult("first"), nil
	})
	second := gateTestCapability("jacu_gate_second", func(context.Context) (capabilityruntime.Result, error) {
		events <- "second-enter"
		return gateTestResult("second"), nil
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "gate-test", Version: "1"}, nil)
	registerGateTestTool(server, gate, first)
	registerGateTestTool(server, gate, second)
	session, ctx := connectWorkspaceTestServer(t, server)

	firstDone := callGateTestTool(session, ctx, first.Spec.Name)
	awaitSignal(t, ctx, gate.attempts, "first gate attempt")
	awaitSignal(t, ctx, gate.acquired, "first gate acquisition")
	if event := awaitSignal(t, ctx, events, "first capability entry"); event != "first-enter" {
		t.Fatalf("first event = %q; want first-enter", event)
	}

	secondDone := callGateTestTool(session, ctx, second.Spec.Name)
	awaitSignal(t, ctx, gate.attempts, "second gate attempt")
	select {
	case <-gate.acquired:
		t.Fatal("second operation acquired gate while first held it")
	default:
	}

	close(releaseFirst)
	if event := awaitSignal(t, ctx, events, "first capability exit"); event != "first-exit" {
		t.Fatalf("event after release = %q; want first-exit", event)
	}
	awaitSignal(t, ctx, gate.acquired, "second gate acquisition after first exit")
	if event := awaitSignal(t, ctx, events, "second capability entry"); event != "second-enter" {
		t.Fatalf("event after first exit = %q; want second-enter", event)
	}
	if err := awaitSignal(t, ctx, firstDone, "first MCP result"); err != nil {
		t.Fatalf("first MCP call: %v", err)
	}
	if err := awaitSignal(t, ctx, secondDone, "second MCP result"); err != nil {
		t.Fatalf("second MCP call: %v", err)
	}
}

func TestWorkspaceOperationGateHonorsCapabilityTimeoutWithoutTokenLeak(t *testing.T) {
	gate := newObservedOperationGate()
	releaseFirst := make(chan struct{})
	firstReleased := false
	defer func() {
		if !firstReleased {
			close(releaseFirst)
		}
	}()
	firstEntered := make(chan struct{}, 1)
	secondEntered := make(chan struct{}, 1)
	thirdEntered := make(chan struct{}, 1)
	first := gateTestCapability("jacu_gate_timeout_first", func(ctx context.Context) (capabilityruntime.Result, error) {
		firstEntered <- struct{}{}
		select {
		case <-releaseFirst:
		case <-ctx.Done():
			return capabilityruntime.Result{}, ctx.Err()
		}
		return gateTestResult("first"), nil
	})
	second := gateTestCapability("jacu_gate_timeout_second", func(context.Context) (capabilityruntime.Result, error) {
		secondEntered <- struct{}{}
		return gateTestResult("second"), nil
	})
	second.Spec.Timeout = 25 * time.Millisecond
	third := gateTestCapability("jacu_gate_timeout_third", func(context.Context) (capabilityruntime.Result, error) {
		thirdEntered <- struct{}{}
		return gateTestResult("third"), nil
	})

	server := mcp.NewServer(&mcp.Implementation{Name: "gate-timeout-test", Version: "1"}, nil)
	registerGateTestTool(server, gate, first)
	registerGateTestTool(server, gate, second)
	registerGateTestTool(server, gate, third)
	session, ctx := connectWorkspaceTestServer(t, server)

	firstDone := callGateTestTool(session, ctx, first.Spec.Name)
	awaitSignal(t, ctx, gate.attempts, "first timeout-test gate attempt")
	awaitSignal(t, ctx, gate.acquired, "first timeout-test gate acquisition")
	awaitSignal(t, ctx, firstEntered, "first timeout-test capability entry")

	secondDone := callGateTestTool(session, ctx, second.Spec.Name)
	awaitSignal(t, ctx, gate.attempts, "second timeout-test gate attempt")
	guardCtx, cancelGuard := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancelGuard()
	if err := awaitSignal(t, guardCtx, gate.rejected, "second gate rejection at capability timeout"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second gate rejection = %v; want context deadline exceeded", err)
	}
	if err := awaitSignal(t, ctx, secondDone, "second call to honor capability timeout"); err == nil {
		t.Fatal("second MCP call succeeded after its capability timeout")
	}
	select {
	case <-gate.acquired:
		t.Fatal("timed-out second operation acquired the gate")
	default:
	}
	select {
	case <-secondEntered:
		t.Fatal("timed-out second operation executed runtime")
	default:
	}

	close(releaseFirst)
	firstReleased = true
	if err := awaitSignal(t, ctx, firstDone, "first timeout-test MCP result"); err != nil {
		t.Fatalf("first MCP call: %v", err)
	}

	thirdDone := callGateTestTool(session, ctx, third.Spec.Name)
	awaitSignal(t, ctx, gate.attempts, "third gate attempt after timeout")
	awaitSignal(t, ctx, gate.acquired, "third gate acquisition after timeout")
	awaitSignal(t, ctx, thirdEntered, "third capability entry after timeout")
	if err := awaitSignal(t, ctx, thirdDone, "third MCP result after timeout"); err != nil {
		t.Fatalf("third MCP call: %v", err)
	}
}

func TestWorkspaceOperationGatesAreIndependentAcrossMCPServers(t *testing.T) {
	gateA := newWorkspaceOperationGate()
	gateB := newWorkspaceOperationGate()
	releaseA := make(chan struct{})
	enteredA := make(chan struct{}, 1)
	enteredB := make(chan struct{}, 1)
	capabilityA := gateTestCapability("jacu_gate_root_a", func(ctx context.Context) (capabilityruntime.Result, error) {
		enteredA <- struct{}{}
		select {
		case <-releaseA:
		case <-ctx.Done():
			return capabilityruntime.Result{}, ctx.Err()
		}
		return gateTestResult("root-a"), nil
	})
	capabilityB := gateTestCapability("jacu_gate_root_b", func(context.Context) (capabilityruntime.Result, error) {
		enteredB <- struct{}{}
		return gateTestResult("root-b"), nil
	})

	serverA := mcp.NewServer(&mcp.Implementation{Name: "gate-a", Version: "1"}, nil)
	serverB := mcp.NewServer(&mcp.Implementation{Name: "gate-b", Version: "1"}, nil)
	registerGateTestTool(serverA, gateA, capabilityA)
	registerGateTestTool(serverB, gateB, capabilityB)
	sessionA, ctxA := connectWorkspaceTestServer(t, serverA)
	sessionB, ctxB := connectWorkspaceTestServer(t, serverB)

	doneA := callGateTestTool(sessionA, ctxA, capabilityA.Spec.Name)
	awaitSignal(t, ctxA, enteredA, "root A capability entry")
	doneB := callGateTestTool(sessionB, ctxB, capabilityB.Spec.Name)
	awaitSignal(t, ctxB, enteredB, "root B capability entry while root A blocked")
	if err := awaitSignal(t, ctxB, doneB, "root B MCP result"); err != nil {
		t.Fatalf("root B MCP call: %v", err)
	}
	close(releaseA)
	if err := awaitSignal(t, ctxA, doneA, "root A MCP result"); err != nil {
		t.Fatalf("root A MCP call: %v", err)
	}
}

type observedOperationGate struct {
	gate     workspaceOperationGate
	attempts chan struct{}
	acquired chan struct{}
	rejected chan error
}

func newObservedOperationGate() *observedOperationGate {
	return &observedOperationGate{
		gate: newWorkspaceOperationGate(), attempts: make(chan struct{}),
		acquired: make(chan struct{}), rejected: make(chan error),
	}
}

func (gate *observedOperationGate) acquire(ctx context.Context) error {
	gate.attempts <- struct{}{}
	if err := gate.gate.acquire(ctx); err != nil {
		gate.rejected <- err
		return err
	}
	gate.acquired <- struct{}{}
	return nil
}

func (gate *observedOperationGate) release() {
	gate.gate.release()
}

type gateTestInput struct{}

type mcpDiffOutputData struct {
	Digest     string   `json:"digest"`
	Files      []string `json:"files"`
	Added      int      `json:"added"`
	Deleted    int      `json:"deleted"`
	InScope    []string `json:"in_scope"`
	OutOfScope []string `json:"out_of_scope"`
	Diff       string   `json:"diff"`
}

type gateTestData struct {
	Value string `json:"value"`
}

func gateTestCapability(name string, handler func(context.Context) (capabilityruntime.Result, error)) capabilityruntime.Capability {
	return capabilityruntime.Capability{
		Spec: workspaceSpec(name, capabilityruntime.RiskWrite, false, false, false, 5*time.Second, 4*1024),
		Handler: func(ctx context.Context, _ json.RawMessage) (capabilityruntime.Result, error) {
			return handler(ctx)
		},
	}
}

func gateTestResult(value string) capabilityruntime.Result {
	return capabilityruntime.Result{
		Status: "ok", Summary: "gate test completed", Data: gateTestData{Value: value},
		Artifacts: []string{}, Warnings: []string{}, NextActions: []string{},
	}
}

func registerGateTestTool(server *mcp.Server, gate workspaceOperationGate, capability capabilityruntime.Capability) {
	mcp.AddTool(server, workspaceTool(capability.Spec.Name, "Exercise the workspace operation gate.", false, false, false, false),
		func(ctx context.Context, _ *mcp.CallToolRequest, input gateTestInput) (*mcp.CallToolResult, envelope[gateTestData], error) {
			return executeTyped[gateTestInput, gateTestData](ctx, gate, capability, input)
		})
}

func connectWorkspaceTestServer(t *testing.T, server *mcp.Server) (*mcp.ClientSession, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	runDone := make(chan error, 1)
	go func() { runDone <- server.Run(ctx, serverTransport) }()
	var session *mcp.ClientSession
	t.Cleanup(func() {
		cancel()
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), time.Second)
		defer cancelCleanup()

		var closeDone <-chan error
		if session != nil {
			connectionResult := make(chan error, 1)
			go func() { connectionResult <- session.Close() }()
			closeDone = connectionResult
		}
		serverDone := (<-chan error)(runDone)
		var cleanupErrors []error
		for closeDone != nil || serverDone != nil {
			select {
			case closeErr := <-closeDone:
				if closeErr != nil {
					cleanupErrors = append(cleanupErrors, closeErr)
				}
				closeDone = nil
			case runErr := <-serverDone:
				if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, context.DeadlineExceeded) {
					cleanupErrors = append(cleanupErrors, runErr)
				}
				serverDone = nil
			case <-cleanupCtx.Done():
				if closeDone != nil {
					cleanupErrors = append(cleanupErrors, fmt.Errorf("timed out waiting for workspace MCP connection close: %w", cleanupCtx.Err()))
				}
				if serverDone != nil {
					cleanupErrors = append(cleanupErrors, fmt.Errorf("timed out waiting for workspace MCP server Run: %w", cleanupCtx.Err()))
				}
				closeDone = nil
				serverDone = nil
			}
		}
		if err := errors.Join(cleanupErrors...); err != nil {
			t.Errorf("workspace MCP server cleanup: %v", err)
		}
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "gate-client", Version: "1"}, nil)
	var err error
	session, err = client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect gate test server: %v", err)
	}
	return session, ctx
}

func callGateTestTool(session *mcp.ClientSession, ctx context.Context, name string) <-chan error {
	return callWorkspaceTestTool(session, ctx, name, map[string]any{})
}

func callWorkspaceDiffMCP(t *testing.T, repo, runID string) (envelope[mcpDiffOutputData], []byte) {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "diff-output-test", Version: "1"}, nil)
	RegisterTool(server, repo)
	session, ctx := connectWorkspaceTestServer(t, server)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: DiffToolName, Arguments: map[string]any{"run_id": runID}})
	if err != nil {
		t.Fatalf("CallTool jacu_diff: %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool jacu_diff returned MCP error: %v", result.GetError())
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured jacu_diff output: %v", err)
	}
	var output envelope[mcpDiffOutputData]
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("decode structured jacu_diff output: %v", err)
	}
	return output, encoded
}

func workspaceFullDiff(t *testing.T, opened OpenData) string {
	t.Helper()
	git, err := gitx.New()
	if err != nil {
		t.Fatalf("gitx.New: %v", err)
	}
	fullDiff, err := git.Diff(context.Background(), opened.WorktreePath, opened.BaseSHA)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	return fullDiff
}

func callWorkspaceTestTool(session *mcp.ClientSession, ctx context.Context, name string, arguments map[string]any) <-chan error {
	done := make(chan error, 1)
	go func() {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
		if err == nil && result.IsError {
			err = fmt.Errorf("tool returned transport error")
		}
		done <- err
	}()
	return done
}

func awaitSignal[T any](t *testing.T, ctx context.Context, signal <-chan T, label string) T {
	t.Helper()
	select {
	case value := <-signal:
		return value
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s: %v", label, ctx.Err())
		var zero T
		return zero
	}
}
