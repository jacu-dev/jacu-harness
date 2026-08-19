// Package verify governs command execution for a mission: which argv may run,
// and under which limits. Deny-by-default, and the policy never comes from the
// request being judged.
package verify

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Entry authorizes a program and, optionally, a required prefix of arguments.
// The prefix authorizes a prefix, not the whole line: {go, [test]} allows
// `go test ./... -race` and does not allow `go build`.
type Entry struct {
	Program           string   `json:"program"`
	RequiredArgPrefix []string `json:"required_arg_prefix,omitempty"`
}

// Config is the per-project policy, read from the project root.
type Config struct {
	Allow    []Entry  `json:"allow,omitempty"`
	Deny     []string `json:"deny,omitempty"`
	PathDirs []string `json:"path_dirs,omitempty"`
}

// ConfigPath is where a project declares its policy. It is read from the
// project root and never from a run worktree: the worktree is writable by the
// model, and a policy the governed object can edit is not a policy.
const ConfigPath = ".jacu/verify-allowlist.json"

// LoadConfig reads the project policy. An absent file is normal and means the
// global list applies alone; a malformed file is an error, because a broken
// policy must not degrade into no policy.
func LoadConfig(root string) (Config, error) {
	path := filepath.Join(root, ConfigPath)
	content, err := os.ReadFile(path) // #nosec G304 -- path is derived from the project root, not from input.
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var config Config
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode %s: %w", ConfigPath, err)
	}
	return config, nil
}

// Allowlist is the effective policy: the curated global list plus the project
// allowances, minus the project denials.
type Allowlist struct {
	entries []Entry
}

// New composes the effective allowlist. Deny wins over allow and over the
// global list, always — it is the only precedence that cannot loosen policy by
// accident.
func New(config Config) Allowlist {
	denied := make(map[string]struct{}, len(config.Deny))
	for _, program := range config.Deny {
		denied[program] = struct{}{}
	}
	entries := make([]Entry, 0, len(globalAllowlist)+len(config.Allow))
	for _, entry := range append(append([]Entry{}, globalAllowlist...), config.Allow...) {
		if _, refused := denied[entry.Program]; refused {
			continue
		}
		entries = append(entries, entry)
	}
	return Allowlist{entries: entries}
}

// Programs lists the authorized program names. It exists so a test can prove
// that checking a command never changes the policy.
func (a Allowlist) Programs() []string {
	programs := make([]string, 0, len(a.entries))
	for _, entry := range a.entries {
		programs = append(programs, entry.Program)
	}
	return programs
}

func (a Allowlist) KnowsProgram(program string) bool {
	for _, entry := range a.entries {
		if entry.Program == program {
			return true
		}
	}
	return false
}

// shellMeta is matched by substring, not by token — deliberately. A legitimate
// `--features=a&b` is refused; smuggling a metachar inside an argument is worse
// than the inconvenience.
var shellMeta = []string{";", "|", "&", "$(", "`", ">", "<", "\n"}

var shellPrograms = map[string]struct{}{
	"sh": {}, "bash": {}, "zsh": {}, "dash": {}, "fish": {}, "ksh": {},
	"cmd.exe": {}, "cmd": {}, "powershell": {}, "pwsh": {},
}

var interpreterFlags = map[string]struct{}{
	"-c": {}, "/C": {}, "/c": {}, "-e": {}, "-E": {}, "--eval": {}, "--exec": {},
}

// Check applies the rejection order of the phase design. The first rule that
// matches decides, and every refusal names its own reason: every later gate
// would also refuse, so a caller reading only "refused" cannot tell which door
// is locked.
//
// The first rule of the design — a command passed as a single string instead of
// an argv array — is enforced by the tool schema, which has no string command
// field to send.
func (a Allowlist) Check(argv []string) error {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return errors.New("missing program")
	}
	program := argv[0]
	args := argv[1:]

	if strings.ContainsAny(program, `/\`) {
		return errors.New("program path blocked; use a bare program name")
	}
	if containsShellMeta(program) {
		return errors.New("shell metachar in program")
	}
	for _, arg := range args {
		if containsShellMeta(arg) {
			return errors.New("shell metachar in arg — smuggling attempt")
		}
	}
	if _, isShell := shellPrograms[strings.ToLower(program)]; isShell {
		return errors.New("shell invocation blocked")
	}
	if len(args) > 0 {
		if _, isInterpreter := interpreterFlags[args[0]]; isInterpreter {
			return errors.New("interpreter flag blocked")
		}
	}
	for _, arg := range args {
		if hasParentSegment(arg) {
			return errors.New("path traversal in arg blocked")
		}
		if isAbsolutePath(arg) {
			return errors.New("absolute path in arg blocked")
		}
	}
	if isRecursiveForcedRemoval(program, args) {
		return errors.New("recursive forced removal blocked")
	}
	if len(a.entries) == 0 {
		return errors.New("no allowlist configured")
	}
	for _, entry := range a.entries {
		if entry.matches(program, args) {
			return nil
		}
	}
	return errors.New("command not in allowlist")
}

func (e Entry) matches(program string, args []string) bool {
	if e.Program != program || len(args) < len(e.RequiredArgPrefix) {
		return false
	}
	return slices.Equal(args[:len(e.RequiredArgPrefix)], e.RequiredArgPrefix)
}

// hasParentSegment reports a real traversal: a path component that is exactly
// "..". Matching the substring instead — which is what the design first wrote,
// copying the heritage gate — refuses `go test ./...`, the single most common
// Go invocation there is, because "..." contains "..". A rule that blocks the
// primary use case is not conservative, it is broken.
func hasParentSegment(arg string) bool {
	for _, segment := range strings.Split(strings.ReplaceAll(arg, `\`, "/"), "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

func containsShellMeta(value string) bool {
	for _, meta := range shellMeta {
		if strings.Contains(value, meta) {
			return true
		}
	}
	return false
}

func isAbsolutePath(arg string) bool {
	if strings.HasPrefix(arg, "/") {
		return true
	}
	// Windows drive-absolute: X:\ or X:/
	if len(arg) >= 3 && arg[1] == ':' && (arg[2] == '\\' || arg[2] == '/') {
		letter := arg[0]
		return (letter >= 'a' && letter <= 'z') || (letter >= 'A' && letter <= 'Z')
	}
	return false
}

// isRecursiveForcedRemoval is a cross product, not a disjunction of pairs:
// ((-r ∨ -R ∨ --recursive) ∧ (-f ∨ --force)). Written the other way — the shape
// the heritage document first described — `rm -r --force /` walks straight
// through. Defense in depth: rm is not in the global list, so the allowlist
// would already refuse it; this exists for the day a project allows it.
func isRecursiveForcedRemoval(program string, args []string) bool {
	if program != "rm" {
		return false
	}
	recursive, forced := false, false
	for _, arg := range args {
		switch {
		case arg == "--recursive":
			recursive = true
		case arg == "--force":
			forced = true
		case strings.HasPrefix(arg, "--"):
		case strings.HasPrefix(arg, "-"):
			for _, flag := range arg[1:] {
				switch flag {
				case 'r', 'R':
					recursive = true
				case 'f':
					forced = true
				}
			}
		}
	}
	return recursive && forced
}

// globalAllowlist is a constant of the binary. What is missing from it is as
// deliberate as what is in it: no deploy, publish, push, release or login; no
// dependency installation, which pulls new code into the worktree before it is
// verified; and no bare git, because git push lives there.
var globalAllowlist = []Entry{
	{Program: "go", RequiredArgPrefix: []string{"test"}},
	{Program: "go", RequiredArgPrefix: []string{"build"}},
	{Program: "go", RequiredArgPrefix: []string{"vet"}},
	{Program: "go", RequiredArgPrefix: []string{"fmt"}},
	{Program: "gofmt"},
	{Program: "golangci-lint", RequiredArgPrefix: []string{"run"}},
	{Program: "cargo", RequiredArgPrefix: []string{"test"}},
	{Program: "cargo", RequiredArgPrefix: []string{"build"}},
	{Program: "cargo", RequiredArgPrefix: []string{"check"}},
	{Program: "cargo", RequiredArgPrefix: []string{"clippy"}},
	{Program: "cargo", RequiredArgPrefix: []string{"fmt"}},
	{Program: "npm", RequiredArgPrefix: []string{"test"}},
	{Program: "npm", RequiredArgPrefix: []string{"run"}},
	{Program: "pnpm", RequiredArgPrefix: []string{"test"}},
	{Program: "pnpm", RequiredArgPrefix: []string{"run"}},
	{Program: "yarn", RequiredArgPrefix: []string{"test"}},
	{Program: "pytest"},
	{Program: "python", RequiredArgPrefix: []string{"-m", "pytest"}},
	{Program: "python", RequiredArgPrefix: []string{"-m", "unittest"}},
	{Program: "python3", RequiredArgPrefix: []string{"-m", "pytest"}},
	{Program: "python3", RequiredArgPrefix: []string{"-m", "unittest"}},
	{Program: "make", RequiredArgPrefix: []string{"test"}},
	{Program: "make", RequiredArgPrefix: []string{"build"}},
	{Program: "make", RequiredArgPrefix: []string{"check"}},
	{Program: "make", RequiredArgPrefix: []string{"lint"}},
	{Program: "git", RequiredArgPrefix: []string{"status"}},
	{Program: "git", RequiredArgPrefix: []string{"diff"}},
	{Program: "git", RequiredArgPrefix: []string{"log"}},
}
