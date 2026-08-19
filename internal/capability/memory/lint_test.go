package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/project"
)

func TestLintBlocksDerivedMemoryWithoutEvidence(t *testing.T) {
	root, in := validMemoryInput(t)
	in.Source = "derived"
	in.Evidence = nil

	assertMemoryLint(t, lint(root, in), Lint{
		Level: "BLOCK", Rule: "derived_without_evidence", Message: "derived memory requires evidence", Field: "evidence",
	})
}

func TestLintBlocksInvalidSource(t *testing.T) {
	for _, source := range []string{"agent", ""} {
		t.Run(source, func(t *testing.T) {
			root, in := validMemoryInput(t)
			in.Source = source

			assertMemoryLint(t, lint(root, in), Lint{
				Level: "BLOCK", Rule: "invalid_source", Message: "source must be human or derived", Field: "source",
			})
		})
	}
}

func TestLintBlocksSecretPatternsInTitleOrBody(t *testing.T) {
	patterns := []struct {
		name  string
		value string
	}{
		{name: "private key", value: "-----BEGIN RSA PRIVATE KEY-----"},
		{name: "GitHub personal token", value: "ghp_abcdefghijklmnopqrstuvwxyz123456"},
		{name: "GitHub OAuth token", value: "gho_abcdefghijklmnopqrstuvwxyz123456"},
		{name: "GitHub fine grained token", value: "github_pat_abcdefghijklmnopqrstuvwxyz"},
		{name: "OpenAI token", value: "sk-abcdefghijklmnopqrstuvwxyz"},
		{name: "AWS access key", value: "AKIAABCDEFGHIJKLMNOP"},
		{name: "Slack bot token", value: "xoxb-1234567890-secret"},
		{name: "Slack user token", value: "xoxp-1234567890-secret"},
		{name: "Slack app token", value: "xoxa-1234567890-secret"},
		{name: "Slack session token", value: "xoxs-1234567890-secret"},
		{name: "bearer token", value: "Bearer abc.def.ghi"},
		{name: "URL credentials", value: "https://alice:hunter2@example.com/private"},
		{name: "password assignment", value: "password = hunter2"},
		{name: "passwd assignment", value: "passwd: hunter2"},
		{name: "secret assignment", value: "secret=hunter2"},
		{name: "token assignment", value: "token: hunter2"},
		{name: "API key assignment", value: "api_key = hunter2"},
	}

	for _, tt := range patterns {
		t.Run(tt.name+" in title", func(t *testing.T) {
			root, in := validMemoryInput(t)
			in.Title = "Credential " + tt.value
			assertMemoryLint(t, lint(root, in), secretLint())
		})
		t.Run(tt.name+" in body", func(t *testing.T) {
			root, in := validMemoryInput(t)
			in.Body = "Credential " + tt.value
			assertMemoryLint(t, lint(root, in), secretLint())
		})
	}
}

func TestLintDoesNotScanEvidenceOrUseEntropy(t *testing.T) {
	root, in := validMemoryInput(t)
	in.Title = "Commit 88a9e265b2092020bc1da3a6e45b31dca9e3a1307cf0ae903e1a10c31e57989d"
	in.Evidence = []string{"Bearer secret-token", "docs/secret=literal.md"}

	if got := lint(root, in); len(got) != 0 {
		t.Fatalf("lint() = %#v; want no lint for entropy or evidence content", got)
	}
}

func TestLintBlocksInvalidKind(t *testing.T) {
	root, in := validMemoryInput(t)
	in.Kind = "note"

	assertMemoryLint(t, lint(root, in), Lint{
		Level: "BLOCK", Rule: "invalid_kind", Message: "kind must be decision, convention, gotcha, or preference", Field: "kind",
	})
}

func TestLintBlocksGlobalScopeForNonPreference(t *testing.T) {
	_, in := validMemoryInput(t)
	in.ProjectID = ""
	in.Kind = "decision"

	assertMemoryLint(t, lint(t.TempDir(), in), Lint{
		Level: "BLOCK", Rule: "global_scope_restricted", Message: "global scope is restricted to preference records", Field: "project_id",
	})
}

func TestLintAllowsGlobalPreference(t *testing.T) {
	_, in := validMemoryInput(t)
	in.ProjectID = ""
	in.Kind = "preference"

	if got := lint(t.TempDir(), in); len(got) != 0 {
		t.Fatalf("lint() = %#v; want no lint", got)
	}
}

func TestLintBlocksProjectIDMismatch(t *testing.T) {
	root, in := validMemoryInput(t)
	in.ProjectID = "prj_0000000000000000"

	assertMemoryLint(t, lint(root, in), Lint{
		Level: "BLOCK", Rule: "project_id_mismatch", Message: "project_id does not match the project root", Field: "project_id",
	})
}

func TestLintFailsClosedWhenProjectRootCannotBeResolved(t *testing.T) {
	root, in := validMemoryInput(t)
	missing := filepath.Join(root, "missing")

	assertMemoryLint(t, lint(missing, in), Lint{
		Level: "BLOCK", Rule: "project_root_unresolved", Message: "project root could not be resolved", Field: "project_id",
	})
}

func TestLintBlocksEmptyTitle(t *testing.T) {
	root, in := validMemoryInput(t)
	in.Title = " \t "

	assertMemoryLint(t, lint(root, in), Lint{
		Level: "BLOCK", Rule: "empty_title", Message: "title is required", Field: "title",
	})
}

func TestLintBlocksTitleWithNewline(t *testing.T) {
	root, in := validMemoryInput(t)
	in.Title = "First line\nSecond line"

	assertMemoryLint(t, lint(root, in), Lint{
		Level: "BLOCK", Rule: "title_newline", Message: "title must not contain newlines", Field: "title",
	})
}

func TestLintBlocksTitleOver120UnicodeRunes(t *testing.T) {
	root, in := validMemoryInput(t)
	in.Title = strings.Repeat("界", 121)

	assertMemoryLint(t, lint(root, in), Lint{
		Level: "BLOCK", Rule: "title_too_long", Message: "title exceeds 120 characters", Field: "title",
	})
}

func TestLintBlocksBodyOver4096Bytes(t *testing.T) {
	root, in := validMemoryInput(t)
	in.Body = strings.Repeat("é", 2049)

	assertMemoryLint(t, lint(root, in), Lint{
		Level: "BLOCK", Rule: "body_too_large", Message: "body exceeds 4KB; summarize", Field: "body",
	})
}

func TestLintBlocksMalformedSupersedes(t *testing.T) {
	root, in := validMemoryInput(t)
	in.Supersedes = "mem_ABC/traversal"

	assertMemoryLint(t, lint(root, in), Lint{
		Level: "BLOCK", Rule: "invalid_supersedes", Message: "supersedes must be a valid memory_id", Field: "supersedes",
	})
}

func TestLintAcceptsValidMemory(t *testing.T) {
	root, in := validMemoryInput(t)
	in.Title = strings.Repeat("界", 120)
	in.Body = strings.Repeat("x", 4096)
	in.Supersedes = "mem_0123456789abcdef"

	if got := lint(root, in); len(got) != 0 {
		t.Fatalf("lint() = %#v; want no lint", got)
	}
}

func validMemoryInput(t *testing.T) (string, Input) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/project\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	projectID, err := project.ID(root)
	if err != nil {
		t.Fatalf("project.ID: %v", err)
	}
	return root, Input{
		ProjectID: projectID,
		Kind:      "decision",
		Title:     "Use deterministic memory identifiers",
		Body:      "The identity excludes mutable record content.",
		Source:    "human",
	}
}

func secretLint() Lint {
	return Lint{
		Level: "BLOCK", Rule: "secret_content", Message: "content matches secret pattern; not stored", Field: "content",
	}
}

func assertMemoryLint(t *testing.T, got []Lint, want Lint) {
	t.Helper()
	for _, item := range got {
		if item == want {
			return
		}
	}
	t.Fatalf("lint() = %#v; want item %#v", got, want)
}
