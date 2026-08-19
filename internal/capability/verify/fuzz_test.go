package verify

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

// FuzzAllowlistCheck asserts the inverse property, which is the one that
// matters: whatever the allowlist lets through must satisfy every invariant the
// rejection order claims to enforce. A rule that is merely present but
// unreachable — the shape the previous product shipped — fails this.
func FuzzAllowlistCheck(f *testing.F) {
	f.Add("go\x00test\x00./...")
	f.Add("sh\x00-c\x00rm -rf /")
	f.Add("\x00")
	f.Add("go\x00test\x00../../escape")
	f.Add("go\x00build\x00-o\x00/tmp/x")
	f.Add("rm\x00-r\x00--force\x00/")
	f.Add("gо\x00test") // Cyrillic о — a program name that only looks like "go"
	f.Add(strings.Repeat("a\x00", 10000))

	allowlist := New(Config{})
	f.Fuzz(func(t *testing.T, encoded string) {
		argv := strings.Split(encoded, "\x00")
		err := allowlist.Check(argv)
		if err != nil {
			return
		}
		// Allowed. Every invariant must hold.
		if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
			t.Fatalf("allowed an empty program: %q", argv)
		}
		if strings.ContainsAny(argv[0], `/\`) {
			t.Fatalf("allowed a program path: %q", argv)
		}
		if _, isShell := shellPrograms[strings.ToLower(argv[0])]; isShell {
			t.Fatalf("allowed a shell: %q", argv)
		}
		for _, arg := range argv {
			if containsShellMeta(arg) {
				t.Fatalf("allowed a metachar: %q", argv)
			}
		}
		for _, arg := range argv[1:] {
			if hasParentSegment(arg) {
				t.Fatalf("allowed a traversal: %q", argv)
			}
			if isAbsolutePath(arg) {
				t.Fatalf("allowed an absolute path: %q", argv)
			}
		}
		if isRecursiveForcedRemoval(argv[0], argv[1:]) {
			t.Fatalf("allowed a recursive forced removal: %q", argv)
		}
		matched := false
		for _, entry := range allowlist.entries {
			if entry.matches(argv[0], argv[1:]) {
				matched = true
			}
		}
		if !matched {
			t.Fatalf("allowed a command no entry matches: %q", argv)
		}
	})
}

// FuzzRunCommandArgv drives the whole capability with hostile argv. The
// property is that a refusal never becomes an execution: a blocked result must
// carry no exit code, because an exit code only exists when a process ran.
func FuzzRunCommandArgv(f *testing.F) {
	f.Add("printf\x00hello")
	f.Add("sh\x00-c\x00touch pwned")
	f.Add("curl\x00https://example.invalid")
	f.Add("")
	f.Add("touch\x00/tmp/pwned")

	f.Fuzz(func(t *testing.T, encoded string) {
		root, _ := seedRun(t, "run_abcdefabcdefabcd", nil, runstate.StatusOpen)
		result := RunCommand(context.Background(), root, RunCommandInput{
			RunID: "run_abcdefabcdefabcd",
			ArgV:  strings.Split(encoded, "\x00"),
		})
		if result.Status != "ok" && result.Status != "blocked" {
			t.Fatalf("status = %q; the envelope only has these two here", result.Status)
		}
		if result.Status == "blocked" {
			if result.Data.Verdict != VerdictBlocked {
				t.Fatalf("blocked status with verdict %q", result.Data.Verdict)
			}
			for _, command := range result.Data.Commands {
				if command.ExitCode != nil {
					t.Fatalf("a refused command reported exit code %d; it must never have run", *command.ExitCode)
				}
			}
		}
		if _, err := json.Marshal(result.Data); err != nil {
			t.Fatalf("result does not encode: %v", err)
		}
	})
}

// FuzzVerifyRunID hits the load path with arbitrary identifiers.
func FuzzVerifyRunID(f *testing.F) {
	f.Add("run_abcdefabcdefabcd")
	f.Add("../../etc/passwd")
	f.Add("")
	f.Add("run_" + strings.Repeat("f", 4096))

	f.Fuzz(func(t *testing.T, runID string) {
		root, _ := seedRun(t, "run_abcdefabcdefabcd", nil, runstate.StatusOpen)
		result := Verify(context.Background(), root, Input{RunID: runID})
		if result.Status != "ok" && result.Status != "blocked" {
			t.Fatalf("status = %q", result.Status)
		}
		if result.Data.EvidenceDigest == "" {
			t.Fatal("every answer carries an evidence digest, even a refusal")
		}
	})
}
