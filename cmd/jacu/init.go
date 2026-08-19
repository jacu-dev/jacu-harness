package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jacu-dev/jacu-harness"
	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/mcpadapter"
)

func runInit(args []string, stdout, stderr io.Writer) int {
	opts, err := parseInitArgs(args)
	if err != nil {
		if _, printErr := fmt.Fprintln(stderr, "init:", err); printErr != nil {
			return 2
		}
		if _, printErr := fmt.Fprintln(stderr, "usage: jacu init --host <claude-code|claude-desktop|codex|cursor|generic|opencode> [--skills-dir DIR] [--config FILE] [--from DIR] [--repo PATH] [--dry-run] [--json]"); printErr != nil {
			return 2
		}
		return 2
	}
	if err := applyInit(opts, stdout); err != nil {
		if _, printErr := fmt.Fprintln(stderr, "init:", err); printErr != nil {
			return 1
		}
		return 1
	}
	return 0
}

type initOptions struct {
	Host      string
	SkillsDir string
	Config    string
	From      string
	Repo      string
	DryRun    bool
	JSON      bool
}

func parseInitArgs(args []string) (initOptions, error) {
	opts := initOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--host":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--host requires a value")
			}
			opts.Host = args[i]
		case "--skills-dir":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--skills-dir requires a value")
			}
			opts.SkillsDir = args[i]
		case "--config":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--config requires a value")
			}
			opts.Config = args[i]
		case "--from":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--from requires a value")
			}
			opts.From = args[i]
		case "--repo":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--repo requires a value")
			}
			opts.Repo = args[i]
		case "--dry-run":
			opts.DryRun = true
		case "--json":
			opts.JSON = true
		default:
			return opts, fmt.Errorf("unknown flag %s", args[i])
		}
	}
	if opts.Host == "" {
		return opts, fmt.Errorf("--host is required")
	}
	if _, err := renderHostPack(opts.Host, opts.Repo); err != nil {
		return opts, err
	}
	return opts, nil
}

func applyInit(opts initOptions, stdout io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.Getenv("HOME")
	}
	skillsDir := opts.SkillsDir
	if skillsDir == "" {
		skillsDir = defaultSkillsDir(opts.Host, home)
	}
	if skillsDir == "" {
		return fmt.Errorf("generic host requires --skills-dir so skills are not written to a guessed path")
	}
	entries, loadErr := loadSkillEntries(opts.From)
	if loadErr != nil {
		return loadErr
	}
	if setErr := validateCompleteSkillSet(entries); setErr != nil {
		return setErr
	}
	if !opts.DryRun {
		if mkdirErr := os.MkdirAll(skillsDir, 0o750); mkdirErr != nil {
			return mkdirErr
		}
		if writeErr := writeSkills(skillsDir, entries); writeErr != nil {
			return writeErr
		}
	}

	pack, packErr := renderHostPack(opts.Host, opts.Repo)
	if packErr != nil {
		return packErr
	}
	configAction := "printed"
	if opts.Config != "" {
		if !opts.DryRun {
			if mergeErr := mergeHostConfig(opts.Host, opts.Config, pack); mergeErr != nil {
				return mergeErr
			}
		}
		configAction = "written"
	} else if _, printErr := fmt.Fprint(stdout, pack); printErr != nil {
		return printErr
	} else if _, hintErr := fmt.Fprintf(stdout, "write the snippet above to %s (pass --config to apply it)\n", hostConfigHint(opts.Host, home)); hintErr != nil {
		return hintErr
	}

	doctorText := doctorReport()
	if !opts.JSON {
		_, printErr := fmt.Fprint(stdout, doctorText)
		return printErr
	}
	result := map[string]any{
		"host":       opts.Host,
		"skills_dir": skillsDir,
		"config":     configAction,
		"doctor":     strings.TrimSpace(doctorText),
	}
	return json.NewEncoder(stdout).Encode(result)
}

type skillFile struct {
	Rel  string
	Data []byte
}

func loadSkillEntries(from string) ([]skillFile, error) {
	if from != "" {
		return readSkillTree(os.DirFS(from), ".")
	}
	return readSkillTree(skillset.FS, "skills")
}

func readSkillTree(fsys fs.FS, root string) ([]skillFile, error) {
	var files []skillFile
	err := fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, "SKILL.md") {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		rel := path
		if root != "." {
			rel = strings.TrimPrefix(path, root+"/")
		}
		files = append(files, skillFile{Rel: rel, Data: data})
		return nil
	})
	return files, err
}

func validateCompleteSkillSet(files []skillFile) error {
	names := map[string]bool{}
	for _, file := range files {
		dir := filepath.ToSlash(filepath.Dir(file.Rel))
		if dir != "." && dir != "" {
			names[dir] = true
		}
	}
	if !names["using-jacu"] {
		return fmt.Errorf("skills source is not a complete set: using-jacu (router) is missing")
	}
	if len(names) < 2 {
		return fmt.Errorf("skills source is not a complete set: router with no capability skills")
	}
	return nil
}

func writeSkills(dest string, files []skillFile) error {
	for _, file := range files {
		target := filepath.Join(dest, filepath.FromSlash(file.Rel))
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(target, file.Data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func mergeHostConfig(host, path, pack string) error {
	if _, err := os.Stat(path); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		// #nosec G304,G703 -- path is the --config file the user named.
		return os.WriteFile(path, []byte(pack), 0o600)
	}
	// #nosec G304 -- path is the --config file the user named.
	existing, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	merged, err := mergeExistingConfig(host, existing, pack)
	if err != nil {
		return err
	}
	if string(merged) == string(existing) {
		return nil
	}
	backup := path + ".jacu-init.bak"
	// #nosec G304,G703 -- backup sits beside the user-named --config file.
	if err := os.WriteFile(backup, existing, 0o600); err != nil {
		return err
	}
	// #nosec G304,G703 -- path is the --config file the user named.
	return os.WriteFile(path, merged, 0o600)
}

func mergeExistingConfig(host string, existing []byte, pack string) ([]byte, error) {
	switch host {
	case "codex":
		if strings.Contains(string(existing), "[mcp_servers.jacu]") {
			if strings.Contains(string(existing), `command = "jacu"`) && strings.Contains(string(existing), `args = ["serve"]`) {
				return existing, nil
			}
			return nil, fmt.Errorf("conflict: %s already registers [mcp_servers.jacu]", host)
		}
		out := strings.TrimRight(string(existing), "\n") + "\n\n" + pack
		return []byte(out), nil
	case "opencode":
		var current map[string]any
		if err := json.Unmarshal(existing, &current); err != nil {
			return nil, fmt.Errorf("parse existing config: %w", err)
		}
		mcp, _ := current["mcp"].(map[string]any)
		if mcp == nil {
			mcp = map[string]any{}
			current["mcp"] = mcp
		}
		if _, exists := mcp["jacu"]; exists {
			return nil, fmt.Errorf("conflict: %s already registers mcp.jacu", host)
		}
		mcp["jacu"] = map[string]any{"type": "local", "command": []string{"jacu", "serve"}, "enabled": true}
		return json.MarshalIndent(current, "", "  ")
	default:
		var current map[string]any
		if err := json.Unmarshal(existing, &current); err != nil {
			return nil, fmt.Errorf("parse existing config: %w", err)
		}
		servers, _ := current["mcpServers"].(map[string]any)
		if servers == nil {
			servers = map[string]any{}
			current["mcpServers"] = servers
		}
		if existingJacu, exists := servers["jacu"]; exists {
			want := map[string]any{"command": "jacu", "args": []any{"serve"}}
			encodedHave, _ := json.Marshal(existingJacu)
			encodedWant, _ := json.Marshal(want)
			if string(encodedHave) == string(encodedWant) {
				return existing, nil
			}
			return nil, fmt.Errorf("conflict: %s already registers mcpServers.jacu", host)
		}
		servers["jacu"] = map[string]any{"command": "jacu", "args": []string{"serve"}}
		out, err := json.MarshalIndent(current, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(out, '\n'), nil
	}
}

func doctorReport() string {
	var b strings.Builder
	fmt.Fprintf(&b, "jacu %s\n", Version)
	fmt.Fprintf(&b, "go %s\n", runtime.Version())
	git, err := gitx.New()
	if err != nil {
		fmt.Fprintf(&b, "git %v\n", err)
	} else if gitVersion, err := git.Version(); err != nil {
		fmt.Fprintf(&b, "git %v\n", err)
	} else {
		fmt.Fprintln(&b, gitVersion)
	}
	fmt.Fprintf(&b, "protocols %s\n", strings.Join(mcpadapter.SupportedProtocolVersions, ", "))
	return b.String()
}
