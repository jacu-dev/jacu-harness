package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Living teaching surfaces must not tell anyone to type jacu-mcp. The current
// module, user-dir, and GitHub repo are jacu-harness. The retired string may
// remain only as the archived module/repo path, the JACU_MCP_* env prefix, or
// named historical/export contracts.
//
// SDD-019 narrowed this allowlist: the compat alias and the ~/.jacu-mcp user
// directory are gone from the product, so a living document that still teaches
// either one is now a defect rather than an accepted exception.
func TestLivingDocsDoNotTeachRetiredCommand(t *testing.T) {
	root := filepath.Join("..", "..")
	allowed := regexp.MustCompile(
		`github\.com/jacu-dev/jacu-mcp|jacu-dev/jacu-mcp|JACU_MCP_[A-Z0-9_]+|"o jacu-mcp não chama modelo"`,
	)
	retired := regexp.MustCompile(`jacu-mcp`)
	// Frozen ADRs record the old product name. SDD-016 is the export contract
	// and must still be able to name the retired string.
	skipRel := map[string]bool{
		"docs/adr/ADR-000-heranca-jacu-code.md":           true,
		"docs/adr/ADR-001.md":                             true,
		"docs/adr/ADR-004.md":                             true,
		"docs/adr/ADR-009-model-control-host-profiles.md": true,
		"docs/adr/ADR-016-memory-bridge.md":               true,
		"docs/sdd/016-open-source-export/sdd.md":          true,
		"CHANGELOG.md":                                    true,
	}

	roots := []string{
		"README.md",
		"CHANGELOG.md",
		"CONTRIBUTING.md",
		"skills",
		"docs/README.md",
		"docs/architecture.md",
		"docs/hygiene.md",
		"docs/install.md",
		"docs/distribution.md",
		"docs/telemetry.md",
		"docs/threat-model.md",
		"docs/reference",
		"docs/decisions",
		"docs/sdd/PROGRAM.md",
		"docs/sdd/CONVENTIONS.md",
		"docs/sdd/specs",
		"docs/sdd/001-native-sdd",
		"docs/sdd/002-telemetry-v2",
		"docs/sdd/003-clean-exit",
		"docs/sdd/004-preflight",
		"docs/sdd/005-clarity-gate",
		"docs/sdd/006-context-admission",
		"docs/sdd/007-surface-profile",
		"docs/sdd/008-audit-hardening",
		"docs/sdd/017-installable-cloud",
		"docs/design",
		"docs/adr",
		".cursor/mcp.json",
	}

	var hits []string
	for _, rel := range roots {
		path := filepath.Join(root, rel)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(path, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if strings.HasSuffix(name, ".lock.json") {
				return nil
			}
			switch filepath.Ext(name) {
			case ".md", ".json":
			default:
				return nil
			}
			relHit, relErr := filepath.Rel(root, name)
			if relErr != nil {
				relHit = name
			}
			relHit = filepath.ToSlash(relHit)
			if skipRel[relHit] {
				return nil
			}
			body, readErr := os.ReadFile(name) // #nosec G304,G122 -- name is a living-doc path walked from the repository root.
			if readErr != nil {
				return readErr
			}
			stripped := allowed.ReplaceAllString(string(body), "")
			if retired.MatchString(stripped) {
				hits = append(hits, relHit)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", rel, err)
		}
	}
	if len(hits) > 0 {
		t.Fatalf("living docs still teach jacu-mcp: %s", strings.Join(hits, ", "))
	}
}
