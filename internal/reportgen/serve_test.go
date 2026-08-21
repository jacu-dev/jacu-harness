package reportgen

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/report"
)

func TestServeBindsLoopbackAndWritesDecisions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "plan.report.json")
	encoded, err := json.Marshal(sampleReport())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Serve(ctx, ServeOptions{InputPath: path, Addr: "127.0.0.1:0", Ready: ready})
	}()
	var addr string
	select {
	case addr = <-ready:
	case err := <-errCh:
		t.Fatalf("serve: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not become ready")
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("bind host = %q", host)
	}
	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("viewer status = %d", resp.StatusCode)
	}
	body, _ := json.Marshal(DecisionWrite{ID: "d1", Answer: "yes"})
	post, err := http.Post("http://"+addr+"/decision", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer post.Body.Close()
	if post.StatusCode != http.StatusNoContent {
		t.Fatalf("decision status = %d", post.StatusCode)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Blocks.Decision[0].Answer != "yes" {
		t.Fatalf("answer = %#v", loaded.Blocks.Decision)
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serve exit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("serve did not exit")
	}
}

func TestServeRefusesNonLoopbackAndAuditWrites(t *testing.T) {
	if err := Serve(context.Background(), ServeOptions{Addr: "0.0.0.0:0"}); err == nil {
		t.Fatal("0.0.0.0 was accepted")
	}
	path := filepath.Join(t.TempDir(), "audit.report.json")
	doc := sampleReport()
	doc.Kind = report.KindAudit
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 1)
	go func() { _ = Serve(ctx, ServeOptions{InputPath: path, Addr: "127.0.0.1:0", Ready: ready}) }()
	addr := <-ready
	body, _ := json.Marshal(DecisionWrite{ID: "d1", Answer: "yes"})
	post, err := http.Post("http://"+addr+"/decision", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer post.Body.Close()
	if post.StatusCode != http.StatusForbidden {
		t.Fatalf("audit write status = %d", post.StatusCode)
	}
}
