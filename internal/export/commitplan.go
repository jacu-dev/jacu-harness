// Package export holds the curated public-import commit plan from ADR-028.
package export

import (
	"fmt"

	"github.com/jacu-dev/jacu-harness/internal/provenance"
)

// Author is the single provenance identity for a curated import.
const Author = "Erick Soares do Couto <ecouto123@gmail.com>"

// Commit is one intended Conventional Commit in the import series.
type Commit struct {
	Subject string `json:"subject"`
	Area    string `json:"area"`
}

// Plan is the area-by-area series ADR-028 described. It documents a fresh
// export. It is not a rewrite script for an already-published public history.
func Plan() []Commit {
	return []Commit{
		{Subject: "chore: import the sanitized public tree", Area: "tree"},
		{Subject: "docs: add English product documentation", Area: "docs"},
		{Subject: "ci: vendor the verify workflow", Area: ".github/workflows"},
		{Subject: "feat: expose the jacu CLI and MCP server", Area: "cmd/jacu"},
		{Subject: "test: cover the exported packages", Area: "internal"},
		{Subject: "chore: add install and cloud bootstrap scripts", Area: "scripts"},
	}
}

// Validate reports the first plan subject that fails provenance-lint.
func Validate() error {
	for _, commit := range Plan() {
		for _, finding := range provenance.CheckSubject(commit.Subject) {
			if finding.Class == provenance.ClassTrace {
				return fmt.Errorf("commit plan subject %q: %s", commit.Subject, finding.Rule)
			}
		}
	}
	return nil
}
