package workspace

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/project"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
	"github.com/jacu-dev/jacu-harness/internal/userstate"
)

type DiscardInput struct {
	RunID string `json:"run_id,omitempty"`
	GC    bool   `json:"gc,omitempty"`
}

type DiscardedRun struct {
	RunID         string   `json:"run_id"`
	ArchivePatch  string   `json:"archive_patch"`
	ArchiveDigest string   `json:"archive_digest"`
	Actions       []string `json:"actions"`
}

type DiscardFailure struct {
	RunID         string   `json:"run_id"`
	Error         string   `json:"error"`
	ArchivePatch  string   `json:"archive_patch"`
	ArchiveDigest string   `json:"archive_digest"`
	Actions       []string `json:"actions"`
}

type DiscardData struct {
	Runs     []DiscardedRun   `json:"runs"`
	Failures []DiscardFailure `json:"failures"`
}

type DiscardResult struct {
	Status      string
	Summary     string
	Data        DiscardData
	Warnings    []string
	NextActions []string
}

func Discard(ctx context.Context, root string, in DiscardInput) (DiscardResult, error) {
	var result DiscardResult
	err := runstate.WithLock(root, func() error {
		var err error
		result, err = discardUnlocked(ctx, root, in)
		return err
	})
	return result, err
}

func discardUnlocked(ctx context.Context, root string, in DiscardInput) (DiscardResult, error) {
	result := DiscardResult{
		Status:      "ok",
		Summary:     "Workspace runs discarded.",
		Data:        DiscardData{Runs: []DiscardedRun{}, Failures: []DiscardFailure{}},
		Warnings:    []string{},
		NextActions: []string{},
	}
	if in.RunID == "" && !in.GC {
		return DiscardResult{}, fmt.Errorf("run_id or gc is required")
	}

	git, err := gitx.New()
	if err != nil {
		return DiscardResult{}, err
	}
	selected := make([]runstate.Run, 0)
	selectedIDs := map[string]bool{}
	if in.RunID != "" {
		if !runstate.ValidRunID(in.RunID) {
			return DiscardResult{}, fmt.Errorf("invalid run_id %q", in.RunID)
		}
		run, err := runstate.Load(root, in.RunID)
		if err != nil {
			return DiscardResult{}, err
		}
		if run.RunID != in.RunID {
			return DiscardResult{}, fmt.Errorf("loaded run_id %q does not match requested run_id %q", run.RunID, in.RunID)
		}
		selected = append(selected, run)
		selectedIDs[in.RunID] = true
	}
	if in.GC {
		runs, err := runstate.List(root)
		if err != nil {
			return DiscardResult{}, err
		}
		for _, run := range runs {
			if selectedIDs[run.RunID] {
				continue
			}
			if run.Status == runstate.StatusCorrupted && (run.Worktree == "" || run.Branch == "") {
				if !runstate.ValidRunID(run.RunID) {
					relative, err := removeInvalidRunMetadata(root, run.RunID)
					if err != nil {
						result.Data.Failures = append(result.Data.Failures, DiscardFailure{
							RunID:   run.RunID,
							Error:   err.Error(),
							Actions: []string{},
						})
						result.Warnings = append(result.Warnings, "discard failed for "+run.RunID+": "+err.Error())
						continue
					}
					result.Data.Runs = append(result.Data.Runs, DiscardedRun{
						RunID:   run.RunID,
						Actions: []string{"removed invalid run metadata " + relative},
					})
					continue
				}
				message := "run metadata is unreadable"
				result.Data.Failures = append(result.Data.Failures, DiscardFailure{
					RunID:   run.RunID,
					Error:   message,
					Actions: []string{},
				})
				result.Warnings = append(result.Warnings, "discard failed for "+run.RunID+": "+message)
				continue
			}
			exists, err := pathExists(run.Worktree)
			if err != nil {
				result.Data.Failures = append(result.Data.Failures, DiscardFailure{
					RunID:         run.RunID,
					Error:         err.Error(),
					ArchivePatch:  run.ArchivePatch,
					ArchiveDigest: run.ArchiveDigest,
					Actions:       []string{},
				})
				result.Warnings = append(result.Warnings, "discard failed for "+run.RunID+": "+err.Error())
				continue
			}
			activeOrphan := (run.Status == runstate.StatusOpen || run.Status == runstate.StatusReviewed) && !exists
			if run.Status == runstate.StatusCorrupted || activeOrphan {
				selected = append(selected, run)
				selectedIDs[run.RunID] = true
			}
		}
	}

	for _, run := range selected {
		discarded, err := discardRun(ctx, root, run, git)
		if err != nil {
			if !in.GC {
				return DiscardResult{}, err
			}
			result.Data.Failures = append(result.Data.Failures, DiscardFailure{
				RunID:         run.RunID,
				Error:         err.Error(),
				ArchivePatch:  discarded.ArchivePatch,
				ArchiveDigest: discarded.ArchiveDigest,
				Actions:       discarded.Actions,
			})
			result.Warnings = append(result.Warnings, "discard failed for "+run.RunID+": "+err.Error())
			continue
		}
		result.Data.Runs = append(result.Data.Runs, discarded)
		emitWorkspaceTelemetry(root, telemetry.EventDiscard, "discarded", "", 0, "completed", "jacu_discard", run)
	}
	if len(result.Data.Failures) > 0 {
		result.Status = "partial"
		result.Summary = "Workspace GC completed partially; some runs failed."
		result.NextActions = append(result.NextActions, "retry gc after resolving the reported run failures")
		return result, nil
	}
	if len(result.Data.Runs) == 0 {
		result.Summary = "No workspace runs discarded."
	}
	return result, nil
}

func removeInvalidRunMetadata(root, runID string) (string, error) {
	if runstate.ValidRunID(runID) || filepath.Base(runID) != runID {
		return "", fmt.Errorf("refusing invalid run metadata path %q", runID)
	}
	relative := filepath.Join(".git", "jacu", "runs", runID+".json")
	if err := os.Remove(filepath.Join(root, relative)); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return relative, nil
}

func discardRun(ctx context.Context, root string, run runstate.Run, git *gitx.Git) (result DiscardedRun, resultErr error) {
	actions := []string{}
	archivePatch := run.ArchivePatch
	archiveDigest := run.ArchiveDigest
	defer func() {
		result.RunID = run.RunID
		result.ArchivePatch = archivePatch
		result.ArchiveDigest = archiveDigest
		result.Actions = append([]string{}, actions...)
	}()
	if err := validateRunIdentity(root, run); err != nil {
		return DiscardedRun{}, err
	}
	final := run
	if err := final.Transition(runstate.StatusDiscarded); err != nil {
		return DiscardedRun{}, err
	}
	exists, pathErr := pathExists(run.Worktree)
	if pathErr != nil {
		return DiscardedRun{}, pathErr
	}
	info, infoErr := git.WorktreeInfo(ctx, root, run.Worktree)
	if infoErr != nil {
		return DiscardedRun{}, infoErr
	}
	if exists && !info.Registered {
		return DiscardedRun{}, fmt.Errorf("worktree %q is not registered; refusing discard", run.Worktree)
	}
	if info.Registered && info.Branch != run.Branch {
		return DiscardedRun{}, fmt.Errorf("worktree branch %q does not match run branch %q", info.Branch, run.Branch)
	}

	if exists {
		diff, diffErr := git.Diff(ctx, run.Worktree, run.BaseSHA)
		if diffErr != nil {
			return DiscardedRun{}, diffErr
		}
		if diff != "" {
			createdArchive, archiveErr := archiveDiff(root, run.RunID, diff)
			if archiveErr != nil {
				return DiscardedRun{}, archiveErr
			}
			archivePatch = createdArchive
			archiveDigest = archiveContentDigest(diff)
			actions = append(actions, "archived current patch to "+archivePatch)
		} else {
			archivePatch = ""
			archiveDigest = ""
		}
		run.ArchivePatch = archivePatch
		run.ArchiveDigest = archiveDigest
		if err := runstate.SaveLocked(root, run); err != nil {
			return DiscardedRun{}, err
		}
		if info.Locked {
			if err := git.WorktreeUnlock(ctx, root, run.Worktree); err != nil {
				return DiscardedRun{}, err
			}
			actions = append(actions, "unlocked worktree "+run.Worktree)
		}
		if err := git.WorktreeRemove(ctx, root, run.Worktree); err != nil {
			return DiscardedRun{}, err
		}
		actions = append(actions, "removed worktree via git "+run.Worktree)
	} else {
		if archivePatch != "" {
			if validationErr := validateArchiveReference(root, run.RunID, archivePatch, archiveDigest); validationErr != nil {
				return DiscardedRun{}, validationErr
			}
			actions = append(actions, "validated recovery archive "+archivePatch)
		}
		actions = append(actions, "confirmed worktree absent "+run.Worktree)
		if info.Locked {
			if unlockErr := git.WorktreeUnlock(ctx, root, run.Worktree); unlockErr != nil {
				return DiscardedRun{}, unlockErr
			}
			actions = append(actions, "unlocked orphaned worktree metadata "+run.Worktree)
		}
		if err := git.WorktreePrune(ctx, root); err != nil {
			return DiscardedRun{}, err
		}
		actions = append(actions, "pruned orphaned worktree metadata")
	}
	branchExists, branchErr := git.BranchExists(ctx, root, run.Branch)
	if branchErr != nil {
		return DiscardedRun{}, branchErr
	}
	if branchExists {
		if err := git.BranchDelete(ctx, root, run.Branch); err != nil {
			return DiscardedRun{}, err
		}
		actions = append(actions, "deleted branch "+run.Branch)
	} else {
		actions = append(actions, "branch already absent "+run.Branch)
	}
	if err := git.WorktreePrune(ctx, root); err != nil {
		return DiscardedRun{}, err
	}
	actions = append(actions, "pruned worktree metadata")

	final.ArchivePatch = archivePatch
	final.ArchiveDigest = archiveDigest
	if err := runstate.SaveLocked(root, final); err != nil {
		return DiscardedRun{}, err
	}
	actions = append(actions, "marked run discarded")
	return result, nil
}

func validateRunIdentity(root string, run runstate.Run) error {
	if !runstate.ValidRunID(run.RunID) {
		return fmt.Errorf("invalid run_id %q", run.RunID)
	}
	suffix := strings.TrimPrefix(run.RunID, "run_")
	expectedBranch := "jacu/run-" + suffix
	if run.Branch != expectedBranch {
		return fmt.Errorf("run %s target identity has branch %q; want %q", run.RunID, run.Branch, expectedBranch)
	}
	projectID, err := project.ID(root)
	if err != nil {
		return err
	}
	stateDir, err := userstate.Dir()
	if err != nil {
		return err
	}
	expectedWorktree := filepath.Join(stateDir, "worktrees", projectID, run.RunID)
	same, err := sameWorktreePath(expectedWorktree, run.Worktree)
	if err != nil {
		return fmt.Errorf("validate run %s worktree identity: %w", run.RunID, err)
	}
	if !same {
		return fmt.Errorf("run %s target identity has worktree %q; want %q", run.RunID, run.Worktree, expectedWorktree)
	}
	if run.BaseSHA == "" {
		return fmt.Errorf("run %s has empty base_sha", run.RunID)
	}
	return nil
}

func sameWorktreePath(expected, actual string) (bool, error) {
	if !filepath.IsAbs(actual) {
		return false, nil
	}
	expectedInfo, expectedErr := os.Stat(expected)
	actualInfo, actualErr := os.Stat(actual)
	if expectedErr == nil && actualErr == nil {
		return os.SameFile(expectedInfo, actualInfo), nil
	}
	if expectedErr != nil && !os.IsNotExist(expectedErr) {
		return false, expectedErr
	}
	if actualErr != nil && !os.IsNotExist(actualErr) {
		return false, actualErr
	}
	if (expectedErr == nil) != (actualErr == nil) {
		return false, nil
	}
	expectedNormalized, err := normalizeMissingPath(expected)
	if err != nil {
		return false, err
	}
	actualNormalized, err := normalizeMissingPath(actual)
	if err != nil {
		return false, err
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		return strings.EqualFold(expectedNormalized, actualNormalized), nil
	}
	return expectedNormalized == actualNormalized, nil
}

func normalizeMissingPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(absolute)
	missing := []string{}
	for {
		_, statErr := os.Stat(current)
		if statErr == nil {
			resolved, resolveErr := filepath.EvalSymlinks(current)
			if resolveErr != nil {
				return "", resolveErr
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(statErr) {
			return "", statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", statErr
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func archiveDiff(root, runID, diff string) (string, error) {
	return archiveDiffWithHook(root, runID, diff, nil)
}

func archiveDiffWithHook(root, runID, diff string, beforeRename func() error) (string, error) {
	if !runstate.ValidRunID(runID) {
		return "", fmt.Errorf("invalid run_id %q", runID)
	}
	relative := expectedArchivePath(runID)
	archive, openErr := openSecureArchiveRoot(root, true)
	if openErr != nil {
		return "", openErr
	}
	defer func() { _ = archive.Close() }()
	destination := filepath.Base(relative)
	if validationErr := validateArchiveDestination(archive.root, destination); validationErr != nil {
		return "", validationErr
	}
	tempName, temp, tempErr := createArchiveTemp(archive.root, runID)
	if tempErr != nil {
		return "", tempErr
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = archive.root.Remove(tempName)
		}
	}()
	if _, err := temp.Write([]byte(diff)); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if beforeRename != nil {
		if err := beforeRename(); err != nil {
			return "", err
		}
	}
	if err := archive.ValidateIdentity(); err != nil {
		return "", err
	}
	if err := validateArchiveDestination(archive.root, destination); err != nil {
		return "", err
	}
	if err := archive.root.Rename(tempName, destination); err != nil {
		return "", err
	}
	removeTemp = false
	return relative, nil
}

func expectedArchivePath(runID string) string {
	return filepath.Join(".git", "jacu", "archive", runID+".patch")
}

type secureArchiveRoot struct {
	path string
	root *os.Root
	info os.FileInfo
}

func openSecureArchiveRoot(root string, create bool) (*secureArchiveRoot, error) {
	gitDir := filepath.Join(root, ".git")
	if err := requireDirectoryNoSymlink(gitDir, false); err != nil {
		return nil, err
	}
	jacuDir := filepath.Join(gitDir, "jacu")
	if err := requireDirectoryNoSymlink(jacuDir, create); err != nil {
		return nil, err
	}
	archiveDir := filepath.Join(jacuDir, "archive")
	if err := requireDirectoryNoSymlink(archiveDir, create); err != nil {
		return nil, err
	}
	info, err := os.Lstat(archiveDir)
	if err != nil {
		return nil, err
	}
	rootHandle, err := os.OpenRoot(archiveDir)
	if err != nil {
		return nil, err
	}
	archive := &secureArchiveRoot{path: archiveDir, root: rootHandle, info: info}
	if err := archive.ValidateIdentity(); err != nil {
		_ = rootHandle.Close()
		return nil, err
	}
	return archive, nil
}

func (a *secureArchiveRoot) Close() error {
	return a.root.Close()
}

func (a *secureArchiveRoot) ValidateIdentity() error {
	if err := requireDirectoryNoSymlink(filepath.Dir(filepath.Dir(a.path)), false); err != nil {
		return err
	}
	if err := requireDirectoryNoSymlink(filepath.Dir(a.path), false); err != nil {
		return err
	}
	current, err := os.Lstat(a.path)
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() ||
		!os.SameFile(a.info, current) {
		return fmt.Errorf("archive directory identity changed during operation")
	}
	rooted, err := a.root.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(a.info, rooted) {
		return fmt.Errorf("archive root identity changed during operation")
	}
	return nil
}

func requireDirectoryNoSymlink(path string, create bool) error {
	info, lstatErr := os.Lstat(path)
	if os.IsNotExist(lstatErr) && create {
		if mkdirErr := os.Mkdir(path, 0o700); mkdirErr != nil && !os.IsExist(mkdirErr) {
			return mkdirErr
		}
		info, lstatErr = os.Lstat(path)
	}
	if lstatErr != nil {
		return lstatErr
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("archive path %q is a symlink", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("archive path %q is not a directory", path)
	}
	return nil
}

func validateArchiveDestination(root *os.Root, name string) error {
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("archive destination %q is a symlink", name)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("archive destination %q is not a regular file", name)
	}
	return nil
}

func createArchiveTemp(root *os.Root, runID string) (string, *os.File, error) {
	for range 100 {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return "", nil, err
		}
		name := "." + runID + "-" + hex.EncodeToString(random[:]) + ".tmp"
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", nil, err
		}
		return name, file, nil
	}
	return "", nil, fmt.Errorf("could not allocate archive temporary file")
}

func archiveContentDigest(content string) string {
	digest := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validateArchiveReference(root, runID, reference, expectedDigest string) error {
	expected := expectedArchivePath(runID)
	if reference != expected {
		return fmt.Errorf("archive_patch %q does not match expected %q", reference, expected)
	}
	if expectedDigest == "" {
		return fmt.Errorf("archive integrity metadata is missing for %q", reference)
	}
	archive, openErr := openSecureArchiveRoot(root, false)
	if openErr != nil {
		return openErr
	}
	defer func() { _ = archive.Close() }()
	name := filepath.Base(expected)
	info, err := archive.root.Lstat(name)
	if err != nil {
		return fmt.Errorf("validate archive_patch %q: %w", reference, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("archive_patch %q is not a non-empty regular file", reference)
	}
	file, err := archive.root.Open(name)
	if err != nil {
		return fmt.Errorf("open archive_patch %q: %w", reference, err)
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	if !openedInfo.Mode().IsRegular() || openedInfo.Size() == 0 {
		_ = file.Close()
		return fmt.Errorf("archive_patch %q changed file type during validation", reference)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := archive.ValidateIdentity(); err != nil {
		return err
	}
	actualDigest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if actualDigest != expectedDigest {
		return fmt.Errorf("archive integrity check failed for %q", reference)
	}
	return nil
}
