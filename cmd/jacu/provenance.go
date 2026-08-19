package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/provenance"
)

func runProvenance(root string, args []string, stdout, stderr io.Writer) int {
	files, history, rng, jsonOutput := false, false, "", false
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			jsonOutput = true
		case "--files":
			files = true
		case "--history":
			history = true
			remaining := args[index+1:]
			if len(remaining) > 0 && !strings.HasPrefix(remaining[0], "--") {
				rng = remaining[0]
				index++
			}
		default:
			_, _ = fmt.Fprintf(stderr, "provenance: unknown option %s\n", args[index])
			_, _ = fmt.Fprintln(stderr, "usage: jacu provenance [--json] [--files] [--history [range]]")
			return 2
		}
	}
	if !files && !history {
		files, history = true, true
	}

	git, err := gitx.New()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "provenance:", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var report provenance.Report
	if files {
		paths, listErr := git.LsFiles(ctx, root)
		if listErr != nil {
			_, _ = fmt.Fprintln(stderr, "provenance: list files:", listErr)
			return 1
		}
		fileReport, scanErr := provenance.ScanFiles(os.DirFS(root), paths)
		if scanErr != nil && len(fileReport.Gaps) == 0 {
			_, _ = fmt.Fprintln(stderr, "provenance: scan files:", scanErr)
			return 1
		}
		report = mergeReports(report, fileReport)
	}
	if history {
		commits, logErr := git.LogCommits(ctx, root, rng)
		if logErr != nil {
			_, _ = fmt.Fprintln(stderr, "provenance: history:", logErr)
			return 1
		}
		converted := make([]provenance.Commit, 0, len(commits))
		for _, commit := range commits {
			converted = append(converted, provenance.Commit{
				Hash:           commit.Hash,
				AuthorName:     commit.AuthorName,
				AuthorEmail:    commit.AuthorEmail,
				CommitterName:  commit.CommitterName,
				CommitterEmail: commit.CommitterEmail,
				Subject:        commit.Subject,
				Body:           commit.Body,
			})
		}
		report = mergeReports(report, provenance.ScanCommits(converted))
	}

	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			_, _ = fmt.Fprintln(stderr, "provenance: encode:", err)
			return 1
		}
	} else {
		_, _ = fmt.Fprintf(stdout, "provenance: traces=%d products=%d policies=%d gaps=%d\n", report.Traces, report.Products, report.Policies, len(report.Gaps))
		for _, finding := range report.Findings {
			if finding.Class != provenance.ClassTrace {
				continue
			}
			_, _ = fmt.Fprintf(stdout, "trace %s %s %s:%d %s\n", finding.Kind, finding.Rule, finding.Path, finding.Line, finding.Excerpt)
		}
	}
	if !report.Clean() {
		return 1
	}
	return 0
}

func mergeReports(left, right provenance.Report) provenance.Report {
	left.Findings = append(left.Findings, right.Findings...)
	left.Traces += right.Traces
	left.Products += right.Products
	left.Policies += right.Policies
	left.Gaps = append(left.Gaps, right.Gaps...)
	return left
}
