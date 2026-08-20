package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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
	configPath := hostConfigHint(opts.Host, home)
	configAction := "printed"
	if opts.Config != "" {
		if !opts.DryRun {
			if mergeErr := mergeHostConfig(opts.Host, opts.Config, pack); mergeErr != nil {
				return mergeErr
			}
		}
		configAction = "written"
		configPath = opts.Config
	} else if !opts.JSON {
		if _, printErr := fmt.Fprint(stdout, pack); printErr != nil {
			return printErr
		}
		if _, hintErr := fmt.Fprintf(stdout, "write the snippet above to %s (pass --config to apply it)\n", configPath); hintErr != nil {
			return hintErr
		}
	}

	doctorText := doctorReport()
	if opts.JSON {
		result := map[string]any{
			"host":        opts.Host,
			"skills_dir":  skillsDir,
			"config":      configAction,
			"config_path": configPath,
			"pack":        pack,
			"doctor":      strings.TrimSpace(doctorText),
		}
		return json.NewEncoder(stdout).Encode(result)
	}
	_, printErr := fmt.Fprint(stdout, doctorText)
	return printErr
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

const retiredMCPCommand = "jacu-mcp"

func mergeExistingConfig(host string, existing []byte, pack string) ([]byte, error) {
	switch host {
	case "codex":
		return mergeCodexConfig(existing, pack)
	case "opencode":
		return mergeOpenCodeConfig(host, existing)
	default:
		return mergeJSONMCPServers(host, existing)
	}
}

func mergeCodexConfig(existing []byte, pack string) ([]byte, error) {
	text := string(existing)
	retired := strings.Contains(text, "[mcp_servers."+retiredMCPCommand+"]") || strings.Contains(text, `command = "`+retiredMCPCommand+`"`)
	if retired {
		migrated := strings.ReplaceAll(text, "[mcp_servers."+retiredMCPCommand+"]", "[mcp_servers.jacu]")
		if strings.Contains(migrated, `args = ["serve"]`) {
			migrated = strings.ReplaceAll(migrated, `command = "`+retiredMCPCommand+`"`, `command = "jacu"`)
		} else {
			migrated = strings.ReplaceAll(migrated, `command = "`+retiredMCPCommand+`"`, "command = \"jacu\"\nargs = [\"serve\"]")
		}
		return []byte(migrated), nil
	}
	if strings.Contains(text, "[mcp_servers.jacu]") {
		if strings.Contains(text, `command = "jacu"`) && strings.Contains(text, `args = ["serve"]`) {
			return existing, nil
		}
		return nil, fmt.Errorf("conflict: codex already registers [mcp_servers.jacu]")
	}
	out := strings.TrimRight(text, "\n") + "\n\n" + pack
	return []byte(out), nil
}

func mergeOpenCodeConfig(host string, existing []byte) ([]byte, error) {
	var current map[string]any
	if err := json.Unmarshal(existing, &current); err != nil {
		return nil, fmt.Errorf("parse existing config: %w", err)
	}
	mcp, _ := current["mcp"].(map[string]any)
	if mcp == nil {
		mcp = map[string]any{}
		current["mcp"] = mcp
	}
	changed, err := applyRetiredServer(mcp, "jacu", canonicalOpenCodeServer(), openCodeLaunchesJacuServe, jsonCommandIsRetired)
	if err != nil {
		return nil, fmt.Errorf("conflict: %s already registers mcp.jacu", host)
	}
	if !changed {
		return existing, nil
	}
	return json.MarshalIndent(current, "", "  ")
}

func mergeJSONMCPServers(host string, existing []byte) ([]byte, error) {
	var current map[string]any
	if err := json.Unmarshal(existing, &current); err != nil {
		return nil, fmt.Errorf("parse existing config: %w", err)
	}
	servers, _ := current["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
		current["mcpServers"] = servers
	}
	changed, err := applyRetiredServer(servers, "jacu", canonicalJSONServer(), jsonLaunchesJacuServe, jsonCommandIsRetired)
	if err != nil {
		return nil, fmt.Errorf("conflict: %s already registers mcpServers.jacu", host)
	}
	if !changed {
		return existing, nil
	}
	out, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func canonicalJSONServer() map[string]any {
	return map[string]any{"command": "jacu", "args": []string{"serve"}}
}

func canonicalOpenCodeServer() map[string]any {
	return map[string]any{"type": "local", "command": []string{"jacu", "serve"}, "enabled": true}
}

func applyRetiredServer(servers map[string]any, key string, canonical map[string]any, launchesServe, isRetired func(any) bool) (bool, error) {
	changed := false
	if _, ok := servers[retiredMCPCommand]; ok {
		if current, exists := servers[key]; exists && !launchesServe(current) && !isRetired(current) {
			return false, fmt.Errorf("conflict")
		}
		delete(servers, retiredMCPCommand)
		changed = true
	}
	if current, exists := servers[key]; exists {
		if launchesServe(current) {
			return changed, nil
		}
		if isRetired(current) {
			servers[key] = canonical
			return true, nil
		}
		return false, fmt.Errorf("conflict")
	}
	servers[key] = canonical
	return true, nil
}

func jsonLaunchesJacuServe(entry any) bool {
	object, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	command, _ := object["command"].(string)
	if command != "jacu" {
		return false
	}
	return jsonStringListEquals(object["args"], "serve")
}

func openCodeLaunchesJacuServe(entry any) bool {
	object, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	return jsonStringListEquals(object["command"], "jacu", "serve")
}

func jsonCommandIsRetired(entry any) bool {
	object, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	switch command := object["command"].(type) {
	case string:
		return filepath.Base(command) == retiredMCPCommand
	default:
		return jsonStringListHasRetiredBinary(command)
	}
}

// jsonStringList flattens the two shapes an argv list arrives in — []any once
// encoding/json has decoded a host config, []string when this process built the
// entry itself — into a single []string. Collapsing the shapes here is what lets
// the callers below index one slice they have just measured instead of carrying
// a "these two slices are the same length" invariant across a loop, which is
// both easier to read and something a bounds checker can actually see.
// A non-string element decodes to the empty string, so it can never match a
// wanted argument that is itself non-empty.
func jsonStringList(value any) ([]string, bool) {
	switch items := value.(type) {
	case []any:
		list := make([]string, 0, len(items))
		for _, item := range items {
			text, _ := item.(string)
			list = append(list, text)
		}
		return list, true
	case []string:
		return items, true
	default:
		return nil, false
	}
}

func jsonStringListHasRetiredBinary(value any) bool {
	items, ok := jsonStringList(value)
	if !ok {
		return false
	}
	if len(items) == 0 {
		return false
	}
	return filepath.Base(items[0]) == retiredMCPCommand
}

func jsonStringListEquals(value any, want ...string) bool {
	items, ok := jsonStringList(value)
	if !ok {
		return false
	}
	return slices.Equal(items, want)
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
