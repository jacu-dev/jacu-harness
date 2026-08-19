package gitx

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestNewFailsWhenGitIsUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	_, err := New()
	if err == nil || !strings.Contains(err.Error(), "git") {
		t.Fatalf("New error = %v; want clear git error", err)
	}
}

func TestRevParseHeadAndHasCommits(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	want := runTestGit(t, repo, "rev-parse", "HEAD")
	got, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead: %v", err)
	}
	if got != want {
		t.Fatalf("RevParseHead = %q; want %q", got, want)
	}
	if !git.HasCommits(context.Background(), repo) {
		t.Fatal("HasCommits = false; want true")
	}
}

func TestHasCommitsReturnsFalseForEmptyRepository(t *testing.T) {
	repo := newEmptyTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if git.HasCommits(context.Background(), repo) {
		t.Fatal("HasCommits = true; want false")
	}
	if _, err := git.RevParseHead(context.Background(), filepath.Clean(repo)); err == nil {
		t.Fatal("RevParseHead empty repository returned nil error")
	}
}

func TestWorktreeAddLockUnlockAndRemove(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead: %v", err)
	}
	worktree := filepath.Join(t.TempDir(), "run")
	branch := "jacu/run-test"

	if err := git.WorktreeAdd(context.Background(), repo, worktree, branch, baseSHA); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if _, err := os.Stat(worktree); err != nil {
		t.Fatalf("worktree stat: %v", err)
	}
	if err := git.WorktreeLock(context.Background(), repo, worktree); err != nil {
		t.Fatalf("WorktreeLock: %v", err)
	}
	if list := runTestGit(t, repo, "worktree", "list", "--porcelain"); !strings.Contains(list, "locked") {
		t.Fatalf("worktree list missing lock: %s", list)
	}
	if err := git.WorktreeUnlock(context.Background(), repo, worktree); err != nil {
		t.Fatalf("WorktreeUnlock: %v", err)
	}
	if err := git.WorktreeRemove(context.Background(), repo, worktree); err != nil {
		t.Fatalf("WorktreeRemove: %v", err)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree remains after remove: %v", err)
	}
}

func TestWorktreePruneRemovesMissingWorktreeMetadata(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead: %v", err)
	}
	worktree := filepath.Join(t.TempDir(), "orphan")

	if err := git.WorktreeAdd(context.Background(), repo, worktree, "jacu/run-orphan", baseSHA); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if err := os.RemoveAll(worktree); err != nil {
		t.Fatalf("remove fixture worktree: %v", err)
	}
	if err := git.WorktreePrune(context.Background(), repo); err != nil {
		t.Fatalf("WorktreePrune: %v", err)
	}
	if list := runTestGit(t, repo, "worktree", "list", "--porcelain"); strings.Contains(list, worktree) {
		t.Fatalf("worktree metadata remains after prune: %s", list)
	}
}

func TestDiffAndNumstat(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\nsecond line\n"), 0o600); err != nil {
		t.Fatalf("write change: %v", err)
	}

	diff, err := git.Diff(context.Background(), repo, baseSHA)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !strings.Contains(diff, "+second line") {
		t.Fatalf("diff missing known hunk: %s", diff)
	}
	added, deleted, files, err := git.DiffNumstat(context.Background(), repo, baseSHA)
	if err != nil {
		t.Fatalf("DiffNumstat: %v", err)
	}
	if added != 1 || deleted != 0 || !reflect.DeepEqual(files, []string{"README.md"}) {
		t.Fatalf("DiffNumstat = added %d deleted %d files %v; want 1, 0, [README.md]", added, deleted, files)
	}
}

func TestReadOnlyNumstatIncludesTrackedUntrackedUnusualAndBinaryWithoutMutatingIndex(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\ntracked line\n"), 0o600); err != nil {
		t.Fatalf("write tracked change: %v", err)
	}
	const unusualPath = "odd\tline\nname.txt"
	if err := os.WriteFile(filepath.Join(repo, unusualPath), []byte("first\nsecond\n"), 0o600); err != nil {
		t.Fatalf("write unusual untracked file: %v", err)
	}
	const binaryPath = "artifact.bin"
	if err := os.WriteFile(filepath.Join(repo, binaryPath), []byte{0x00, 0x01, 0x02, 0xff}, 0o600); err != nil {
		t.Fatalf("write binary untracked file: %v", err)
	}

	indexPath := filepath.Join(repo, ".git", "index")
	statusBefore := runTestGitRaw(t, repo, "status", "--porcelain=v1", "-z")
	indexBefore, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index before: %v", err)
	}
	indexInfoBefore, err := os.Lstat(indexPath)
	if err != nil {
		t.Fatalf("lstat index before: %v", err)
	}
	added, deleted, files, err := git.ReadOnlyNumstat(context.Background(), repo, baseSHA)
	if err != nil {
		t.Fatalf("ReadOnlyNumstat: %v", err)
	}
	if added != 3 || deleted != 0 {
		t.Fatalf("ReadOnlyNumstat = added %d deleted %d; want 3, 0", added, deleted)
	}
	wantFiles := map[string]bool{"README.md": true, unusualPath: true, binaryPath: true}
	assertUniquePhysicalFiles(t, files, wantFiles)

	indexAfter, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("read index after: %v", err)
	}
	indexInfoAfter, err := os.Lstat(indexPath)
	if err != nil {
		t.Fatalf("lstat index after: %v", err)
	}
	if !bytes.Equal(indexAfter, indexBefore) || indexInfoAfter.Mode() != indexInfoBefore.Mode() || !indexInfoAfter.ModTime().Equal(indexInfoBefore.ModTime()) {
		t.Fatalf("index changed during read-only numstat")
	}
	if statusAfter := runTestGitRaw(t, repo, "status", "--porcelain=v1", "-z"); statusAfter != statusBefore {
		t.Fatalf("porcelain status changed:\nbefore: %q\nafter:  %q", statusBefore, statusAfter)
	}
}

func TestReadOnlyNumstatDoesNotCreateMissingIndex(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const deletedPath = "deleted.txt"
	if err := os.WriteFile(filepath.Join(repo, deletedPath), []byte("delete me\n"), 0o600); err != nil {
		t.Fatalf("write tracked deletion fixture: %v", err)
	}
	runTestGit(t, repo, "add", deletedPath)
	runTestGit(t, repo, "commit", "-m", "add deletion fixture")
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead: %v", err)
	}
	indexPath := filepath.Join(repo, ".git", "index")
	if err := os.Remove(indexPath); err != nil {
		t.Fatalf("remove index fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\ntracked addition\n"), 0o600); err != nil {
		t.Fatalf("write tracked modification: %v", err)
	}
	if err := os.Remove(filepath.Join(repo, deletedPath)); err != nil {
		t.Fatalf("remove tracked fixture: %v", err)
	}
	const textPath = "untracked.txt"
	if err := os.WriteFile(filepath.Join(repo, textPath), []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatalf("write untracked text: %v", err)
	}
	const unusualPath = "odd\tline\nname.txt"
	if err := os.WriteFile(filepath.Join(repo, unusualPath), []byte("odd\n"), 0o600); err != nil {
		t.Fatalf("write unusual untracked file: %v", err)
	}
	const binaryPath = "artifact.bin"
	if err := os.WriteFile(filepath.Join(repo, binaryPath), []byte{0x00, 0x01, 0xff}, 0o600); err != nil {
		t.Fatalf("write untracked binary: %v", err)
	}

	added, deleted, files, err := git.ReadOnlyNumstat(context.Background(), repo, baseSHA)
	if err != nil {
		t.Fatalf("ReadOnlyNumstat without index: %v", err)
	}
	if added != 4 || deleted != 1 {
		t.Fatalf("ReadOnlyNumstat without index = added %d deleted %d; want 4, 1", added, deleted)
	}
	assertUniquePhysicalFiles(t, files, map[string]bool{
		"README.md": true, deletedPath: true, textPath: true, unusualPath: true, binaryPath: true,
	})
	if _, err := os.Lstat(indexPath); !os.IsNotExist(err) {
		t.Fatalf("missing index was created: %v", err)
	}
}

func TestReadOnlyNumstatUsesFixedTemporaryIndexInvocations(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead: %v", err)
	}
	for _, path := range []string{"one.txt", "two.txt", "three.txt"} {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(path+"\n"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	logPath := filepath.Join(t.TempDir(), "git-invocations.log")
	wrapper := filepath.Join(t.TempDir(), "git-wrapper")
	script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\t%%s\\n' \"$GIT_INDEX_FILE\" \"$1\" >> %q\nexec %q \"$@\"\n", logPath, git.bin)
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatalf("write git wrapper: %v", err)
	}
	instrumentedGit := &Git{bin: wrapper}
	if _, _, _, err := instrumentedGit.ReadOnlyNumstat(context.Background(), repo, baseSHA); err != nil {
		t.Fatalf("ReadOnlyNumstat: %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read invocation log: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("git invocations = %d; want fixed 3:\n%s", len(lines), content)
	}
	wantCommands := []string{"read-tree", "add", "diff"}
	var temporaryIndex string
	for index, line := range lines {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] != wantCommands[index] {
			t.Fatalf("invocation %d = %q; want non-empty temp index and %s", index, line, wantCommands[index])
		}
		if temporaryIndex == "" {
			temporaryIndex = parts[0]
		} else if parts[0] != temporaryIndex {
			t.Fatalf("temporary index changed between invocations: %q vs %q", temporaryIndex, parts[0])
		}
	}
	if filepath.Clean(temporaryIndex) == filepath.Join(repo, ".git", "index") {
		t.Fatalf("temporary index points at real index: %q", temporaryIndex)
	}
	if _, err := os.Stat(filepath.Dir(temporaryIndex)); !os.IsNotExist(err) {
		t.Fatalf("temporary index directory not cleaned: %v", err)
	}
}

func TestDiffReadersUseFixedTemporaryIndexInvocations(t *testing.T) {
	tests := []struct {
		name string
		call func(t *testing.T, git *Git, repo, baseSHA string)
	}{
		{
			name: "Diff",
			call: func(t *testing.T, git *Git, repo, baseSHA string) {
				t.Helper()
				patch, err := git.Diff(context.Background(), repo, baseSHA)
				if err != nil {
					t.Fatalf("Diff: %v", err)
				}
				if !strings.Contains(patch, "+new one") || !strings.Contains(patch, "+new two") || !strings.Contains(patch, "-delete one") || !strings.Contains(patch, "-delete two") {
					t.Fatalf("Diff missing full new/deleted content:\n%s", patch)
				}
			},
		},
		{
			name: "DiffNumstat",
			call: func(t *testing.T, git *Git, repo, baseSHA string) {
				t.Helper()
				added, deleted, files, err := git.DiffNumstat(context.Background(), repo, baseSHA)
				if err != nil {
					t.Fatalf("DiffNumstat: %v", err)
				}
				if added != 2 || deleted != 2 {
					t.Fatalf("DiffNumstat = added %d deleted %d; want 2, 2", added, deleted)
				}
				assertUniquePhysicalFiles(t, files, map[string]bool{"new.txt": true, "deleted.txt": true})
			},
		},
		{
			name: "DiffSnapshot",
			call: func(t *testing.T, git *Git, repo, baseSHA string) {
				t.Helper()
				snapshot, err := git.DiffSnapshot(context.Background(), repo, baseSHA)
				if err != nil {
					t.Fatalf("DiffSnapshot: %v", err)
				}
				if snapshot.Added != 2 || snapshot.Deleted != 2 {
					t.Fatalf("snapshot = added %d deleted %d; want 2, 2", snapshot.Added, snapshot.Deleted)
				}
				assertUniquePhysicalFiles(t, snapshot.Files, map[string]bool{"new.txt": true, "deleted.txt": true})
				if !strings.Contains(snapshot.Patch, "+new one") || !strings.Contains(snapshot.Patch, "+new two") || !strings.Contains(snapshot.Patch, "-delete one") || !strings.Contains(snapshot.Patch, "-delete two") {
					t.Fatalf("DiffSnapshot missing full new/deleted content:\n%s", snapshot.Patch)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newTestRepo(t)
			if err := os.WriteFile(filepath.Join(repo, "deleted.txt"), []byte("delete one\ndelete two\n"), 0o600); err != nil {
				t.Fatalf("write tracked deletion fixture: %v", err)
			}
			runTestGit(t, repo, "add", "deleted.txt")
			runTestGit(t, repo, "commit", "-m", "add deletion fixture")
			git, err := New()
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			baseSHA, err := git.RevParseHead(context.Background(), repo)
			if err != nil {
				t.Fatalf("RevParseHead: %v", err)
			}
			if err := os.Remove(filepath.Join(repo, "deleted.txt")); err != nil {
				t.Fatalf("remove tracked fixture: %v", err)
			}
			if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new one\nnew two\n"), 0o600); err != nil {
				t.Fatalf("write untracked fixture: %v", err)
			}

			logPath := filepath.Join(t.TempDir(), "git-invocations.log")
			wrapper := filepath.Join(t.TempDir(), "git-wrapper")
			script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\t%%s\\n' \"$GIT_INDEX_FILE\" \"$1\" >> %q\nexec %q \"$@\"\n", logPath, git.bin)
			if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
				t.Fatalf("write git wrapper: %v", err)
			}
			instrumentedGit := &Git{bin: wrapper}
			test.call(t, instrumentedGit, repo, baseSHA)

			content, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("read invocation log: %v", err)
			}
			lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
			if len(lines) != 3 {
				t.Fatalf("git invocations = %d; want fixed 3:\n%s", len(lines), content)
			}
			wantCommands := []string{"read-tree", "add", "diff"}
			var temporaryIndex string
			for index, line := range lines {
				parts := strings.SplitN(line, "\t", 2)
				if len(parts) != 2 || parts[0] == "" || parts[1] != wantCommands[index] {
					t.Fatalf("invocation %d = %q; want non-empty temp index and %s", index, line, wantCommands[index])
				}
				if temporaryIndex == "" {
					temporaryIndex = parts[0]
				} else if parts[0] != temporaryIndex {
					t.Fatalf("temporary index changed between invocations: %q vs %q", temporaryIndex, parts[0])
				}
			}
			if filepath.Clean(temporaryIndex) == filepath.Join(repo, ".git", "index") {
				t.Fatalf("temporary index points at real index: %q", temporaryIndex)
			}
			if _, err := os.Stat(filepath.Dir(temporaryIndex)); !os.IsNotExist(err) {
				t.Fatalf("temporary index directory not cleaned: %v", err)
			}
		})
	}
}

func TestDiffTemporaryIndexCleansUpAfterEveryCommandFailure(t *testing.T) {
	for _, failureCommand := range []string{"read-tree", "add", "diff"} {
		t.Run(failureCommand, func(t *testing.T) {
			repo := newTestRepo(t)
			git, err := New()
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			baseSHA, err := git.RevParseHead(context.Background(), repo)
			if err != nil {
				t.Fatalf("RevParseHead: %v", err)
			}
			if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("new\n"), 0o600); err != nil {
				t.Fatalf("write untracked fixture: %v", err)
			}

			logPath := filepath.Join(t.TempDir(), "git-invocations.log")
			wrapper := filepath.Join(t.TempDir(), "git-wrapper")
			script := fmt.Sprintf("#!/bin/sh\nprintf '%%s\\t%%s\\n' \"$GIT_INDEX_FILE\" \"$1\" >> %q\nif [ \"$1\" = %q ]; then exit 17; fi\nexec %q \"$@\"\n", logPath, failureCommand, git.bin)
			if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
				t.Fatalf("write git wrapper: %v", err)
			}
			instrumentedGit := &Git{bin: wrapper}
			if _, err := instrumentedGit.DiffSnapshot(context.Background(), repo, baseSHA); err == nil {
				t.Fatalf("DiffSnapshot succeeded when %s failed", failureCommand)
			}

			content, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatalf("read invocation log: %v", err)
			}
			lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
			if len(lines) == 0 {
				t.Fatal("no Git invocation recorded")
			}
			var temporaryIndex string
			for _, line := range lines {
				parts := strings.SplitN(line, "\t", 2)
				if len(parts) != 2 || parts[0] == "" {
					t.Fatalf("invocation missing temporary index: %q", line)
				}
				if temporaryIndex == "" {
					temporaryIndex = parts[0]
				} else if parts[0] != temporaryIndex {
					t.Fatalf("temporary index changed between invocations: %q vs %q", temporaryIndex, parts[0])
				}
			}
			if _, err := os.Stat(filepath.Dir(temporaryIndex)); !os.IsNotExist(err) {
				t.Fatalf("temporary index directory not cleaned after %s failure: %v", failureCommand, err)
			}
		})
	}
}

func assertUniquePhysicalFiles(t *testing.T, files []string, want map[string]bool) {
	t.Helper()
	if len(files) != len(want) {
		t.Fatalf("files = %q; want %v", files, want)
	}
	seen := make(map[string]bool, len(files))
	for _, path := range files {
		if !want[path] {
			t.Fatalf("unexpected physical path %q in %q", path, files)
		}
		if seen[path] {
			t.Fatalf("duplicate physical path %q in %q", path, files)
		}
		seen[path] = true
	}
}

func TestDiffSnapshotCapturesPatchAndNumstatInSingleGitInvocation(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("first line\ndiff --git content line\n"), 0o600); err != nil {
		t.Fatalf("write new file: %v", err)
	}

	logPath := filepath.Join(t.TempDir(), "diff-invocations.log")
	wrapper := filepath.Join(t.TempDir(), "git-wrapper")
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"diff\" ]; then\n  printf 'diff\\n' >> %q\nfi\nexec %q \"$@\"\n", logPath, git.bin)
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatalf("write git wrapper: %v", err)
	}
	instrumentedGit := &Git{bin: wrapper}

	snapshot, err := instrumentedGit.DiffSnapshot(context.Background(), repo, baseSHA)
	if err != nil {
		t.Fatalf("DiffSnapshot: %v", err)
	}
	if snapshot.Added != 2 || snapshot.Deleted != 0 || !reflect.DeepEqual(snapshot.Files, []string{"new.txt"}) {
		t.Fatalf("snapshot numstat = added %d deleted %d files %v; want 2, 0, [new.txt]", snapshot.Added, snapshot.Deleted, snapshot.Files)
	}
	if !strings.Contains(snapshot.Patch, "+first line") || !strings.Contains(snapshot.Patch, "+diff --git content line") {
		t.Fatalf("snapshot patch missing content:\n%s", snapshot.Patch)
	}
	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read diff invocation log: %v", err)
	}
	if got := strings.Count(string(logContent), "diff\n"); got != 1 {
		t.Fatalf("git diff invocations = %d; want exactly 1", got)
	}
}

func TestDiffSnapshotIncludesBinaryPatchAndPhysicalPath(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead: %v", err)
	}
	const path = "artifact.bin"
	if err := os.WriteFile(filepath.Join(repo, path), []byte{0x00, 0x01, 0x02, 0x03, 0xff}, 0o600); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	snapshot, err := git.DiffSnapshot(context.Background(), repo, baseSHA)
	if err != nil {
		t.Fatalf("DiffSnapshot binary: %v", err)
	}
	if snapshot.Added != 0 || snapshot.Deleted != 0 || !reflect.DeepEqual(snapshot.Files, []string{path}) {
		t.Fatalf("binary numstat = added %d deleted %d files %q; want 0, 0, [%q]", snapshot.Added, snapshot.Deleted, snapshot.Files, path)
	}
	if !strings.Contains(snapshot.Patch, "GIT binary patch") {
		t.Fatalf("binary payload missing from patch:\n%s", snapshot.Patch)
	}
}

func TestDiffSnapshotPreservesTabAndNewlineInPhysicalPath(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead: %v", err)
	}
	const path = "odd\tline\nname.txt"
	if err := os.WriteFile(filepath.Join(repo, path), []byte("physical path\n"), 0o600); err != nil {
		t.Fatalf("write unusual path: %v", err)
	}

	snapshot, err := git.DiffSnapshot(context.Background(), repo, baseSHA)
	if err != nil {
		t.Fatalf("DiffSnapshot unusual path: %v", err)
	}
	if !reflect.DeepEqual(snapshot.Files, []string{path}) {
		t.Fatalf("snapshot files = %q; want exact physical path %q", snapshot.Files, path)
	}
}

func TestCommitAllPreservesMessageAndReturnsSHA(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("updated\n"), 0o600); err != nil {
		t.Fatalf("write change: %v", err)
	}
	message := "Update fixture\n\nAcceptance criteria:\n- README updated\n\nJacu-Run: run_test\nJacu-Mission: msn_test\n"

	sha, err := git.CommitAll(context.Background(), repo, message)
	if err != nil {
		t.Fatalf("CommitAll: %v", err)
	}
	if head := runTestGit(t, repo, "rev-parse", "HEAD"); sha != head {
		t.Fatalf("CommitAll sha = %q; HEAD = %q", sha, head)
	}
	object := runTestGitRaw(t, repo, "cat-file", "commit", sha)
	_, storedMessage, ok := strings.Cut(object, "\n\n")
	if !ok {
		t.Fatalf("commit object missing message separator: %q", object)
	}
	if storedMessage != message {
		t.Fatalf("stored message differs\n got: %q\nwant: %q", storedMessage, message)
	}
}

func TestCommitTreeUsesPinnedTreeWhenIndexChangesLater(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("validated tree\n"), 0o600); err != nil {
		t.Fatalf("write validated tree: %v", err)
	}
	treeSHA, err := git.StageTree(context.Background(), repo)
	if err != nil {
		t.Fatalf("StageTree: %v", err)
	}
	patch, err := git.DiffTree(context.Background(), repo, baseSHA, treeSHA)
	if err != nil {
		t.Fatalf("DiffTree: %v", err)
	}
	if !strings.Contains(patch, "+validated tree") {
		t.Fatalf("pinned tree patch missing validated content:\n%s", patch)
	}

	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("late staged tree\n"), 0o600); err != nil {
		t.Fatalf("write late tree: %v", err)
	}
	if err := git.StageAll(context.Background(), repo); err != nil {
		t.Fatalf("StageAll late tree: %v", err)
	}
	commitSHA, err := git.CommitTree(context.Background(), repo, baseSHA, treeSHA, "commit pinned tree\n")
	if err != nil {
		t.Fatalf("CommitTree: %v", err)
	}
	if parent := runTestGit(t, repo, "rev-parse", commitSHA+"^"); parent != baseSHA {
		t.Fatalf("commit parent = %q; want expected parent %q", parent, baseSHA)
	}
	if committedTree := runTestGit(t, repo, "rev-parse", commitSHA+"^{tree}"); committedTree != treeSHA {
		t.Fatalf("committed tree = %q; want pinned tree %q", committedTree, treeSHA)
	}
	if committed := runTestGit(t, repo, "show", commitSHA+":README.md"); committed != "validated tree" {
		t.Fatalf("committed README = %q; want validated tree", committed)
	}
}

func TestCommitTreeRejectsHeadDifferentFromExpectedParent(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("validated tree\n"), 0o600); err != nil {
		t.Fatalf("write validated tree: %v", err)
	}
	treeSHA, err := git.StageTree(context.Background(), repo)
	if err != nil {
		t.Fatalf("StageTree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("injected head\n"), 0o600); err != nil {
		t.Fatalf("write injected head: %v", err)
	}
	runTestGit(t, repo, "add", "README.md")
	runTestGit(t, repo, "commit", "-m", "injected commit")
	injectedHead := runTestGit(t, repo, "rev-parse", "HEAD")

	if _, err := git.CommitTree(context.Background(), repo, baseSHA, treeSHA, "must not advance ref\n"); err == nil {
		t.Fatal("CommitTree accepted HEAD different from expected parent")
	}
	if head := runTestGit(t, repo, "rev-parse", "HEAD"); head != injectedHead {
		t.Fatalf("HEAD = %q; want injected commit preserved %q", head, injectedHead)
	}
}

func TestUpdateHeadCASMovesHeadFromExpectedCommit(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead base: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("applied\n"), 0o600); err != nil {
		t.Fatalf("write applied change: %v", err)
	}
	appliedSHA, err := git.CommitAll(context.Background(), repo, "applied commit\n")
	if err != nil {
		t.Fatalf("CommitAll: %v", err)
	}

	if err := git.UpdateHeadCAS(context.Background(), repo, baseSHA, appliedSHA); err != nil {
		t.Fatalf("UpdateHeadCAS: %v", err)
	}
	if head := runTestGit(t, repo, "rev-parse", "HEAD"); head != baseSHA {
		t.Fatalf("HEAD = %q; want rolled back base %q", head, baseSHA)
	}
}

func TestUpdateHeadCASRejectsUnexpectedCurrentHead(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead base: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("applied\n"), 0o600); err != nil {
		t.Fatalf("write applied change: %v", err)
	}
	appliedSHA, err := git.CommitAll(context.Background(), repo, "applied commit\n")
	if err != nil {
		t.Fatalf("CommitAll: %v", err)
	}

	err = git.UpdateHeadCAS(context.Background(), repo, baseSHA, baseSHA)
	if err == nil {
		t.Fatal("UpdateHeadCAS accepted unexpected current HEAD")
	}
	if head := runTestGit(t, repo, "rev-parse", "HEAD"); head != appliedSHA {
		t.Fatalf("HEAD = %q; want applied commit preserved %q", head, appliedSHA)
	}
}

func TestWorktreeRemoveUsesGitForceForAuthorizedDirtyWorktree(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead: %v", err)
	}
	worktree := filepath.Join(t.TempDir(), "dirty-run")
	if err := git.WorktreeAdd(context.Background(), repo, worktree, "jacu/run-dirty", baseSHA); err != nil {
		t.Fatalf("WorktreeAdd: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "README.md"), []byte("dirty after commit\n"), 0o600); err != nil {
		t.Fatalf("write dirty worktree: %v", err)
	}

	if err := git.WorktreeRemove(context.Background(), repo, worktree); err != nil {
		t.Fatalf("WorktreeRemove dirty authorized worktree: %v", err)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("dirty worktree remains after Git removal: %v", err)
	}
}

func TestDetectsSubmodulesAndLFS(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	if git.HasSubmodules(ctx, repo) {
		t.Fatal("HasSubmodules = true before fixture")
	}
	if git.UsesLFS(ctx, repo) {
		t.Fatal("UsesLFS = true before fixture")
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitmodules"), []byte("[submodule \"dep\"]\n\tpath = dep\n\turl = ../dep\n"), 0o600); err != nil {
		t.Fatalf("write .gitmodules: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("*.bin filter=lfs diff=lfs merge=lfs -text\n"), 0o600); err != nil {
		t.Fatalf("write .gitattributes: %v", err)
	}

	if !git.HasSubmodules(ctx, repo) {
		t.Fatal("HasSubmodules = false with .gitmodules")
	}
	if !git.UsesLFS(ctx, repo) {
		t.Fatal("UsesLFS = false with filter=lfs")
	}
}

func TestCountAheadAndBranchDelete(t *testing.T) {
	repo := newTestRepo(t)
	git, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	baseSHA, err := git.RevParseHead(context.Background(), repo)
	if err != nil {
		t.Fatalf("RevParseHead: %v", err)
	}
	for i := 1; i <= 2; i++ {
		content := []byte(strings.Repeat("advance\n", i))
		if err := os.WriteFile(filepath.Join(repo, "README.md"), content, 0o600); err != nil {
			t.Fatalf("write advance %d: %v", i, err)
		}
		runTestGit(t, repo, "add", "README.md")
		runTestGit(t, repo, "commit", "-m", "advance")
	}

	ahead, err := git.CountAhead(context.Background(), repo, baseSHA)
	if err != nil {
		t.Fatalf("CountAhead: %v", err)
	}
	if ahead != 2 {
		t.Fatalf("CountAhead = %d; want 2", ahead)
	}

	const branch = "jacu/run-delete"
	runTestGit(t, repo, "branch", branch)
	if err := git.BranchDelete(context.Background(), repo, branch); err != nil {
		t.Fatalf("BranchDelete: %v", err)
	}
	branches := runTestGit(t, repo, "for-each-ref", "--format=%(refname:short)", "refs/heads/")
	if strings.Contains(branches, branch) {
		t.Fatalf("branch remains after delete: %s", branches)
	}
}
