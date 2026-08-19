package cleanexit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

var failureClasses = map[string]struct{}{
	"branch_local": {}, "branch_remote": {}, "worktree": {}, "untracked": {},
	"stash": {}, "run_open": {}, "main_mismatch": {},
}

const gitQueryTimeout = 10 * time.Second

type Finding struct {
	Class  string
	Target string
	Detail string
	Locked bool
}

type Report struct {
	Verdict  string
	Findings []Finding
}

func Detect(project string) Report {
	report := Report{Verdict: "pass", Findings: []Finding{}}
	add := func(class, target, detail string) {
		if _, ok := failureClasses[class]; !ok {
			class = "main_mismatch"
		}
		report.Findings = append(report.Findings, Finding{Class: class, Target: target, Detail: detail})
		report.Verdict = "fail"
	}
	if _, err := os.Stat(filepath.Join(project, ".git")); err != nil {
		add("main_mismatch", project, "project git state unavailable")
		return report
	}

	branch, err := gitOutputChecked(project, "branch", "--show-current")
	if err != nil {
		add("main_mismatch", project, "project git state unavailable")
		return report
	}
	if branch != "" && branch != "main" {
		add("branch_local", branch, "current branch is not main")
	}
	status, err := gitOutputChecked(project, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		add("main_mismatch", project, "project git state unavailable")
		return report
	}
	if status != "" {
		for _, line := range strings.Split(strings.TrimSpace(status), "\n") {
			if strings.HasPrefix(line, "?? ") {
				add("untracked", strings.TrimSpace(strings.TrimPrefix(line, "?? ")), "user-created untracked path preserved")
			}
		}
	}
	stash, err := gitOutputChecked(project, "stash", "list")
	if err != nil {
		add("main_mismatch", project, "project git state unavailable")
		return report
	}
	if stash != "" {
		add("stash", "stash", "stash entries remain")
	}
	remote, err := gitOutputChecked(project, "branch", "-r", "--contains", "HEAD")
	if err != nil {
		add("main_mismatch", project, "project git state unavailable")
		return report
	}
	if branch != "" && remote != "" && branch != "main" {
		add("branch_remote", branch, "remote branch remains")
	}
	runs, err := runstate.List(project)
	if err != nil {
		add("main_mismatch", project, "project run state unavailable")
		return report
	}
	for _, run := range runs {
		if run.Status == runstate.StatusOpen || run.Status == runstate.StatusReviewed {
			add("run_open", run.RunID, "run remains open")
		}
	}
	worktrees, err := gitOutputChecked(project, "worktree", "list", "--porcelain")
	if err != nil {
		add("main_mismatch", project, "project git state unavailable")
		return report
	}
	if worktrees != "" {
		known := map[string]struct{}{}
		for _, run := range runs {
			if run.Worktree != "" {
				known[filepath.Clean(run.Worktree)] = struct{}{}
			}
		}
		lines := strings.Split(worktrees, "\n")
		primary := true
		for index := 0; index < len(lines); index++ {
			if !strings.HasPrefix(lines[index], "worktree ") {
				continue
			}
			// Git porcelain lists the primary worktree first. Do not compare a
			// lexical temp/symlink path against Git's canonical physical path.
			if primary {
				primary = false
				continue
			}
			path := filepath.Clean(strings.TrimPrefix(lines[index], "worktree "))
			if _, ok := known[path]; !ok {
				add("worktree", path, "worktree is not referenced by an open run")
				for detailIndex := index + 1; detailIndex < len(lines) && !strings.HasPrefix(lines[detailIndex], "worktree "); detailIndex++ {
					if strings.HasPrefix(lines[detailIndex], "locked") {
						report.Findings[len(report.Findings)-1].Locked = true
						break
					}
				}
			}
		}
	}
	return report
}

func gitOutput(project string, args ...string) string {
	output, _ := gitOutputChecked(project, args...)
	return output
}

func gitOutputChecked(project string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitQueryTimeout)
	defer cancel()
	// #nosec G204 -- git is fixed and project is the validated repository root; args are fixed detector queries.
	command := exec.CommandContext(ctx, "git", append([]string{"-C", project}, args...)...)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
