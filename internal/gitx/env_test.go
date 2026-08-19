package gitx

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCleanGitEnvRemovesAllRepositoryLocationVariables(t *testing.T) {
	input := []string{
		"PATH=/bin",
		"KEEP_ME=yes",
		"GIT_DIR=/victim/.git",
		"GIT_WORK_TREE=/victim",
		"GIT_INDEX_FILE=/victim/index",
		"GIT_OBJECT_DIRECTORY=/victim/objects",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES=/victim/alternate",
		"GIT_COMMON_DIR=/victim/common",
		"GIT_NAMESPACE=victim",
		"GIT_PREFIX=victim/",
		"GIT_CEILING_DIRECTORIES=/victim",
	}
	clean := cleanGitEnv(input)
	joined := strings.Join(clean, "\x00")
	for _, name := range gitLocationVariables {
		if strings.Contains(joined, name+"=") {
			t.Fatalf("%s survived cleanGitEnv: %v", name, clean)
		}
	}
	if !strings.Contains(joined, "KEEP_ME=yes") || !strings.Contains(joined, "PATH=/bin") {
		t.Fatalf("trusted variables were lost: %v", clean)
	}
}

func TestGitCommandsIgnoreAmbientRepositoryLocationVariables(t *testing.T) {
	repo := newTestRepo(t)
	victim := t.TempDir()
	for _, name := range gitLocationVariables {
		t.Setenv(name, victim)
	}
	git, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if !git.HasCommits(context.Background(), repo) {
		t.Fatal("ambient GIT_* variables redirected the Git command")
	}
	for _, name := range gitLocationVariables {
		if _, ok := os.LookupEnv(name); !ok {
			t.Fatalf("test environment lost %s", name)
		}
	}
}
