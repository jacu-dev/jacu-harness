package storage

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/userstate"
)

// StatusOrphan marks a worktree on disk that no run state claims. It is the
// only status here that does not come from runstate: everything else is the
// run's own word for itself.
const StatusOrphan = "orphan"

// OpenRun is one worktree that still exists on disk, with whatever the run
// state says about it. Disk presence is the entry condition, not run status —
// an `applied` run whose worktree survived is an anomaly, and hiding it is how
// gigabytes go unnoticed.
type OpenRun struct {
	RunID          string    `json:"run_id"`
	ProjectID      string    `json:"project_id"`
	Repo           string    `json:"repo,omitempty"`
	RepoName       string    `json:"repo_name,omitempty"`
	Status         string    `json:"status"`
	Objective      string    `json:"objective,omitempty"`
	Branch         string    `json:"branch,omitempty"`
	Worktree       string    `json:"worktree"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
	AgeSeconds     int64     `json:"age_seconds"`
	Bytes          int64     `json:"bytes"`
	BytesTruncated bool      `json:"bytes_truncated"`
}

// Terminal reports whether the run reached a state that should have removed
// its worktree. A true here on a run that still has one on disk is a leak.
func (r OpenRun) Terminal() bool {
	return r.Status == string(runstate.StatusApplied) || r.Status == string(runstate.StatusDiscarded)
}

type StatusReport struct {
	Runs       []OpenRun `json:"runs"`
	TotalRuns  int       `json:"total_runs"`
	TotalBytes int64     `json:"total_bytes"`
	Truncated  bool      `json:"truncated"`
	Failed     []string  `json:"failed"`
}

type StatusOptions struct {
	Now time.Time
	// WorktreesRoot defaults to ~/.jacu-harness/worktrees. Tests set it.
	WorktreesRoot string
}

// Status walks every project's worktree directory and reports what is still
// on disk. It is deliberately global: a run parked in one repository is
// invisible from another, and invisible parked work is the failure this
// command exists to prevent.
func Status(options StatusOptions) StatusReport {
	report := StatusReport{Runs: []OpenRun{}, Failed: []string{}}
	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	root := options.WorktreesRoot
	if root == "" {
		root = filepath.Join(userstate.DirOrLocal(), "worktrees")
	}
	projects, err := safeReadDir(root)
	if err != nil {
		// A missing root is not a failure: it means nothing has ever run.
		if !os.IsNotExist(err) {
			report.Failed = append(report.Failed, "worktrees-unreadable")
		}
		return report
	}
	for _, project := range projects {
		if !project.IsDir() {
			continue
		}
		projectDir := filepath.Join(root, project.Name())
		runs, runsErr := safeReadDir(projectDir)
		if runsErr != nil {
			report.Failed = append(report.Failed, "project-unreadable/"+project.Name())
			continue
		}
		for _, entry := range runs {
			if !entry.IsDir() {
				continue
			}
			report.Runs = append(report.Runs, inspectWorktree(projectDir, project.Name(), entry.Name(), now, &report))
		}
	}
	sort.Slice(report.Runs, func(i, j int) bool {
		if report.Runs[i].Bytes != report.Runs[j].Bytes {
			return report.Runs[i].Bytes > report.Runs[j].Bytes
		}
		return report.Runs[i].RunID < report.Runs[j].RunID
	})
	report.TotalRuns = len(report.Runs)
	for _, run := range report.Runs {
		report.TotalBytes += run.Bytes
	}
	report.Failed = sortedUnique(report.Failed)
	return report
}

func inspectWorktree(projectDir, projectID, runID string, now time.Time, report *StatusReport) OpenRun {
	path := filepath.Join(projectDir, runID)
	out := OpenRun{RunID: runID, ProjectID: projectID, Worktree: path, Status: StatusOrphan}

	bytes, newest, complete := measureTree(path)
	out.Bytes = bytes
	out.BytesTruncated = !complete
	if !complete {
		report.Truncated = true
	}

	repo, ok := repoOfWorktree(path)
	if !ok {
		// No gitdir link, or it points nowhere: the directory is orphaned and
		// its age is the only thing left to report.
		out.AgeSeconds = ageSeconds(newest, now)
		return out
	}
	out.Repo = repo
	out.RepoName = filepath.Base(repo)

	run, loadErr := runstate.Load(repo, runID)
	if loadErr != nil {
		out.Status = string(runstate.StatusCorrupted)
		out.AgeSeconds = ageSeconds(newest, now)
		return out
	}
	out.Status = string(run.Status)
	out.Objective = run.MissionInput.Objective
	out.Branch = run.Branch
	out.CreatedAt = run.CreatedAt
	stamp := run.CreatedAt
	if stamp.IsZero() {
		stamp = newest
	}
	out.AgeSeconds = ageSeconds(stamp, now)
	return out
}

// repoOfWorktree reads the `.git` link a linked worktree carries and returns
// the repository that owns it. The file holds
// `gitdir: /path/to/repo/.git/worktrees/<run_id>`, so the repository is the
// prefix before `/.git/worktrees/`.
func repoOfWorktree(path string) (string, bool) {
	// Confined to the worktree: the name is fixed, but the directory came from
	// a scan and must not be able to reach outside itself through a link.
	raw, err := readConfinedFile(path, ".git")
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(raw))
	gitdir, found := strings.CutPrefix(line, "gitdir:")
	if !found {
		return "", false
	}
	gitdir = strings.TrimSpace(gitdir)
	marker := string(filepath.Separator) + filepath.Join(".git", "worktrees") + string(filepath.Separator)
	index := strings.Index(gitdir, marker)
	if index <= 0 {
		return "", false
	}
	repo := filepath.Clean(gitdir[:index])
	// Absolute and free of traversal, because it was read out of a file. The
	// repository is not probed here on purpose: runstate.Load is the next
	// call and it fails on a path that is not a repository, so a second stat
	// would only duplicate that answer with a wider file operation.
	if !filepath.IsAbs(repo) || strings.Contains(repo, "..") {
		return "", false
	}
	return repo, true
}

// measureTree answers "how much disk is this holding", and nothing else. It
// is separate from treeStats on purpose: treeStats gates removal and reports
// unsafe the moment it meets a symlink, which in a checkout with installed
// dependencies is immediately — `node_modules/.bin` is nothing but symlinks.
// Using that walk to size a worktree reported 46 MB for 1.1 GB, because the
// first symlink aborted the sum and the partial total was printed as fact.
//
// Here a symlink contributes zero bytes and is not followed — its target is
// counted where it actually lives, or lives outside and is not ours — and the
// walk carries on. `complete` is false only when the walk genuinely stopped
// early: budget spent, depth exceeded, or a directory it could not read.
func measureTree(path string) (int64, time.Time, bool) {
	budget := 0
	return measureTreeBounded(path, 0, &budget)
}

func measureTreeBounded(path string, depth int, budget *int) (int64, time.Time, bool) {
	if depth > maxMeasureDepth || *budget >= maxTreeStat {
		return 0, time.Time{}, false
	}
	(*budget)++
	info, err := os.Lstat(path)
	if err != nil {
		return 0, time.Time{}, false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, info.ModTime(), true
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return 0, info.ModTime(), true
		}
		return info.Size(), info.ModTime(), true
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, info.ModTime(), false
	}
	total, newest, complete := int64(0), info.ModTime(), true
	for _, entry := range entries {
		bytes, childNewest, childComplete := measureTreeBounded(filepath.Join(path, entry.Name()), depth+1, budget)
		total += bytes
		if childNewest.After(newest) {
			newest = childNewest
		}
		if !childComplete {
			complete = false
		}
	}
	return total, newest, complete
}

func ageSeconds(stamp, now time.Time) int64 {
	if stamp.IsZero() {
		return 0
	}
	age := now.Sub(stamp)
	if age < 0 {
		return 0
	}
	return int64(age.Seconds())
}
