package main

import (
	"encoding/json"
	"fmt"
	"os"

	storagecap "github.com/jacu-dev/jacu-harness/internal/capability/storage"
)

func runStorage(root string, args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(stderr, "storage: usage is storage inspect|prune [--dry-run] [--json]")
		return 2
	}
	jsonOutput, dryRun, apply := false, false, false
	for _, arg := range args[1:] {
		switch arg {
		case "--json":
			jsonOutput = true
		case "--dry-run":
			dryRun = true
		case "--apply":
			apply = true
		default:
			_, _ = fmt.Fprintln(stderr, "storage: unknown option", arg)
			return 2
		}
	}
	if apply && dryRun {
		_, _ = fmt.Fprintln(stderr, "storage: --apply and --dry-run cannot be combined")
		return 2
	}
	var report storagecap.Report
	switch args[0] {
	case "inspect":
		if apply {
			_, _ = fmt.Fprintln(stderr, "storage inspect: --apply is invalid")
			return 2
		}
		report = storagecap.Inspect(root)
	case "prune":
		report = storagecap.Prune(root, apply)
	default:
		_, _ = fmt.Fprintln(stderr, "storage: unknown subcommand", args[0])
		return 2
	}
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			return 1
		}
		if len(report.Failed) > 0 {
			return 1
		}
		return 0
	}
	_, err := fmt.Fprintf(stdout, "storage %s: %d classes, %d actions\n", args[0], len(report.Items), len(report.Actions))
	if err != nil {
		return 1
	}
	if len(report.Failed) > 0 {
		return 1
	}
	return 0
}
