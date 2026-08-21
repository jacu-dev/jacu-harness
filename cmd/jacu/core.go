package main

import (
	"fmt"
	"io"

	"github.com/jacu-dev/jacu-harness/internal/capability/memory"
	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	"github.com/jacu-dev/jacu-harness/internal/capability/orchestration"
	"github.com/jacu-dev/jacu-harness/internal/capability/projectinspect"
	reportcapability "github.com/jacu-dev/jacu-harness/internal/capability/report"
	"github.com/jacu-dev/jacu-harness/internal/capability/verify"
	"github.com/jacu-dev/jacu-harness/internal/capability/workspace"
	headlessreport "github.com/jacu-dev/jacu-harness/internal/report"
	"github.com/jacu-dev/jacu-harness/internal/reportgen"
)

func runInspect(root string, args []string, stdout, stderr io.Writer) int {
	opts, code, message := parseSurfaceArgs(args)
	if code != 0 {
		return usageSurface(stderr, message, "inspect: usage is inspect [--json] [--events] [--input JSON]")
	}
	var input projectinspect.Input
	if err := decodeSurfaceInput(opts, &input); err != nil {
		return usageSurface(stderr, err.Error(), "inspect: usage is inspect [--json] [--events] [--input JSON]")
	}
	return writeSurface(stdout, projectinspect.Run(surfaceContext(opts, stdout, stderr), root, input), opts)
}

func runCompile(root string, args []string, stdout, stderr io.Writer) int {
	opts, code, message := parseSurfaceArgs(args)
	if code != 0 {
		return usageSurface(stderr, message, "compile: usage is compile [--json] [--events] [--input JSON]")
	}
	var input missioncompile.Input
	if err := decodeSurfaceInput(opts, &input); err != nil {
		return usageSurface(stderr, err.Error(), "compile: usage is compile [--json] [--events] [--input JSON]")
	}
	return writeSurface(stdout, missioncompile.Run(surfaceContext(opts, stdout, stderr), root, input), opts)
}

func runWorkspace(root string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return usageSurface(stderr, "", "workspace: usage is workspace open|status|diff|apply|discard [--json] [--events]")
	}
	command := args[0]
	opts, code, message := parseSurfaceArgs(args[1:])
	if code != 0 {
		return usageSurface(stderr, message, "workspace: usage is workspace "+command+" [--json] [--events] [--run-id ID] [--input JSON]")
	}
	ctx := surfaceContext(opts, stdout, stderr)
	switch command {
	case "open":
		var input workspace.OpenInput
		if err := decodeSurfaceInput(opts, &input); err != nil {
			return usageSurface(stderr, err.Error(), "workspace open: usage is workspace open [--json] [--mission-id ID] [--input JSON]")
		}
		return writeSurface(stdout, workspace.RunOpen(ctx, root, input), opts)
	case "status":
		var input workspace.StatusInput
		if err := decodeSurfaceInput(opts, &input); err != nil {
			return usageSurface(stderr, err.Error(), "workspace status: usage is workspace status [--json] [--task-id ID]")
		}
		return writeSurface(stdout, workspace.RunStatus(ctx, root, input), opts)
	case "diff":
		var input workspace.DiffInput
		if err := decodeSurfaceInput(opts, &input); err != nil {
			return usageSurface(stderr, err.Error(), "workspace diff: usage is workspace diff [--json] [--run-id ID]")
		}
		return writeSurface(stdout, workspace.RunDiff(ctx, root, input), opts)
	case "apply":
		var input workspace.ApplyInput
		if err := decodeSurfaceInput(opts, &input); err != nil {
			return usageSurface(stderr, err.Error(), "workspace apply: usage is workspace apply [--json] [--run-id ID] [--approve-destructive]")
		}
		return writeSurface(stdout, workspace.RunApply(ctx, root, input, workspace.CLIHostName), opts)
	case "discard":
		var input workspace.DiscardInput
		if err := decodeSurfaceInput(opts, &input); err != nil {
			return usageSurface(stderr, err.Error(), "workspace discard: usage is workspace discard [--json] [--run-id ID] [--gc]")
		}
		return writeSurface(stdout, workspace.RunDiscard(ctx, root, input), opts)
	default:
		return usageSurface(stderr, "workspace: unknown command "+command, "workspace: usage is workspace open|status|diff|apply|discard")
	}
}

func runMemory(root string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return usageSurface(stderr, "", "memory: usage is memory save|recall [--json] [--input JSON]")
	}
	command := args[0]
	opts, code, message := parseSurfaceArgs(args[1:])
	if code != 0 {
		return usageSurface(stderr, message, "memory: usage is memory "+command+" [--json] [--input JSON]")
	}
	ctx := surfaceContext(opts, stdout, stderr)
	switch command {
	case "save":
		var input memory.Input
		if err := decodeSurfaceInput(opts, &input); err != nil {
			return usageSurface(stderr, err.Error(), "memory save: usage is memory save [--json] [--input JSON]")
		}
		return writeSurface(stdout, memory.RunSave(ctx, root, input), opts)
	case "recall":
		var input memory.RecallInput
		if err := decodeSurfaceInput(opts, &input); err != nil {
			return usageSurface(stderr, err.Error(), "memory recall: usage is memory recall [--json] [--query TEXT] [--input JSON]")
		}
		return writeSurface(stdout, memory.RunRecall(ctx, root, input), opts)
	default:
		return usageSurface(stderr, "memory: unknown command "+command, "memory: usage is memory save|recall")
	}
}

func runVerify(root string, args []string, stdout, stderr io.Writer) int {
	opts, code, message := parseSurfaceArgs(args)
	if code != 0 {
		return usageSurface(stderr, message, "verify: usage is verify [--json] [--events] [--run-id ID] [--async] [--cancel] [--task-id ID] [--input JSON]")
	}
	var input verify.Input
	if err := decodeSurfaceInput(opts, &input); err != nil {
		return usageSurface(stderr, err.Error(), "verify: usage is verify [--json] [--events] [--run-id ID] [--input JSON]")
	}
	return writeSurface(stdout, verify.Run(surfaceContext(opts, stdout, stderr), root, input), opts)
}

func runFlow(root string, args []string, stdout, stderr io.Writer) int {
	opts, code, message := parseSurfaceArgs(args)
	if code != 0 {
		return usageSurface(stderr, message, "flow: usage is flow [--json] [--events] [--run-id ID] [--async] [--input JSON]")
	}
	var input orchestration.Input
	if err := decodeSurfaceInput(opts, &input); err != nil {
		return usageSurface(stderr, err.Error(), "flow: usage is flow [--json] [--events] [--input JSON]")
	}
	return writeSurface(stdout, orchestration.Run(surfaceContext(opts, stdout, stderr), root, input), opts)
}

func runReport(root string, args []string, stdout, stderr io.Writer) int {
	jsonOnly := false
	events := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOnly = true
		case "--events":
			events = true
		default:
			return usageSurface(stderr, "report: unknown option "+arg, "report: usage is report [--json] [--events]")
		}
	}
	opts := surfaceOptions{jsonOut: jsonOnly, events: events}
	if events && !jsonOnly {
		return writeSurface(stdout, reportcapability.Run(surfaceContext(opts, stdout, stderr), root, reportcapability.Input{}), opts)
	}
	if events {
		_ = reportcapability.Run(surfaceContext(opts, stdout, stderr), root, reportcapability.Input{})
	}
	report, err := headlessreport.BuildAudit(root)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "build report failed:", err)
		return 1
	}
	if jsonOnly {
		encoded, encodeErr := headlessreport.EncodeJSON(report)
		if encodeErr != nil {
			_, _ = fmt.Fprintln(stderr, "encode", headlessreport.QualityJSONName, "failed:", encodeErr)
			return 1
		}
		_, _ = stdout.Write(append(encoded, '\n'))
		return 0
	}
	markdown, err := reportgen.Markdown(report)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "render report failed:", err)
		return 1
	}
	_, _ = fmt.Fprint(stdout, markdown)
	return 0
}
