package cleanexit

import (
	"context"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

type RemovalReport struct {
	Verdict  string
	Removed  []string
	Findings []Finding
}

func Remove(project string, report Report) RemovalReport {
	result := RemovalReport{Verdict: "pass", Removed: []string{}, Findings: []Finding{}}
	if len(report.Findings) == 0 {
		return result
	}
	runs, _ := runstate.List(project)
	for _, finding := range report.Findings {
		switch finding.Class {
		case "worktree":
			if !removeOwnedWorktree(project, finding.Target, runs) {
				result.Findings = append(result.Findings, finding)
				continue
			}
			result.Removed = append(result.Removed, finding.Target)
		case "branch_local":
			if !removeMergedBranch(project, finding.Target) {
				result.Findings = append(result.Findings, finding)
				continue
			}
			result.Removed = append(result.Removed, finding.Target)
		case "run_open", "untracked", "stash", "branch_remote", "main_mismatch":
			result.Findings = append(result.Findings, finding)
		}
	}
	if len(result.Findings) > 0 {
		result.Verdict = "fail"
	}
	return result
}

func removeOwnedWorktree(project, target string, runs []runstate.Run) bool {
	owned := false
	for _, run := range runs {
		if run.Worktree == target && (run.Status == runstate.StatusApplied || run.Status == runstate.StatusDiscarded) {
			owned = true
			if err := runCleanExitGit(project, "worktree", "remove", "--force", target); err != nil {
				return false
			}
			if err := runstate.Delete(project, run.RunID); err != nil {
				return false
			}
		}
	}
	return owned
}

func removeMergedBranch(project, branch string) bool {
	if !strings.HasPrefix(branch, "jacu/") {
		return false
	}
	if err := runCleanExitGit(project, "merge-base", "--is-ancestor", branch, "main"); err != nil {
		return false
	}
	return runCleanExitGit(project, "branch", "-d", branch) == nil
}

func runCleanExitGit(project string, args ...string) error {
	git, err := gitx.New()
	if err != nil {
		return err
	}
	return git.Exec(context.Background(), project, args...)
}
