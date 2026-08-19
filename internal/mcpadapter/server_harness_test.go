package mcpadapter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPTestRunHarnessCancelsJoinsAndIsIdempotent(t *testing.T) {
	started := make(chan struct{})
	exited := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	harness := startMCPTestRunHarness(func(ctx context.Context) error {
		close(started)
		defer close(exited)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return nil
		}
	})
	awaitHarnessSignal(t, started, "server run start")

	if err := harness.shutdown(); err != nil {
		t.Fatalf("first shutdown: %v", err)
	}
	select {
	case <-exited:
	default:
		releaseOnce.Do(func() { close(release) })
		awaitHarnessSignal(t, exited, "legacy server run release")
		t.Fatal("shutdown returned before server Run exited")
	}
	if err := harness.shutdown(); err != nil {
		t.Fatalf("idempotent shutdown: %v", err)
	}
	releaseOnce.Do(func() { close(release) })
}

func TestMCPTestRunHarnessReturnsUnexpectedRunError(t *testing.T) {
	wantErr := errors.New("server run failed")
	returned := make(chan struct{})
	harness := startMCPTestRunHarness(func(context.Context) error {
		defer close(returned)
		return wantErr
	})
	awaitHarnessSignal(t, returned, "server run failure")

	if err := harness.shutdown(); !errors.Is(err, wantErr) {
		t.Fatalf("shutdown error = %v; want %v", err, wantErr)
	}
}

func TestMCPTestRunHarnessCancelsBeforeClosingConnection(t *testing.T) {
	harness := startMCPTestRunHarness(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	harness.closeConnection = func() error {
		if err := harness.ctx.Err(); !errors.Is(err, context.Canceled) {
			return fmt.Errorf("connection close started before cancellation: context error = %v", err)
		}
		return nil
	}

	if err := harness.shutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestMCPTestRunHarnessUsesOneDeadlineForBlockedCloseAndJoin(t *testing.T) {
	closeStarted := make(chan struct{})
	closeExited := make(chan struct{})
	releaseClose := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseClose) }) }
	harness := startMCPTestRunHarness(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})
	harness.closeConnection = func() error {
		close(closeStarted)
		defer close(closeExited)
		<-releaseClose
		return nil
	}

	shutdownDone := make(chan error, 1)
	t.Cleanup(func() {
		release()
		awaitHarnessSignal(t, closeStarted, "connection close cleanup start")
		awaitHarnessSignal(t, closeExited, "connection close cleanup")
	})
	go func() { shutdownDone <- harness.shutdown() }()
	awaitHarnessSignal(t, closeStarted, "connection close start")
	guardCtx, cancelGuard := context.WithTimeout(context.Background(), 2*mcpTestRunJoinTimeout)
	defer cancelGuard()
	var shutdownErr error
	select {
	case shutdownErr = <-shutdownDone:
	case <-guardCtx.Done():
		release()
		awaitHarnessSignal(t, closeExited, "blocked connection close release")
		<-shutdownDone
		t.Fatalf("shutdown exceeded shared deadline: %v", guardCtx.Err())
	}
	if shutdownErr == nil || !strings.Contains(shutdownErr.Error(), "timed out waiting for MCP connection close") {
		t.Fatalf("shutdown error = %v; want connection close timeout", shutdownErr)
	}
	if strings.Contains(shutdownErr.Error(), "server Run") {
		t.Fatalf("server Run consumed a second deadline: %v", shutdownErr)
	}
	release()
	awaitHarnessSignal(t, closeExited, "timed-out connection close exit")
}

func TestMCPTestRunHarnessTreatsWrappedCancellationAsNormal(t *testing.T) {
	harness := startMCPTestRunHarness(func(ctx context.Context) error {
		<-ctx.Done()
		return fmt.Errorf("server stopped: %w", ctx.Err())
	})
	if err := harness.shutdown(); err != nil {
		t.Fatalf("wrapped cancellation = %v; want normal shutdown", err)
	}
}

const mcpTestRunJoinTimeout = time.Second

type mcpTestRunHarness struct {
	ctx             context.Context
	cancel          context.CancelFunc
	runDone         <-chan error
	closeConnection func() error
	shutdownOnce    sync.Once
	shutdownErr     error
}

func startMCPTestRunHarness(run func(context.Context) error) *mcpTestRunHarness {
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- run(ctx) }()
	return &mcpTestRunHarness{ctx: ctx, cancel: cancel, runDone: runDone}
}

func (harness *mcpTestRunHarness) shutdown() error {
	harness.shutdownOnce.Do(func() {
		harness.cancel()
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), mcpTestRunJoinTimeout)
		defer cancelShutdown()

		var closeDone <-chan error
		if harness.closeConnection != nil {
			connectionResult := make(chan error, 1)
			go func() { connectionResult <- harness.closeConnection() }()
			closeDone = connectionResult
		}
		runDone := harness.runDone
		var shutdownErrors []error
		for closeDone != nil || runDone != nil {
			select {
			case closeErr := <-closeDone:
				if closeErr != nil {
					shutdownErrors = append(shutdownErrors, closeErr)
				}
				closeDone = nil
			case runErr := <-runDone:
				if runErr != nil && !errors.Is(runErr, context.Canceled) {
					shutdownErrors = append(shutdownErrors, runErr)
				}
				runDone = nil
			case <-shutdownCtx.Done():
				if closeDone != nil {
					shutdownErrors = append(shutdownErrors, fmt.Errorf("timed out waiting for MCP connection close: %w", shutdownCtx.Err()))
				}
				if runDone != nil {
					shutdownErrors = append(shutdownErrors, fmt.Errorf("timed out waiting for MCP server Run: %w", shutdownCtx.Err()))
				}
				closeDone = nil
				runDone = nil
			}
		}
		harness.shutdownErr = errors.Join(shutdownErrors...)
	})
	return harness.shutdownErr
}

func connectMCPTestServer(t *testing.T, server *mcp.Server, clientName string) (*mcp.ClientSession, context.Context) {
	t.Helper()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	harness := startMCPTestRunHarness(func(ctx context.Context) error {
		return server.Run(ctx, serverTransport)
	})
	t.Cleanup(func() {
		if err := harness.shutdown(); err != nil {
			t.Errorf("MCP server cleanup: %v", err)
		}
	})

	client := mcp.NewClient(&mcp.Implementation{Name: clientName, Version: "0"}, nil)
	session, err := client.Connect(harness.ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	harness.closeConnection = session.Close
	return session, harness.ctx
}

func awaitHarnessSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s: %v", label, ctx.Err())
	}
}
