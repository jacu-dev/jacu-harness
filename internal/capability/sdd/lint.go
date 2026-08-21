package sdd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/capability/workspace"
	"github.com/jacu-dev/jacu-harness/internal/gitx"
)

var requiredSections = []string{
	"Why", "Locked decisions", "Out of scope", "Write scope", "Requirements",
	"Non-goals", "Open decisions", "Tasks", "Done", "Follow-ups",
}

// LintSDDContentWithLock lints document and lock bytes that were obtained by
// one already-confined filesystem boundary. It deliberately never opens a
// path, so a caller cannot validate one lock and then follow a swapped symlink.
func LintSDDContentWithLock(directory string, content, lock []byte) []Finding {
	return lintSDDContentWithLock(directory, content, lock, true)
}

// LintSelectedSDDContentWithLock lints an SDD explicitly selected by the
// caller. Unlike repository-wide lint, selection is an assertion that this
// SDD owns the working-tree change, so its write scope is checked even when
// the SDD document itself was committed before the implementation changes.
func LintSelectedSDDContentWithLock(directory string, content, lock []byte) []Finding {
	return lintSDDContentWithLock(directory, content, lock, false)
}

func lintSDDContentWithLock(directory string, content, lock []byte, scopeOnlyWhenDocumentChanged bool) []Finding {
	changed, err := changedPathsFor(directory)
	if err != nil {
		return []Finding{{Code: "sdd_git_state_unavailable", Severity: SeverityBlock, Target: directory, Message: "repository state unavailable"}}
	}
	document, parseErr := Parse(content)
	if parseErr != nil {
		document = Document{}
	}
	if scopeOnlyWhenDocumentChanged {
		changed = scopeChangesFor(directory, changed)
	}
	return lintDocumentWithLock(document, directory, changed, lock)
}

func scopeChangesFor(directory string, changed []string) []string {
	root, ok := repositoryRoot(directory)
	if !ok {
		return changed
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return changed
	}
	base := filepath.Base(filepath.Clean(directory))
	prefix := "docs/sdd/" + base + "/"
	for _, path := range changed {
		if strings.HasPrefix(filepath.ToSlash(path), prefix) {
			return changed
		}
	}
	return nil
}

func lintBytes(content []byte) (Document, []Finding) {
	document, _ := Parse(content)
	if len(content) == 0 {
		return document, []Finding{{Code: "sdd_missing_section", Severity: SeverityBlock, Target: "sdd.md", Message: "document is empty"}}
	}
	return document, nil
}

func lintDocumentWithLock(document Document, directory string, changedPaths []string, lock []byte) []Finding {
	findings := make([]Finding, 0)
	if !validChangeDirectory(filepath.Base(filepath.Clean(directory))) {
		findings = append(findings, Finding{Code: "sdd_bad_directory", Severity: SeverityBlock, Target: directory, Message: "directory must match NNN-slug"})
	}
	sections := make(map[string]Section, len(document.Sections))
	for _, section := range document.Sections {
		sections[strings.ToLower(section.Name)] = section
	}
	for _, required := range requiredSections {
		if _, ok := sections[strings.ToLower(required)]; !ok {
			findings = append(findings, Finding{Code: "sdd_missing_section", Severity: SeverityBlock, Target: required, Message: "required section is missing"})
		}
	}

	findings = append(findings, validateTasks(parseTasks(document))...)
	findings = append(findings, lintRequirements(document.Requirements)...)
	findings = append(findings, lintLockedDecisions(document, sections)...)
	findings = append(findings, lintOpenDecisions(sections)...)
	findings = append(findings, lintScope(document, changedPaths)...)
	findings = append(findings, lintLockContent(document, directory, lock)...)
	if portugueseStopwords(document) >= 3 {
		findings = append(findings, Finding{Code: "sdd_language_not_english", Severity: SeverityWarn, Target: "sdd.md", Message: "language heuristic found Portuguese stopwords"})
	}
	findings = append(findings, Finding{Code: "sdd_delta_summary", Severity: SeverityInfo, Target: "requirements", Message: deltaSummary(document.Requirements)})
	return findings
}

func lintRequirements(requirements []Requirement) []Finding {
	findings := make([]Finding, 0)
	for _, requirement := range requirements {
		if len(requirement.Scenarios) == 0 {
			findings = append(findings, Finding{Code: "sdd_requirement_without_scenario", Severity: SeverityWarn, Target: requirement.Name, Message: "requirement has no Scenario"})
		}
	}
	return findings
}

func lintLockedDecisions(document Document, sections map[string]Section) []Finding {
	section, ok := sections["locked decisions"]
	if !ok {
		return nil
	}
	adr := strings.TrimSpace(document.FrontMatter["adr"])
	if adr == "" {
		for _, line := range section.Lines {
			if strings.Contains(line, "ADR-") {
				return nil
			}
		}
		return []Finding{{Code: "sdd_locked_decision_without_adr", Severity: SeverityWarn, Target: "Locked decisions", Message: "locked decisions have no ADR reference"}}
	}
	return nil
}

func lintOpenDecisions(sections map[string]Section) []Finding {
	section, ok := sections["open decisions"]
	if !ok {
		return nil
	}
	for _, line := range section.Lines {
		trimmed := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(trimmed, "- [ ]") && !strings.Contains(trimmed, "none") && !strings.Contains(trimmed, "resolved") {
			return []Finding{{Code: "sdd_open_decision", Severity: SeverityBlock, Target: "Open decisions", Message: "open decision remains unresolved"}}
		}
	}
	return nil
}

func lintScope(document Document, changedPaths []string) []Finding {
	allowed, forbidden := writeScope(document)
	for _, path := range changedPaths {
		if workspace.ScopesConflict(path, allowed, forbidden) {
			return []Finding{{Code: "sdd_out_of_scope_touched", Severity: SeverityBlock, Target: path, Message: "changed path is outside the declared write scope"}}
		}
	}
	return nil
}

func writeScope(document Document) (allowed, forbidden []string) {
	for _, section := range document.Sections {
		if !strings.EqualFold(section.Name, "Write scope") {
			continue
		}
		mode := ""
		for _, rawLine := range section.Lines {
			line := strings.TrimSpace(rawLine)
			switch strings.ToLower(line) {
			case "**allowed**":
				mode = "allowed"
				continue
			case "**forbidden**":
				mode = "forbidden"
				continue
			}
			if mode == "" || line == "" || strings.HasPrefix(line, "<!--") || strings.HasPrefix(line, "*") {
				continue
			}
			line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
			if mode == "allowed" {
				allowed = append(allowed, line)
			} else {
				forbidden = append(forbidden, line)
			}
		}
	}
	return normalizeStrings(allowed), normalizeStrings(forbidden)
}

func lintLockContent(document Document, directory string, content []byte) []Finding {
	path := filepath.Join(directory, "sdd.lock.json")
	var lock struct {
		ContentSHA256 string `json:"content_sha256"`
	}
	if err := json.Unmarshal(content, &lock); err != nil || lock.ContentSHA256 != normalizedContentSHA(document) {
		return []Finding{{Code: "sdd_stale_lock", Severity: SeverityBlock, Target: path, Message: "lock does not match normalized document"}}
	}
	return nil
}

func normalizedContentSHA(document Document) string {
	encoded, _ := json.Marshal(normalizeDocument(document))
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func portugueseStopwords(document Document) int {
	text := strings.ToLower(string(document.Raw))
	count := 0
	for _, word := range []string{" este ", " esta ", " deve ", " devem ", " para ", " uma ", " com ", " não ", " nao ", " que ", " sistema ", " quando ", " então ", " entao "} {
		count += strings.Count(" "+text+" ", word)
	}
	return count
}

func deltaSummary(requirements []Requirement) string {
	counts := map[string]int{"ADDED": 0, "MODIFIED": 0, "REMOVED": 0}
	for _, requirement := range requirements {
		if _, ok := counts[requirement.Delta]; ok {
			counts[requirement.Delta]++
		}
	}
	return fmt.Sprintf("ADDED=%d MODIFIED=%d REMOVED=%d", counts["ADDED"], counts["MODIFIED"], counts["REMOVED"])
}

func validChangeDirectory(name string) bool {
	if len(name) < 5 || name[3] != '-' {
		return false
	}
	for _, character := range name[:3] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return strings.TrimSpace(name[4:]) != ""
}

func changedPathsFor(directory string) ([]string, error) {
	root, ok := repositoryRoot(directory)
	if !ok {
		// Standalone parser tests may lint a document before it is placed in a
		// repository. There is no changed-path claim to make in that mode.
		return []string{}, nil
	}
	git, err := gitx.New()
	if err != nil {
		return nil, err
	}
	output, err := git.StatusPorcelainZ(context.Background(), root)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0)
	for _, record := range bytes.Split([]byte(output), []byte{0}) {
		if len(record) < 4 {
			continue
		}
		path := string(record[3:])
		if arrow := strings.LastIndex(path, " -> "); arrow >= 0 {
			path = path[arrow+4:]
		}
		paths = append(paths, filepath.ToSlash(path))
	}
	return paths, nil
}

func repositoryRoot(path string) (string, bool) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	if info, statErr := os.Stat(absolute); statErr == nil && !info.IsDir() {
		absolute = filepath.Dir(absolute)
	}
	for {
		if _, err := os.Stat(filepath.Join(absolute, ".git")); err == nil {
			return absolute, true
		}
		parent := filepath.Dir(absolute)
		if parent == absolute {
			return "", false
		}
		absolute = parent
	}
}

func exitCode(findings []Finding) int {
	for _, finding := range findings {
		if finding.Severity == SeverityBlock {
			return 1
		}
	}
	return 0
}

func statusCode(findings []Finding) string {
	return strconv.Itoa(exitCode(findings))
}
