package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/project"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/userstate"
)

type OpenInput struct {
	MissionInput missioncompile.Input `json:"mission_input"`
	MissionID    string               `json:"mission_id"`
}

type OpenData struct {
	RunID        string `json:"run_id"`
	Branch       string `json:"branch"`
	WorktreePath string `json:"worktree_path"`
	BaseSHA      string `json:"base_sha"`
}

type OpenResult struct {
	Status   string
	Summary  string
	Data     OpenData
	Warnings []string
}

func Open(ctx context.Context, root string, in OpenInput) (OpenResult, error) {
	var result OpenResult
	err := runstate.WithLock(root, func() error {
		var err error
		result, err = openUnlocked(ctx, root, in)
		return err
	})
	return result, err
}

func openUnlocked(ctx context.Context, root string, in OpenInput) (OpenResult, error) {
	mission, compileStatus, _ := missioncompile.Compile(root, in.MissionInput)
	if mission.MissionID != in.MissionID {
		return blockedOpen("mission integrity check failed"), nil
	}
	if compileStatus == "blocked" {
		return blockedOpen("mission has BLOCK lint; fix and recompile"), nil
	}
	if mission.Ceremony == "direct" {
		return blockedOpen("direct ceremony needs no workspace"), nil
	}

	git, err := gitx.New()
	if err != nil {
		return OpenResult{}, err
	}
	if !git.HasCommits(ctx, root) {
		return blockedOpen("repository has no commits"), nil
	}
	if git.HasSubmodules(ctx, root) {
		return blockedOpen("submodules are not supported yet"), nil
	}
	warnings := []string{}
	if git.UsesLFS(ctx, root) {
		warnings = append(warnings, "git-lfs detected; worktree compatibility is not guaranteed")
	}

	baseSHA, err := git.RevParseHead(ctx, root)
	if err != nil {
		return OpenResult{}, err
	}
	projectID, err := project.ID(root)
	if err != nil {
		return OpenResult{}, err
	}
	runID, suffix, err := newRunID()
	if err != nil {
		return OpenResult{}, err
	}
	stateDir, err := userstate.Dir()
	if err != nil {
		return OpenResult{}, err
	}
	worktree := filepath.Join(stateDir, "worktrees", projectID, runID)
	branch := "jacu/run-" + suffix
	if mkdirErr := os.MkdirAll(filepath.Dir(worktree), 0o700); mkdirErr != nil {
		return OpenResult{}, mkdirErr
	}
	cleanup := func(deleteBranch bool) {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = git.WorktreeUnlock(cleanupCtx, root, worktree)
		_ = git.WorktreeRemove(cleanupCtx, root, worktree)
		if deleteBranch {
			_ = git.BranchDelete(cleanupCtx, root, branch)
		}
	}
	branchCreated, err := git.WorktreeAddOwnedBranch(ctx, root, worktree, branch, baseSHA)
	if err != nil {
		cleanup(branchCreated)
		return OpenResult{}, err
	}
	if err := git.WorktreeLock(ctx, root, worktree); err != nil {
		cleanup(branchCreated)
		return OpenResult{}, err
	}
	state := runstate.Run{
		SchemaVersion: runstate.CurrentSchemaVersion,
		RunID:         runID,
		MissionID:     mission.MissionID,
		MissionInput:  in.MissionInput,
		Mission:       mission,
		Status:        runstate.StatusOpen,
		CreatedAt:     time.Now().UTC(),
		BaseSHA:       baseSHA,
		Branch:        branch,
		Worktree:      worktree,
	}
	if mission.Program != nil {
		state.ProgramID = mission.Program.ProgramID
		state.ProgramMissionIDs = append([]string{}, mission.Program.MissionIDs...)
	}
	if err := runstate.SaveLocked(root, state); err != nil {
		cleanup(branchCreated)
		return OpenResult{}, err
	}
	return OpenResult{
		Status:  "ok",
		Summary: "Workspace opened.",
		Data: OpenData{
			RunID:        runID,
			Branch:       branch,
			WorktreePath: worktree,
			BaseSHA:      baseSHA,
		},
		Warnings: warnings,
	}, nil
}

func blockedOpen(summary string) OpenResult {
	return OpenResult{Status: "blocked", Summary: summary, Warnings: []string{}}
}

func newRunID() (string, string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	suffix := hex.EncodeToString(raw[:])
	return "run_" + suffix, suffix, nil
}
