package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/capability/verify"
	"github.com/jacu-dev/jacu-harness/internal/project"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
	"github.com/jacu-dev/jacu-harness/internal/userstate"
)

const (
	retentionAge = 30 * 24 * time.Hour
	maxInventory = 2048
	maxSkipped   = 512
	maxDepth     = 16
	// maxTreeStat and maxMeasureDepth bound measureTree, and are far wider
	// than the inventory budget on purpose. A worktree with `node_modules` in
	// it is 36k files nested well past sixteen levels; measuring it under the
	// 2048/16 budget reported 88 MB for 2.1 GB and said nothing about having
	// stopped early. That walk only reads metadata and never acts, so the
	// wider bound costs milliseconds and buys a number that is true.
	maxTreeStat     = 500_000
	maxMeasureDepth = 64
)

type Item struct {
	Owner            string `json:"owner"`
	State            string `json:"state"`
	Action           string `json:"action"`
	Count            int    `json:"count"`
	Bytes            int64  `json:"bytes"`
	OldestAgeSeconds int64  `json:"oldest_age_seconds"`
	NewestAgeSeconds int64  `json:"newest_age_seconds"`
	// BytesTruncated says the byte count above is a floor, not a total. A
	// report that stops early and stays silent about it is worse than no
	// report: it reads as an answer.
	BytesTruncated bool `json:"bytes_truncated,omitempty"`
	oldest         time.Time
	newest         time.Time
}

type Action struct {
	Owner  string `json:"owner"`
	Ref    string `json:"ref"`
	Reason string `json:"reason"`
	Bytes  int64  `json:"bytes"`
	path   string
	info   os.FileInfo
	parent os.FileInfo
}

type Report struct {
	Items   []Item   `json:"items"`
	Actions []Action `json:"actions"`
	Skipped []string `json:"skipped"`
	DryRun  bool     `json:"dry_run"`
	Applied bool     `json:"applied"`
	Failed  []string `json:"failed"`
}

type Options struct {
	Now          time.Time
	TelemetryDir string
	ToolchainDir string
	WorktreeDir  string
	// beforeAction is a deterministic test seam for preview/apply swap tests.
	beforeAction func(Action)
	// duringRemove swaps a child after enumeration and before descriptor-safe
	// revalidation. Production callers cannot set this unexported test seam.
	duringRemove func(string)
}

type storageRoots struct {
	runs, archive, tasks, telemetry, toolchain, worktrees string
}

func Inspect(root string) Report { return buildWithOptions(root, Options{}, false) }
func InspectWithOptions(root string, options Options) Report {
	return buildWithOptions(root, options, false)
}
func Prune(root string, apply bool) Report { return PruneWithOptions(root, Options{}, apply) }

func PruneWithOptions(root string, options Options, apply bool) Report {
	report := buildWithOptions(root, options, apply)
	if !apply {
		report.DryRun = true
		return report
	}
	report.DryRun = false
	roots, err := resolveRoots(root, options)
	if err != nil {
		report.Failed = appendBounded(report.Failed, "project/identity")
		return report
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	appliedArchives := map[string]bool{}
	for _, action := range report.Actions {
		if options.beforeAction != nil {
			options.beforeAction(action)
		}
		if action.Owner == "tasks" {
			if !sameDirectory(action, roots.tasks) {
				report.Skipped = appendBounded(report.Skipped, action.Ref)
				continue
			}
			// This is the only task mutation. The owning verify package rechecks
			// statuses, ages, compaction and the 1,000-record cap under its lock.
			if err := verify.RetainTasksAt(root, now); err != nil {
				report.Failed = appendBounded(report.Failed, action.Ref)
			}
			continue
		}
		// Filesystem/run actions use one runstate lock per action. Task
		// retention is deliberately outside this block because it locks itself.
		err := runstate.WithLock(root, func() error {
			if !revalidate(root, roots, action, now, appliedArchives) {
				report.Skipped = appendBounded(report.Skipped, action.Ref)
				return nil
			}
			if err := applyAction(action, options.duringRemove); err != nil {
				return err
			}
			if action.Owner == "archive" {
				runID := strings.TrimSuffix(strings.TrimPrefix(action.Ref, "archive/"), ".patch")
				appliedArchives[runID] = true
			}
			return nil
		})
		if err != nil {
			report.Failed = appendBounded(report.Failed, action.Ref)
		}
	}
	report.Skipped = sortedUnique(report.Skipped)
	report.Failed = sortedUnique(report.Failed)
	report.Applied = len(report.Failed) == 0
	return report
}

func buildWithOptions(root string, options Options, apply bool) Report {
	report := Report{Items: []Item{}, Actions: []Action{}, Skipped: []string{}, Failed: []string{}, DryRun: !apply}
	roots, err := resolveRoots(root, options)
	if err != nil {
		report.Failed = append(report.Failed, "project/identity")
		return report
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runs := reportRuns(root, roots, &report, now)
	reportArchives(root, roots, runs, &report, now)
	reportTasks(roots.tasks, &report, now)
	reportTelemetry(roots.telemetry, &report, now)
	reportToolchain(root, roots, &report, now)
	reportWorktrees(roots.worktrees, &report, now)
	sort.Slice(report.Items, func(i, j int) bool { return report.Items[i].Owner < report.Items[j].Owner })
	sort.Slice(report.Actions, func(i, j int) bool {
		left, right := actionOrder(report.Actions[i].Owner), actionOrder(report.Actions[j].Owner)
		if left != right {
			return left < right
		}
		return report.Actions[i].Ref < report.Actions[j].Ref
	})
	report.Skipped = sortedUnique(report.Skipped)
	report.Failed = sortedUnique(report.Failed)
	return report
}

func resolveRoots(root string, options Options) (storageRoots, error) {
	projectID, err := project.ID(root)
	if err != nil {
		return storageRoots{}, err
	}
	base := filepath.Join(root, ".git", "jacu")
	roots := storageRoots{runs: filepath.Join(base, "runs"), archive: filepath.Join(base, "archive"), tasks: filepath.Join(base, "tasks")}
	roots.telemetry = options.TelemetryDir
	if roots.telemetry == "" {
		roots.telemetry = telemetry.NewStore().Directory()
	}
	roots.toolchain = options.ToolchainDir
	roots.worktrees = options.WorktreeDir
	if roots.toolchain == "" || roots.worktrees == "" {
		stateDir, stateErr := userstate.Dir()
		if stateErr != nil {
			return storageRoots{}, stateErr
		}
		if roots.toolchain == "" {
			roots.toolchain = filepath.Join(stateDir, "toolchain-home", projectID)
		}
		if roots.worktrees == "" {
			roots.worktrees = filepath.Join(stateDir, "worktrees", projectID)
		}
	}
	return roots, nil
}

type ownedRun struct {
	run  runstate.Run
	path string
	info os.FileInfo
}

func reportRuns(root string, roots storageRoots, report *Report, now time.Time) map[string]ownedRun {
	item := Item{Owner: "runs", State: "owned", Action: "retain"}
	result := map[string]ownedRun{}
	entries, err := safeReadDir(roots.runs)
	if err != nil {
		report.Items = append(report.Items, item)
		return result
	}
	parent, _ := os.Lstat(roots.runs)
	for _, entry := range entries {
		if item.Count >= maxInventory {
			addSkip(report, "runs/output-cap")
			break
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			addSkip(report, "runs/unknown")
			continue
		}
		path := filepath.Join(roots.runs, entry.Name())
		info, statErr := regularWithin(path, roots.runs)
		item.Count++
		if statErr != nil {
			addSkip(report, "runs/unsafe")
			continue
		}
		item.observe(info.ModTime(), now)
		item.Bytes += info.Size()
		runID := strings.TrimSuffix(entry.Name(), ".json")
		run, loadErr := runstate.Load(root, runID)
		if loadErr != nil || !terminalAgedRun(run, info, roots, now) {
			addSkip(report, "runs/"+safeRef(runID))
			continue
		}
		owned := ownedRun{run: run, path: path, info: info}
		result[runID] = owned
		if run.ArchivePatch == "" && run.ArchiveDigest == "" {
			report.Actions = append(report.Actions, runAction(owned, parent))
		}
	}
	report.Items = append(report.Items, item)
	return result
}

func reportArchives(root string, roots storageRoots, runs map[string]ownedRun, report *Report, now time.Time) {
	item := Item{Owner: "archive", State: "owned", Action: "retain"}
	entries, err := safeReadDir(roots.archive)
	if err != nil {
		report.Items = append(report.Items, item)
		return
	}
	parent, _ := os.Lstat(roots.archive)
	matched := map[string]bool{}
	for _, entry := range entries {
		if item.Count >= maxInventory {
			addSkip(report, "archive/output-cap")
			break
		}
		path := filepath.Join(roots.archive, entry.Name())
		info, statErr := regularWithin(path, roots.archive)
		item.Count++
		if statErr != nil {
			addSkip(report, "archive/unsafe")
			continue
		}
		item.observe(info.ModTime(), now)
		item.Bytes += info.Size()
		runID := strings.TrimSuffix(entry.Name(), ".patch")
		owned, ok := runs[runID]
		if !ok || entry.Name() != runID+".patch" || !archiveMatches(root, roots, owned.run, path, info, now) {
			addSkip(report, "archive/"+safeRef(entry.Name()))
			continue
		}
		matched[runID] = true
		report.Actions = append(report.Actions, Action{Owner: "archive", Ref: "archive/" + entry.Name(), Reason: "matching-terminal-run-archive-older-than-30d", Bytes: info.Size(), path: path, info: info, parent: parent})
	}
	for runID, owned := range runs {
		if owned.run.ArchivePatch != "" && matched[runID] {
			runsParent, _ := os.Lstat(roots.runs)
			report.Actions = append(report.Actions, runAction(owned, runsParent))
		} else if owned.run.ArchivePatch != "" {
			addSkip(report, "runs/"+runID+"-archive-unproven")
		}
	}
	report.Items = append(report.Items, item)
}

func reportTasks(dir string, report *Report, now time.Time) {
	item := Item{Owner: "tasks", State: "owned", Action: "retain"}
	entries, err := safeReadDir(dir)
	if err == nil {
		for _, entry := range entries {
			if item.Count >= maxInventory {
				addSkip(report, "tasks/output-cap")
				break
			}
			path := filepath.Join(dir, entry.Name())
			info, statErr := regularWithin(path, dir)
			item.Count++
			if statErr == nil {
				item.observe(info.ModTime(), now)
			}
			if statErr != nil {
				addSkip(report, "tasks/unsafe")
				continue
			}
			item.Bytes += info.Size()
		}
		if len(entries) > 0 {
			if info, statErr := os.Lstat(dir); statErr == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				report.Actions = append(report.Actions, Action{Owner: "tasks", Ref: "tasks/shared-retention", Reason: "delegate-to-verify-retention", Bytes: item.Bytes, path: dir, info: info})
			}
		}
	}
	report.Items = append(report.Items, item)
}

func reportTelemetry(dir string, report *Report, now time.Time) {
	item := Item{Owner: "telemetry", State: "owned", Action: "retain"}
	entries, err := safeReadDir(dir)
	if err == nil {
		for _, entry := range entries {
			if item.Count >= maxInventory {
				addSkip(report, "telemetry/output-cap")
				break
			}
			path := filepath.Join(dir, entry.Name())
			info, statErr := regularWithin(path, dir)
			item.Count++
			if statErr == nil {
				item.observe(info.ModTime(), now)
			}
			if statErr != nil {
				addSkip(report, "telemetry/unsafe")
				continue
			}
			item.Bytes += info.Size()
		}
		if len(entries) > 0 {
			addSkip(report, "telemetry/store-owned-retention")
		}
	}
	report.Items = append(report.Items, item)
}

func reportToolchain(root string, roots storageRoots, report *Report, now time.Time) {
	item := Item{Owner: "toolchain", State: "owned", Action: "retain"}
	info, err := os.Lstat(roots.toolchain)
	if err == nil {
		item.Count = 1
		item.observe(info.ModTime(), now)
		bytes, newest, safe := treeStats(roots.toolchain)
		item.Bytes = bytes
		item.observe(newest, now)
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 && safe && oldTime(newest, now) && !hasActiveState(root) {
			report.Actions = append(report.Actions, Action{Owner: "toolchain", Ref: "toolchain/project-cache", Reason: "idle-project-toolchain-older-than-30d", Bytes: bytes, path: roots.toolchain, info: info})
		} else {
			addSkip(report, "toolchain/active-young-or-unsafe")
		}
	}
	report.Items = append(report.Items, item)
}

func reportWorktrees(dir string, report *Report, now time.Time) {
	item := Item{Owner: "worktrees", State: "owned", Action: "retain"}
	entries, err := safeReadDir(dir)
	if err == nil {
		if len(entries) == 0 {
			info, statErr := os.Lstat(dir)
			if statErr == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				report.Actions = append(report.Actions, Action{Owner: "worktrees", Ref: "worktrees/project-parent", Reason: "empty-owned-parent", path: dir, info: info})
			}
		}
		for _, entry := range entries {
			if item.Count >= maxInventory {
				addSkip(report, "worktrees/output-cap")
				break
			}
			item.Count++
			path := filepath.Join(dir, entry.Name())
			if info, statErr := os.Lstat(path); statErr == nil {
				item.observe(info.ModTime(), now)
			}
			// measureTree, not treeStats: a worktree is a checkout with
			// dependencies installed in it, and treeStats abandons the sum at
			// the first symlink. This inventory never removes a worktree
			// (`report-only` below), so it has no use for treeStats' safety
			// verdict — only for a size that is true.
			bytes, _, complete := measureTree(path)
			item.Bytes += bytes
			if !complete {
				item.BytesTruncated = true
			}
			addSkip(report, "worktrees/report-only")
		}
	}
	report.Items = append(report.Items, item)
}

func terminalAgedRun(run runstate.Run, info os.FileInfo, roots storageRoots, now time.Time) bool {
	if !runstate.ValidRunID(run.RunID) || (run.Status != runstate.StatusApplied && run.Status != runstate.StatusDiscarded) || run.CreatedAt.IsZero() || !oldTime(run.CreatedAt, now) || !oldTime(info.ModTime(), now) {
		return false
	}
	suffix := strings.TrimPrefix(run.RunID, "run_")
	if run.Branch != "jacu/run-"+suffix || run.BaseSHA == "" || filepath.Clean(run.Worktree) != filepath.Join(roots.worktrees, run.RunID) {
		return false
	}
	if _, err := os.Lstat(run.Worktree); err == nil || !os.IsNotExist(err) {
		return false
	}
	return true
}

func archiveMatches(root string, roots storageRoots, run runstate.Run, path string, info os.FileInfo, now time.Time) bool {
	expectedRelative := filepath.Join(".git", "jacu", "archive", run.RunID+".patch")
	if run.ArchivePatch != expectedRelative || !strings.HasPrefix(run.ArchiveDigest, "sha256:") || !oldTime(info.ModTime(), now) || filepath.Clean(path) != filepath.Join(roots.archive, run.RunID+".patch") {
		return false
	}
	content, err := readConfinedFile(root, expectedRelative)
	if err != nil || len(content) == 0 {
		return false
	}
	digest := sha256.Sum256(content)
	return run.ArchiveDigest == "sha256:"+hex.EncodeToString(digest[:])
}

func runAction(owned ownedRun, parent os.FileInfo) Action {
	return Action{Owner: "runs", Ref: "runs/" + owned.run.RunID, Reason: "terminal-owned-artifact-older-than-30d", Bytes: owned.info.Size(), path: owned.path, info: owned.info, parent: parent}
}

func revalidate(root string, roots storageRoots, action Action, now time.Time, appliedArchives map[string]bool) bool {
	switch action.Owner {
	case "archive":
		if !sameDirectoryInfo(action.parent, roots.archive) {
			return false
		}
		if _, err := regularSame(action, roots.archive); err != nil {
			return false
		}
		runID := strings.TrimSuffix(strings.TrimPrefix(action.Ref, "archive/"), ".patch")
		run, info, ok := reloadTerminalRun(root, roots, runID, now)
		return ok && archiveMatches(root, roots, run, action.path, action.info, now) && info != nil
	case "runs":
		if !sameDirectoryInfo(action.parent, roots.runs) {
			return false
		}
		if _, err := regularSame(action, roots.runs); err != nil {
			return false
		}
		runID := strings.TrimPrefix(action.Ref, "runs/")
		run, _, ok := reloadTerminalRun(root, roots, runID, now)
		if !ok {
			return false
		}
		if run.ArchivePatch != "" {
			if !appliedArchives[run.RunID] {
				return false
			}
			expected := filepath.Join(roots.archive, run.RunID+".patch")
			if _, err := os.Lstat(expected); !os.IsNotExist(err) {
				return false
			}
		}
		return true
	case "toolchain":
		if !sameDirectory(action, roots.toolchain) || hasActiveState(root) {
			return false
		}
		_, newest, safe := treeStats(roots.toolchain)
		return safe && oldTime(newest, now)
	case "worktrees":
		if !sameDirectory(action, roots.worktrees) {
			return false
		}
		entries, err := os.ReadDir(roots.worktrees)
		return err == nil && len(entries) == 0
	default:
		return false
	}
}

func reloadTerminalRun(root string, roots storageRoots, runID string, now time.Time) (runstate.Run, os.FileInfo, bool) {
	path := filepath.Join(roots.runs, runID+".json")
	info, err := regularWithin(path, roots.runs)
	if err != nil {
		return runstate.Run{}, nil, false
	}
	run, err := runstate.Load(root, runID)
	return run, info, err == nil && terminalAgedRun(run, info, roots, now)
}

func applyAction(action Action, duringRemove func(string)) error {
	switch action.Owner {
	case "archive", "runs":
		return os.Remove(action.path)
	case "toolchain":
		return removeTree(action.path, action.info, duringRemove)
	case "worktrees":
		return os.Remove(action.path)
	default:
		return fmt.Errorf("unsupported storage action %q", action.Owner)
	}
}

func hasActiveState(root string) bool {
	runs, err := runstate.List(root)
	if err != nil {
		return true
	}
	for _, run := range runs {
		if run.Status == runstate.StatusOpen || run.Status == runstate.StatusReviewed || run.Status == runstate.StatusCorrupted {
			return true
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, ".git", "jacu", "tasks"))
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return true
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return true
		}
		taskID := strings.TrimSuffix(entry.Name(), ".json")
		if !verify.ValidTaskID(taskID) {
			return true
		}
		content, readErr := readConfinedFile(root, filepath.Join(".git", "jacu", "tasks", entry.Name()))
		var task verify.Task
		if readErr != nil || json.Unmarshal(content, &task) != nil || task.TaskID != taskID {
			return true
		}
		if task.SchemaVersion != "" && task.SchemaVersion != "1" && task.SchemaVersion != verify.CurrentTaskSchemaVersion {
			return true
		}
		if !strings.HasPrefix(task.Capability, "jacu_") || (task.RunID != "" && !runstate.ValidRunID(task.RunID)) {
			return true
		}
		switch task.Status {
		case verify.TaskDone, verify.TaskFailed, verify.TaskCancelled, verify.TaskTimeout:
			continue
		case verify.TaskQueued, verify.TaskRunning:
			return true
		default:
			return true
		}
	}
	return false
}

func regularWithin(path, root string) (os.FileInfo, error) {
	if !confined(path, root) {
		return nil, errors.New("path is outside owner root")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("path is not a regular owned file")
	}
	return info, nil
}

func regularSame(action Action, root string) (os.FileInfo, error) {
	info, err := regularWithin(action.path, root)
	if err != nil || action.info == nil || !os.SameFile(action.info, info) {
		return nil, errors.New("file identity changed")
	}
	return info, nil
}

func sameDirectory(action Action, expected string) bool {
	if filepath.Clean(action.path) != filepath.Clean(expected) || action.info == nil {
		return false
	}
	info, err := os.Lstat(expected)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 && os.SameFile(action.info, info)
}

func sameDirectoryInfo(expectedInfo os.FileInfo, path string) bool {
	if expectedInfo == nil {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 && os.SameFile(expectedInfo, info)
}

func safeReadDir(path string) ([]os.DirEntry, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("unsafe directory")
	}
	return os.ReadDir(path)
}

func readConfinedFile(rootPath, name string) ([]byte, error) {
	confinedRoot, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	content, readErr := confinedRoot.ReadFile(name)
	closeErr := confinedRoot.Close()
	return content, errors.Join(readErr, closeErr)
}

func confined(path, root string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// treeStats answers a safety question, not a size question: its third return
// is false the moment anything in the tree is a symlink or otherwise not a
// plain file, and callers gate removal on it. Do not relax that to make a
// number look better — use measureTree, which is only ever read by a human.
func treeStats(path string) (int64, time.Time, bool) {
	budget := 0
	return treeStatsBounded(path, 0, &budget)
}

func treeStatsBounded(path string, depth int, budget *int) (int64, time.Time, bool) {
	if depth > maxDepth || *budget >= maxInventory {
		return 0, time.Time{}, false
	}
	(*budget)++
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return 0, time.Time{}, false
	}
	newest := info.ModTime()
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return 0, newest, false
		}
		return info.Size(), newest, true
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, time.Time{}, false
	}
	var total int64
	for _, entry := range entries {
		bytes, childNewest, safe := treeStatsBounded(filepath.Join(path, entry.Name()), depth+1, budget)
		if !safe {
			return total, newest, false
		}
		total += bytes
		if childNewest.After(newest) {
			newest = childNewest
		}
	}
	return total, newest, true
}

func removeTree(path string, expected os.FileInfo, duringRemove func(string)) error {
	if expected == nil || !expected.IsDir() || expected.Mode()&os.ModeSymlink != 0 {
		return errors.New("owned tree root identity is invalid")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(expected, opened) {
		_ = root.Close()
		return errors.New("owned tree root identity changed")
	}
	budget := 0
	removeErr := removeRootContents(root, "", 0, &budget, duringRemove)
	closeErr := root.Close()
	if removeErr != nil {
		return removeErr
	}
	if closeErr != nil {
		return closeErr
	}
	current, err := os.Lstat(path)
	if err != nil || current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(expected, current) {
		return errors.New("owned tree root changed before final removal")
	}
	return os.Remove(path)
}

func removeRootContents(root *os.Root, prefix string, depth int, budget *int, duringRemove func(string)) error {
	if depth > maxDepth {
		return errors.New("owned tree exceeds traversal cap")
	}
	if depth == 0 {
		if *budget >= maxInventory {
			return errors.New("owned tree exceeds traversal cap")
		}
		(*budget)++
	}
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	entries, readErr := directory.ReadDir(-1)
	closeErr := directory.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if *budget >= maxInventory {
			return errors.New("owned tree exceeds traversal cap")
		}
		name := entry.Name()
		before, err := root.Lstat(name)
		if err != nil || before.Mode()&os.ModeSymlink != 0 || (!before.IsDir() && !before.Mode().IsRegular()) {
			return errors.New("owned tree child is unsafe")
		}
		reference := name
		if prefix != "" {
			reference = filepath.Join(prefix, name)
		}
		if duringRemove != nil {
			duringRemove(reference)
		}
		current, err := root.Lstat(name)
		if err != nil || current.Mode()&os.ModeSymlink != 0 || (!current.IsDir() && !current.Mode().IsRegular()) || !os.SameFile(before, current) {
			return errors.New("owned tree child identity changed")
		}
		(*budget)++
		if current.IsDir() {
			child, openErr := root.OpenRoot(name)
			if openErr != nil {
				return openErr
			}
			opened, statErr := child.Stat(".")
			if statErr != nil || !os.SameFile(current, opened) {
				_ = child.Close()
				return errors.New("owned tree child root identity changed")
			}
			if removeErr := removeRootContents(child, reference, depth+1, budget, duringRemove); removeErr != nil {
				recursiveCloseErr := child.Close()
				return errors.Join(removeErr, recursiveCloseErr)
			}
			if childCloseErr := child.Close(); childCloseErr != nil {
				return childCloseErr
			}
		}
		final, err := root.Lstat(name)
		if err != nil || final.Mode()&os.ModeSymlink != 0 || (!final.IsDir() && !final.Mode().IsRegular()) || !os.SameFile(current, final) {
			return errors.New("owned tree child changed before removal")
		}
		if err := root.Remove(name); err != nil {
			return err
		}
	}
	return nil
}

func actionOrder(owner string) int {
	switch owner {
	case "archive":
		return 0
	case "tasks":
		return 1
	case "toolchain":
		return 2
	case "runs":
		return 3
	case "worktrees":
		return 4
	default:
		return 5
	}
}

func oldTime(value, now time.Time) bool {
	return !value.IsZero() && !value.After(now.Add(-retentionAge))
}

func (item *Item) observe(value, now time.Time) {
	if value.IsZero() {
		return
	}
	if item.oldest.IsZero() || value.Before(item.oldest) {
		item.oldest = value
	}
	if item.newest.IsZero() || value.After(item.newest) {
		item.newest = value
	}
	item.OldestAgeSeconds = max(int64(0), int64(now.Sub(item.oldest).Seconds()))
	item.NewestAgeSeconds = max(int64(0), int64(now.Sub(item.newest).Seconds()))
}
func safeRef(ref string) string {
	ref = filepath.Base(ref)
	if len(ref) > 96 {
		return ref[:96]
	}
	return ref
}
func addSkip(report *Report, ref string) { report.Skipped = appendBounded(report.Skipped, ref) }
func appendBounded(values []string, value string) []string {
	if len(values) >= maxSkipped {
		return values
	}
	return append(values, value)
}
func sortedUnique(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
func Encode(report Report) ([]byte, error) { return json.Marshal(report) }
func Validate(report Report) error {
	if report.DryRun && report.Applied {
		return fmt.Errorf("dry-run report cannot be applied")
	}
	return nil
}
