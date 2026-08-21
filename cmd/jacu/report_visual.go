package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jacu-dev/jacu-harness/internal/reportgen"
)

const reportVisualUsage = "report: usage is report [--json] [--events] | report render --input FILE [--output FILE] [--json] | report serve --input FILE [--port N] [--json]"

func runReportVisual(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return usageSurface(stderr, "", reportVisualUsage)
	}
	command := args[0]
	jsonOut, inputPath, outputPath, port, err := parseReportVisualArgs(args[1:])
	if err != nil {
		return usageSurface(stderr, err.Error(), reportVisualUsage)
	}
	if inputPath == "" {
		return usageSurface(stderr, "report: --input is required", reportVisualUsage)
	}
	switch command {
	case "render", "export":
		return runReportRender(inputPath, outputPath, jsonOut, stdout, stderr)
	case "serve":
		return runReportServe(inputPath, port, jsonOut, stdout, stderr)
	default:
		return usageSurface(stderr, "report: unknown command "+command, reportVisualUsage)
	}
}

func parseReportVisualArgs(args []string) (jsonOut bool, inputPath, outputPath string, port int, err error) {
	port = 0
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			jsonOut = true
		case "--input":
			inputPath, index, err = reportVisualValue(args, index)
			if err != nil {
				return
			}
		case "--output":
			outputPath, index, err = reportVisualValue(args, index)
			if err != nil {
				return
			}
		case "--port":
			var raw string
			raw, index, err = reportVisualValue(args, index)
			if err != nil {
				return
			}
			if _, scanErr := fmt.Sscanf(raw, "%d", &port); scanErr != nil || port < 0 {
				err = fmt.Errorf("--port requires a non-negative integer")
				return
			}
		default:
			err = fmt.Errorf("report: unknown option %s", args[index])
			return
		}
	}
	return
}

func reportVisualValue(args []string, index int) (string, int, error) {
	if index+1 >= len(args) {
		return "", index, fmt.Errorf("%s requires a value", args[index])
	}
	return args[index+1], index + 1, nil
}

func runReportRender(inputPath, outputPath string, jsonOut bool, stdout, stderr io.Writer) int {
	doc, err := reportgen.Load(inputPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "report render:", err)
		return 1
	}
	body, err := reportgen.HTML(doc)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "report render:", err)
		return 1
	}
	if outputPath != "" {
		if err := os.WriteFile(outputPath, []byte(body), 0o600); err != nil {
			_, _ = fmt.Fprintln(stderr, "report render:", err)
			return 1
		}
	}
	if jsonOut {
		payload := map[string]any{"html_bytes": len(body), "bound_port": false}
		if outputPath != "" {
			payload["output"] = outputPath
		} else {
			payload["html"] = body
		}
		if err := json.NewEncoder(stdout).Encode(payload); err != nil {
			_, _ = fmt.Fprintln(stderr, "report render:", err)
			return 1
		}
		return 0
	}
	if outputPath == "" {
		_, _ = io.WriteString(stdout, body)
	}
	return 0
}

func runReportServe(inputPath string, port int, jsonOut bool, stdout, stderr io.Writer) int {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ready := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- reportgen.Serve(ctx, reportgen.ServeOptions{InputPath: inputPath, Addr: addr, Ready: ready})
	}()
	select {
	case bound := <-ready:
		if jsonOut {
			_ = json.NewEncoder(stdout).Encode(map[string]any{"addr": bound, "host": "127.0.0.1"})
		} else {
			_, _ = fmt.Fprintln(stdout, bound)
		}
		err := <-errCh
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "report serve:", err)
			return 1
		}
		return 0
	case err := <-errCh:
		_, _ = fmt.Fprintln(stderr, "report serve:", err)
		return 1
	}
}

func isReportVisualCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch strings.ToLower(args[0]) {
	case "render", "export", "serve":
		return true
	default:
		return false
	}
}
