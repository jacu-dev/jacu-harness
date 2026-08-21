package workspace

import (
	"context"
	"fmt"
	"github.com/jacu-dev/jacu-harness/internal/testgit"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	"github.com/jacu-dev/jacu-harness/internal/project"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/userstate"
)

func TestOpenHappyPathCreatesLockedWorktreeAndCanonicalState(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	input := validMissionInput(t, repo)
	mission, status, _ := missioncompile.Compile(repo, input)
	if status != "ok" || mission.MissionID == "" {
		t.Fatalf("Compile fixture = status %q mission %#v", status, mission)
	}

	result, openErr := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: mission.MissionID})
	if openErr != nil {
		t.Fatalf("Open: %v", openErr)
	}
	if result.Status != "ok" {
		t.Fatalf("status = %q; summary = %q", result.Status, result.Summary)
	}
	if _, err := os.Stat(result.Data.WorktreePath); err != nil {
		t.Fatalf("worktree stat: %v", err)
	}
	if list := runGit(t, repo, "worktree", "list", "--porcelain"); !strings.Contains(list, "locked") {
		t.Fatalf("worktree is not locked: %s", list)
	}
	state, err := runstate.Load(repo, result.Data.RunID)
	if err != nil {
		t.Fatalf("Load state: %v", err)
	}
	if !reflect.DeepEqual(state.MissionInput, input) || !reflect.DeepEqual(state.Mission, mission) {
		t.Fatalf("canonical mission state differs: %#v", state)
	}
	if state.CreatedAt.Location() != time.UTC || state.CreatedAt.IsZero() {
		t.Fatalf("created_at = %v; want non-zero UTC", state.CreatedAt)
	}
	if state.Status != runstate.StatusOpen || state.BaseSHA != result.Data.BaseSHA || state.Branch != result.Data.Branch || state.Worktree != result.Data.WorktreePath {
		t.Fatalf("state does not match output: state=%#v data=%#v", state, result.Data)
	}
	projectID, err := project.ID(repo)
	if err != nil {
		t.Fatalf("project.ID: %v", err)
	}
	runHome, err := userstate.RunHome(projectID, result.Data.RunID)
	if err != nil {
		t.Fatalf("RunHome: %v", err)
	}
	if info, statErr := os.Stat(runHome); statErr != nil || !info.IsDir() {
		t.Fatalf("run home %s: %v", runHome, statErr)
	}
	cleanupWorktree(t, repo, result.Data.WorktreePath)
}

func TestTwoOpensIsolateRunHomes(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("JACU_HOME", t.TempDir())
	input := validMissionInput(t, repo)
	mission, status, _ := missioncompile.Compile(repo, input)
	if status != "ok" {
		t.Fatalf("Compile: %q", status)
	}
	first, err := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: mission.MissionID})
	if err != nil || first.Status != "ok" {
		t.Fatalf("first Open = %#v %v", first, err)
	}
	defer cleanupWorktree(t, repo, first.Data.WorktreePath)
	second, err := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: mission.MissionID})
	if err != nil || second.Status != "ok" {
		t.Fatalf("second Open = %#v %v", second, err)
	}
	defer cleanupWorktree(t, repo, second.Data.WorktreePath)
	projectID, err := project.ID(repo)
	if err != nil {
		t.Fatal(err)
	}
	homeA, err := userstate.RunHome(projectID, first.Data.RunID)
	if err != nil {
		t.Fatal(err)
	}
	homeB, err := userstate.RunHome(projectID, second.Data.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if homeA == homeB {
		t.Fatal("runs shared a home")
	}
	if err := os.WriteFile(filepath.Join(homeA, "marker"), []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(homeB, "marker")); !os.IsNotExist(err) {
		t.Fatalf("second run observed first run file: %v", err)
	}
}

func TestOpenCleansWorktreeWhenAddReturnsAfterCancellation(t *testing.T) {
	repo := newTestRepo(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	input := validMissionInput(t, repo)
	mission, status, _ := missioncompile.Compile(repo, input)
	if status != "ok" {
		t.Fatalf("Compile status = %q; want ok", status)
	}

	realGit, lookupErr := exec.LookPath("git")
	if lookupErr != nil {
		t.Fatalf("find git: %v", lookupErr)
	}
	wrapperDir := t.TempDir()
	ready := filepath.Join(wrapperDir, "worktree-add.ready")
	wrapper := filepath.Join(wrapperDir, "git")
	script := fmt.Sprintf(`#!/bin/sh
real_git=%q
ready=%q
if [ "$1" = "worktree" ] && [ "$2" = "add" ]; then
  "$real_git" "$@"
  code=$?
  if [ "$code" -eq 0 ]; then
    : > "$ready"
    while true; do sleep 1; done
  fi
  exit "$code"
fi
exec "$real_git" "$@"
`, realGit, ready)
	// #nosec G306 -- the test wrapper must be executable by the test process.
	if writeErr := os.WriteFile(wrapper, []byte(script), 0o700); writeErr != nil {
		t.Fatalf("write git wrapper: %v", writeErr)
	}
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithCancel(context.Background())
	openDone := make(chan error, 1)
	go func() {
		_, openErr := Open(ctx, repo, OpenInput{MissionInput: input, MissionID: mission.MissionID})
		openDone <- openErr
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat worktree add marker: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatal("worktree add did not reach cancellation point")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-openDone:
		if err == nil {
			t.Fatal("Open error = nil; want cancelled worktree add error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Open did not return after cancellation")
	}

	branches := runGit(t, repo, "for-each-ref", "--format=%(refname:short)", "refs/heads/jacu/run-*")
	metadata, err := os.ReadDir(filepath.Join(repo, ".git", "worktrees"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read worktree metadata: %v", err)
	}
	projectID, err := project.ID(repo)
	if err != nil {
		t.Fatalf("project.ID: %v", err)
	}
	worktreeDirs, err := os.ReadDir(filepath.Join(home, ".jacu-harness", "worktrees", projectID))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read worktree directory: %v", err)
	}
	if branches != "" || len(metadata) != 0 || len(worktreeDirs) != 0 {
		t.Fatalf("cancelled Open left branch=%q metadata_entries=%d worktree_dirs=%d; want none", branches, len(metadata), len(worktreeDirs))
	}
}

func TestOpenPreservesPreExistingBranchWhenWorktreeAddFails(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	input := validMissionInput(t, repo)
	mission, status, _ := missioncompile.Compile(repo, input)
	if status != "ok" {
		t.Fatalf("Compile status = %q; want ok", status)
	}

	realGit, lookupErr := exec.LookPath("git")
	if lookupErr != nil {
		t.Fatalf("find git: %v", lookupErr)
	}
	wrapperDir := t.TempDir()
	branchRecord := filepath.Join(wrapperDir, "pre-existing-branch")
	wrapper := filepath.Join(wrapperDir, "git")
	script := fmt.Sprintf(`#!/bin/sh
real_git=%q
branch_record=%q
if [ "$1" = "worktree" ] && [ "$2" = "add" ] && [ "$3" = "-b" ]; then
  branch=$4
  "$real_git" branch -- "$branch" HEAD
  printf '%%s\n' "$branch" > "$branch_record"
fi
if [ "$1" = "update-ref" ]; then
  case "$2" in
    refs/heads/jacu/run-*)
      branch=${2#refs/heads/}
      "$real_git" branch -- "$branch" HEAD
      printf '%%s\n' "$branch" > "$branch_record"
      ;;
  esac
fi
exec "$real_git" "$@"
`, realGit, branchRecord)
	// #nosec G306 -- the test wrapper must be executable by the test process.
	if writeErr := os.WriteFile(wrapper, []byte(script), 0o700); writeErr != nil {
		t.Fatalf("write git wrapper: %v", writeErr)
	}
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, openErr := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: mission.MissionID})
	if openErr == nil {
		t.Fatal("Open error = nil; want branch collision failure")
	}
	// #nosec G304 -- branchRecord is a fixed path under this test's t.TempDir.
	branchBytes, readErr := os.ReadFile(branchRecord)
	if readErr != nil {
		t.Fatalf("read pre-existing branch record: %v", readErr)
	}
	branch := strings.TrimSpace(string(branchBytes))
	wantSHA := runGit(t, repo, "rev-parse", "HEAD")
	gotRef := runGit(t, repo, "for-each-ref", "--format=%(refname:short) %(objectname)", "refs/heads/"+branch)
	if gotRef != branch+" "+wantSHA {
		t.Fatalf("pre-existing branch after Open failure = %q; want %q", gotRef, branch+" "+wantSHA)
	}
}

func TestOpenBlocksMissionIntegrityMismatch(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	input := validMissionInput(t, repo)
	mission, _, _ := missioncompile.Compile(repo, input)
	input.RiskHint = "safe"

	result, err := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: mission.MissionID})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if result.Status != "blocked" || result.Summary != "mission integrity check failed" {
		t.Fatalf("result = %#v", result)
	}
}

func TestOpenBlocksMissionWithBlockLint(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	input := missioncompile.Input{}

	result, err := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: ""})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if result.Status != "blocked" || result.Summary != "mission has BLOCK lint; fix and recompile" {
		t.Fatalf("result = %#v", result)
	}
}

func TestOpenBlocksDirectCeremony(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	input := missioncompile.Input{Objective: "Explain this project architecture clearly", RiskHint: "safe"}

	result, err := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: ""})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if result.Status != "blocked" || result.Summary != "direct ceremony needs no workspace" {
		t.Fatalf("result = %#v", result)
	}
}

func TestOpenBlocksRepositoryWithoutCommits(t *testing.T) {
	repo := newEmptyTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	input := validMissionInput(t, repo)
	mission, _, _ := missioncompile.Compile(repo, input)

	result, err := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: mission.MissionID})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if result.Status != "blocked" || result.Summary != "repository has no commits" {
		t.Fatalf("result = %#v", result)
	}
}

func TestOpenBlocksSubmodules(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	if err := os.WriteFile(filepath.Join(repo, ".gitmodules"), []byte("[submodule \"dep\"]\n\tpath = dep\n\turl = ../dep\n"), 0o600); err != nil {
		t.Fatalf("write .gitmodules: %v", err)
	}
	input := validMissionInput(t, repo)
	mission, _, _ := missioncompile.Compile(repo, input)

	result, err := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: mission.MissionID})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if result.Status != "blocked" || result.Summary != "submodules are not supported yet" {
		t.Fatalf("result = %#v", result)
	}
}

func TestOpenWarnsOnLFS(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	if err := os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("*.bin filter=lfs diff=lfs merge=lfs -text\n"), 0o600); err != nil {
		t.Fatalf("write .gitattributes: %v", err)
	}
	input := validMissionInput(t, repo)
	mission, _, _ := missioncompile.Compile(repo, input)

	result, err := Open(context.Background(), repo, OpenInput{MissionInput: input, MissionID: mission.MissionID})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if result.Status != "ok" || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "git-lfs detected") {
		t.Fatalf("result = %#v", result)
	}
	cleanupWorktree(t, repo, result.Data.WorktreePath)
}

func TestOpenCreatesTwoIsolatedRuns(t *testing.T) {
	repo := newTestRepo(t)
	t.Setenv("HOME", t.TempDir())
	input := validMissionInput(t, repo)
	mission, _, _ := missioncompile.Compile(repo, input)
	openInput := OpenInput{MissionInput: input, MissionID: mission.MissionID}

	first, err := Open(context.Background(), repo, openInput)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	second, err := Open(context.Background(), repo, openInput)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	if first.Data.RunID == second.Data.RunID || first.Data.Branch == second.Data.Branch || first.Data.WorktreePath == second.Data.WorktreePath {
		t.Fatalf("runs are not isolated: first=%#v second=%#v", first.Data, second.Data)
	}
	cleanupWorktree(t, repo, first.Data.WorktreePath)
	cleanupWorktree(t, repo, second.Data.WorktreePath)
}

func validMissionInput(t *testing.T, repo string) missioncompile.Input {
	t.Helper()
	projectID, err := project.ID(repo)
	if err != nil {
		t.Fatalf("project.ID: %v", err)
	}
	return missioncompile.Input{
		Objective:            "Update project README fixture",
		Context:              missioncompile.Context{ProjectID: projectID},
		AcceptanceCriteria:   []string{"README fixture updated"},
		VerificationCommands: [][]string{{"go", "test", "./..."}},
		AllowedPaths:         []string{"README.md"},
		RiskHint:             "write",
	}
}

func newTestRepo(t *testing.T) string {
	t.Helper()
	repo := newEmptyTestRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "initial commit")
	return repo
}

func newEmptyTestRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.name", "Jacu Test")
	runGit(t, repo, "config", "user.email", "jacu-test@example.invalid")
	return repo
}

func runGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// #nosec G204 -- the test helper invokes a fixed binary with test-controlled argv.
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repo
	cmd.Env = testgit.Env()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func cleanupWorktree(t *testing.T, repo, worktree string) {
	t.Helper()
	runGit(t, worktree, "reset", "--hard", "HEAD")
	runGit(t, worktree, "clean", "-fd")
	runGit(t, repo, "worktree", "unlock", worktree)
	runGit(t, repo, "worktree", "remove", worktree)
}
