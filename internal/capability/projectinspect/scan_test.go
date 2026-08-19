package projectinspect

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestScanDetectsGoProjectWithStableID(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.com/project\n\ngo 1.26\n")
	writeFixture(t, root, "main.go", "package main\n")

	first, warnings, err := Scan(context.Background(), root, Input{})
	if err != nil {
		t.Fatalf("first scan: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	second, _, err := Scan(context.Background(), root, Input{})
	if err != nil {
		t.Fatalf("second scan: %v", err)
	}

	if !reflect.DeepEqual(first.Languages, []string{"go"}) {
		t.Fatalf("languages = %v; want [go]", first.Languages)
	}
	if !reflect.DeepEqual(first.Manifests, []string{"go.mod"}) {
		t.Fatalf("manifests = %v; want [go.mod]", first.Manifests)
	}
	if !reflect.DeepEqual(first.TestCommands, []string{"go test ./..."}) {
		t.Fatalf("test commands = %v; want [go test ./...]", first.TestCommands)
	}
	if !strings.HasPrefix(first.ProjectID, "prj_") || first.ProjectID != second.ProjectID {
		t.Fatalf("project_id instável: first=%q second=%q", first.ProjectID, second.ProjectID)
	}
}

func TestScanDetectsGoAndTypeScript(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.com/project\n\ngo 1.26\n")
	writeFixture(t, root, "main.go", "package main\n")
	writeFixture(t, root, "package.json", `{"scripts":{"test":"vitest"}}`)
	writeFixture(t, root, "src/index.ts", "export {}\n")

	summary, warnings, err := Scan(context.Background(), root, Input{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if !reflect.DeepEqual(summary.Languages, []string{"go", "typescript"}) {
		t.Fatalf("languages = %v; want [go typescript]", summary.Languages)
	}
	if !reflect.DeepEqual(summary.Manifests, []string{"go.mod", "package.json"}) {
		t.Fatalf("manifests = %v; want [go.mod package.json]", summary.Manifests)
	}
	if !slices.Contains(summary.TestCommands, "npm test") {
		t.Fatalf("test commands = %v; missing npm test", summary.TestCommands)
	}
}

func TestScanWarnsOnMalformedPackageJSON(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "package.json", `{`)

	_, warnings, err := Scan(context.Background(), root, Input{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !slices.Contains(warnings, "malformed package.json") {
		t.Fatalf("warning = %v; want malformed package.json", warnings)
	}
}

func TestScanTruncatesAtMaxFiles(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, 4)

	summary, warnings, err := Scan(context.Background(), root, Input{MaxFiles: 2})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !summary.Truncated || !slices.Contains(warnings, "file limit reached") {
		t.Fatalf("scan acima do limite deve truncar com warning: summary=%+v warnings=%v", summary, warnings)
	}
	if summary.FileCount != 2 {
		t.Fatalf("file_count = %d; want 2", summary.FileCount)
	}
}

func TestScanClampsMaxFilesAtTenThousand(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, 10001)

	summary, warnings, err := Scan(context.Background(), root, Input{MaxFiles: 20000})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if summary.FileCount != 10000 || !summary.Truncated || !slices.Contains(warnings, "file limit reached") {
		t.Fatalf("max_files deve limitar em 10000: summary=%+v warnings=%v", summary, warnings)
	}
}

func TestScanEmptyProjectReturnsValidSummary(t *testing.T) {
	root := t.TempDir()

	summary, warnings, err := Scan(context.Background(), root, Input{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if summary.ProjectID == "" || summary.Name == "" || summary.FileCount != 0 || summary.Truncated {
		t.Fatalf("summary inválido: %+v", summary)
	}
	if summary.Languages == nil || summary.Manifests == nil || summary.TestCommands == nil {
		t.Fatalf("listas devem ser vazias, não nil: %+v", summary)
	}
}

func TestScanDoesNotFollowDirectorySymlinkOutsideRoot(t *testing.T) {
	outside := t.TempDir()
	writeFixture(t, outside, "go.mod", "module outside.example/project\n")
	writeFixture(t, outside, "main.go", "package main\n")
	root := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	summary, warnings, err := Scan(context.Background(), root, Input{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(summary.Languages) != 0 || len(summary.Manifests) != 0 || len(summary.TestCommands) != 0 {
		t.Fatalf("symlink externo contaminou resultado: %+v", summary)
	}
}

func TestScanRefusesManifestSymlinkOutsideRoot(t *testing.T) {
	const secret = "SYMLINK_SECRET_MUST_NOT_BE_READ"
	outside := t.TempDir()
	writeFixture(t, outside, "package.json", `{"scripts":{"test":"vitest"},"secret":"`+secret+`"}`)
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(outside, "package.json"), filepath.Join(root, "package.json")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	summary, warnings, err := Scan(context.Background(), root, Input{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if slices.Contains(summary.Manifests, "package.json") || slices.Contains(summary.TestCommands, "npm test") {
		t.Fatalf("manifest via symlink foi lido: summary=%+v warnings=%v", summary, warnings)
	}
	encoded, err := json.Marshal(struct {
		Summary  Summary  `json:"summary"`
		Warnings []string `json:"warnings"`
	}{summary, warnings})
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("conteúdo externo vazou no resultado: %s", encoded)
	}
}

func TestScanListsSensitiveFilesWithoutReadingContents(t *testing.T) {
	root := t.TempDir()
	secrets := map[string]string{
		".env":       "ENV_SECRET_VALUE",
		"id_rsa":     "RSA_SECRET_VALUE",
		"secret.pem": "PEM_SECRET_VALUE",
	}
	for name, content := range secrets {
		writeFixture(t, root, name, content)
	}

	summary, warnings, err := Scan(context.Background(), root, Input{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	encoded, err := json.Marshal(struct {
		Summary  Summary  `json:"summary"`
		Warnings []string `json:"warnings"`
	}{summary, warnings})
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	result := string(encoded)
	if !strings.Contains(result, "sensitive files present (not read)") {
		t.Fatalf("warning de sensíveis ausente: %s", result)
	}
	for name, secret := range secrets {
		if !strings.Contains(result, name) {
			t.Errorf("nome sensível %q ausente do warning: %s", name, result)
		}
		if strings.Contains(result, secret) {
			t.Errorf("conteúdo de %q vazou no resultado: %s", name, result)
		}
	}
}

func TestScanHandlesHostileNames(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "ção 💥.go", "package unicode\n")
	writeFixture(t, root, "line\nbreak.go", "package newline\n")

	summary, _, err := Scan(context.Background(), root, Input{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !slices.Contains(summary.Languages, "go") {
		t.Fatalf("linguagem Go ausente: %+v", summary)
	}
}

func TestScanIgnoresGitDirectoryAndMarksPresence(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, ".git/go.mod", "module forged.example/project\n")
	writeFixture(t, root, ".git/main.go", "package forged\n")

	summary, warnings, err := Scan(context.Background(), root, Input{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(summary.Languages) != 0 || len(summary.Manifests) != 0 || len(summary.TestCommands) != 0 {
		t.Fatalf("conteúdo de .git contaminou resultado: %+v", summary)
	}
	if !slices.Contains(warnings, "git directory present (not scanned)") {
		t.Fatalf("presença de .git não foi marcada: %v", warnings)
	}
}

type cancelAfterContext struct {
	context.Context
	remaining int
}

func (c *cancelAfterContext) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return nil
}

func TestCancelDuringScan(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, 100)
	ctx := &cancelAfterContext{Context: context.Background(), remaining: 10}

	_, _, err := Scan(ctx, root, Input{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("scan cancelado retornou %v; want context.Canceled", err)
	}
}

func TestScanDoesNotReadOversizedManifest(t *testing.T) {
	root := t.TempDir()
	content := `{"scripts":{"test":"vitest"},"padding":"` + strings.Repeat("x", 256*1024) + `"}`
	writeFixture(t, root, "package.json", content)

	summary, warnings, err := Scan(context.Background(), root, Input{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !slices.Contains(summary.Manifests, "package.json") {
		t.Fatalf("manifest ausente: %+v", summary)
	}
	if slices.Contains(summary.TestCommands, "npm test") {
		t.Fatalf("manifest gigante foi lido: %+v", summary)
	}
	if !slices.Contains(warnings, "package.json exceeds 256KB (not read)") {
		t.Fatalf("warning de tamanho ausente: %v", warnings)
	}
}
