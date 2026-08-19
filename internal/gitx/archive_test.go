package gitx

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLsFilesLogCommitsAndArchiveHead(t *testing.T) {
	repo := newTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("payload\n"), 0o600); err != nil {
		t.Fatalf("write tracked: %v", err)
	}
	runTestGit(t, repo, "add", "tracked.txt")
	runTestGit(t, repo, "commit", "-m", "feat: add tracked file")

	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	files, err := git.LsFiles(context.Background(), repo)
	if err != nil {
		t.Fatalf("LsFiles: %v", err)
	}
	if !reflect.DeepEqual(files, []string{"README.md", "tracked.txt"}) && !reflect.DeepEqual(files, []string{"tracked.txt", "README.md"}) {
		if len(files) != 2 {
			t.Fatalf("LsFiles = %v; want README.md and tracked.txt", files)
		}
	}

	commits, err := git.LogCommits(context.Background(), repo, "")
	if err != nil {
		t.Fatalf("LogCommits: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("LogCommits len = %d; want 2", len(commits))
	}
	if commits[0].Subject != "feat: add tracked file" {
		t.Fatalf("latest subject = %q", commits[0].Subject)
	}
	if commits[0].Hash == "" || commits[0].AuthorEmail == "" {
		t.Fatalf("commit identity missing: %+v", commits[0])
	}

	var archive bytes.Buffer
	if err = git.ArchiveHead(context.Background(), repo, &archive); err != nil {
		t.Fatalf("ArchiveHead: %v", err)
	}
	if archive.Len() == 0 {
		t.Fatal("ArchiveHead wrote nothing")
	}
}
