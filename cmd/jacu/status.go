package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	storagecap "github.com/jacu-dev/jacu-harness/internal/capability/storage"
)

// runStatus prints every run that still owns a worktree on disk, across every
// project — not just the one the shell happens to be in.
//
// It exists because parked work was invisible. `statusline` answers "what is
// the current run here", which reports one run and says nothing about the
// gigabytes another repository is holding. Two reviewed runs sat unapplied for
// hours with 2.1 GB behind them and nothing in the tool could say so.
//
// Exit codes:
//
//	0  the scan completed — including when there is nothing parked
//	1  the scan could not read something it owns
//	2  bad arguments
//
// The exit code deliberately does not change with the number of parked runs.
// Having work in progress is not an error, and a code that flips on content
// would make this unusable in a shell prompt.
func runStatus(args []string, stdout, stderr io.Writer, options storagecap.StatusOptions) int {
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		default:
			_, _ = fmt.Fprintln(stderr, "status: unknown option", arg)
			_, _ = fmt.Fprintln(stderr, "status: usage is status [--json]")
			return 2
		}
	}

	report := storagecap.Status(options)

	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			return 1
		}
		if len(report.Failed) > 0 {
			return 1
		}
		return 0
	}

	if report.TotalRuns == 0 {
		_, _ = fmt.Fprintln(stdout, "no parked runs.")
		if len(report.Failed) > 0 {
			_, _ = fmt.Fprintln(stderr, "status: failed at", strings.Join(report.Failed, ", "))
			return 1
		}
		return 0
	}

	for _, run := range report.Runs {
		size := formatBytes(run.Bytes)
		if run.BytesTruncated {
			size = ">" + size
		}
		flag := ""
		if run.Terminal() {
			flag = "  <- leftover worktree from a finished run"
		}
		if run.Status == storagecap.StatusOrphan {
			flag = "  <- no run claims this"
		}
		_, _ = fmt.Fprintf(stdout, "%s  %-9s  %6s  %8s  %s%s\n",
			shortRunID(run.RunID), run.Status, formatAge(run.AgeSeconds), size, run.RepoName, flag)
		if run.Objective != "" {
			_, _ = fmt.Fprintf(stdout, "  %s\n", truncateObjective(run.Objective, 96))
		}
	}

	_, _ = fmt.Fprintf(stdout, "\n%s . %s in worktrees\n",
		pluralRuns(report.TotalRuns), formatBytes(report.TotalBytes))
	if report.Truncated {
		_, _ = fmt.Fprintln(stdout, "some sizes are incomplete and marked with >")
	}
	if len(report.Failed) > 0 {
		_, _ = fmt.Fprintln(stderr, "status: failed at", strings.Join(report.Failed, ", "))
		return 1
	}
	return 0
}

func pluralRuns(n int) string {
	if n == 1 {
		return "1 parked run"
	}
	return fmt.Sprintf("%d parked runs", n)
}

func shortRunID(runID string) string {
	trimmed := strings.TrimPrefix(runID, "run_")
	if len(trimmed) > 8 {
		trimmed = trimmed[:8]
	}
	return "run_" + trimmed
}

// formatBytes reports in the unit a person decides with. Exact bytes belong in
// --json, where a machine reads them.
func formatBytes(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(bytes)/float64(1<<20))
	case bytes >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(bytes)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func formatAge(seconds int64) string {
	switch {
	case seconds >= 86400:
		return fmt.Sprintf("%dd", seconds/86400)
	case seconds >= 3600:
		return fmt.Sprintf("%dh", seconds/3600)
	case seconds >= 60:
		return fmt.Sprintf("%dmin", seconds/60)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

// truncateObjective cuts on a rune boundary: mission objectives may contain
// non-ASCII text and slicing bytes would split a rune.
func truncateObjective(objective string, limit int) string {
	objective = strings.Join(strings.Fields(objective), " ")
	runes := []rune(objective)
	if len(runes) <= limit {
		return objective
	}
	return string(runes[:limit]) + "..."
}
