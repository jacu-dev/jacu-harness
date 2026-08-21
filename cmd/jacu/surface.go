package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
)

type surfaceOptions struct {
	jsonOut bool
	events  bool
	input   string
	runID   string
	flags   map[string]any
}

func parseSurfaceArgs(args []string) (surfaceOptions, int, string) {
	opts := surfaceOptions{flags: map[string]any{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			opts.jsonOut = true
		case arg == "--events":
			opts.events = true
		case arg == "--input":
			if i+1 >= len(args) {
				return opts, 2, "--input requires a JSON object"
			}
			i++
			opts.input = args[i]
		case arg == "--run-id":
			if i+1 >= len(args) {
				return opts, 2, "--run-id requires a value"
			}
			i++
			opts.runID = args[i]
		case arg == "--mission-id":
			if i+1 >= len(args) {
				return opts, 2, "--mission-id requires a value"
			}
			i++
			opts.flags["mission_id"] = args[i]
		case arg == "--task-id":
			if i+1 >= len(args) {
				return opts, 2, "--task-id requires a value"
			}
			i++
			opts.flags["task_id"] = args[i]
		case arg == "--async":
			opts.flags["async"] = true
		case arg == "--cancel":
			opts.flags["cancel"] = true
		case arg == "--gc":
			opts.flags["gc"] = true
		case arg == "--approve-destructive":
			opts.flags["approve_destructive"] = true
		case arg == "--query":
			if i+1 >= len(args) {
				return opts, 2, "--query requires a value"
			}
			i++
			opts.flags["query"] = args[i]
		case strings.HasPrefix(arg, "{"):
			if opts.input != "" {
				return opts, 2, "multiple JSON inputs"
			}
			opts.input = arg
		default:
			return opts, 2, "unknown option " + arg
		}
	}
	return opts, 0, ""
}

func decodeSurfaceInput(opts surfaceOptions, dest any) error {
	raw := opts.input
	if raw == "" {
		raw = "{}"
	}
	if err := json.Unmarshal([]byte(raw), dest); err != nil {
		return fmt.Errorf("input JSON: %w", err)
	}
	if opts.runID != "" {
		opts.flags["run_id"] = opts.runID
	}
	if len(opts.flags) == 0 {
		return nil
	}
	encoded, err := json.Marshal(dest)
	if err != nil {
		return err
	}
	var merged map[string]any
	if err := json.Unmarshal(encoded, &merged); err != nil {
		return err
	}
	for key, value := range opts.flags {
		merged[key] = value
	}
	encoded, err = json.Marshal(merged)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, dest)
}

func surfaceContext(opts surfaceOptions, stdout, stderr io.Writer) context.Context {
	ctx := context.Background()
	if !opts.events {
		return ctx
	}
	writer := stdout
	if opts.jsonOut {
		writer = stderr
	}
	return capabilityruntime.WithLiveEvents(ctx, writer)
}

func writeSurface(stdout io.Writer, result capabilityruntime.Result, opts surfaceOptions) int {
	if opts.jsonOut || opts.events {
		encoded, err := capabilityruntime.MarshalEnvelope(result)
		if err != nil {
			return 1
		}
		_, _ = stdout.Write(append(encoded, '\n'))
		return capabilityruntime.ExitCode(result.Status)
	}
	_, _ = fmt.Fprintf(stdout, "%s  %s\n", result.Status, result.Summary)
	return capabilityruntime.ExitCode(result.Status)
}

func usageSurface(stderr io.Writer, message, usage string) int {
	if message != "" {
		_, _ = fmt.Fprintln(stderr, message)
	}
	_, _ = fmt.Fprintln(stderr, usage)
	return 2
}
