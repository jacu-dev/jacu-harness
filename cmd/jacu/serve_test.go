package main

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func stopProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

func TestServeSpeaksOnlyJSONRPCOnStdout(t *testing.T) {
	cmd := exec.Command("go", "run", "-buildvcs=false", ".", "serve")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer stopProcess(cmd)

	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`
	if _, err := stdin.Write([]byte(init + "\n")); err != nil {
		t.Fatalf("write initialize: %v", err)
	}

	scanner := bufio.NewScanner(stdout)
	done := make(chan string, 1)
	go func() {
		if scanner.Scan() {
			done <- scanner.Text()
		}
	}()
	select {
	case line := <-done:
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("stdout não é JSON puro: %q", line)
		}
		if !strings.Contains(line, `"jsonrpc"`) {
			t.Fatalf("resposta sem jsonrpc: %q", line)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout esperando resposta initialize")
	}
}

func TestServeShutsDownCleanlyOnSIGINT(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "jacu")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, output)
	}

	cmd := exec.Command(binary, "serve")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	init := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`
	if _, err := stdin.Write([]byte(init + "\n")); err != nil {
		stopProcess(cmd)
		t.Fatalf("write initialize: %v", err)
	}

	ready := make(chan bool, 1)
	go func() {
		ready <- bufio.NewScanner(stdout).Scan()
	}()
	select {
	case ok := <-ready:
		if !ok {
			stopProcess(cmd)
			t.Fatal("serve encerrou antes de initialize")
		}
	case <-time.After(10 * time.Second):
		stopProcess(cmd)
		t.Fatal("timeout esperando initialize antes de SIGINT")
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		stopProcess(cmd)
		t.Fatalf("signal: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case waitErr := <-done:
		if code := cmd.ProcessState.ExitCode(); code != 0 {
			t.Fatalf("exit code = %d, wait error = %v; want 0", code, waitErr)
		}
	case <-time.After(2 * time.Second):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		t.Fatal("timeout esperando encerramento após SIGINT")
	}
}
