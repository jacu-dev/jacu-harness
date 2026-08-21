package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/capability/clarity"
	"github.com/jacu-dev/jacu-harness/internal/capability/sdd"
)

const clarityUsage = "clarity: usage is clarity probe|ingest|verdict [--json] [--sdd PATH] [--readback PATH] [--previous-spec-bytes N] [--round N]"

func runClarity(root string, args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	if len(args) == 0 {
		return usageSurface(stderr, "", clarityUsage)
	}
	command := args[0]
	jsonOut, sddPath, readbacks, previousBytes, round, err := parseClarityArgs(args[1:])
	if err != nil {
		return usageSurface(stderr, err.Error(), clarityUsage)
	}
	switch command {
	case "probe":
		return runClarityProbe(root, sddPath, jsonOut, stdout, stderr)
	case "ingest":
		return runClarityIngest(root, sddPath, jsonOut, previousBytes, round, readbacks, stdout, stderr, stdin)
	case "verdict":
		return runClarityVerdict(root, sddPath, jsonOut, previousBytes, round, readbacks, stdout, stderr)
	default:
		return usageSurface(stderr, "clarity: unknown command "+command, clarityUsage)
	}
}

func parseClarityArgs(args []string) (jsonOut bool, sddPath string, readbacks []string, previousBytes int64, round int, err error) {
	round = 1
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			jsonOut = true
		case "--sdd":
			sddPath, index, err = clarityValue(args, index)
			if err != nil {
				return
			}
		case "--readback":
			var value string
			value, index, err = clarityValue(args, index)
			if err != nil {
				return
			}
			readbacks = append(readbacks, value)
		case "--previous-spec-bytes":
			var value string
			value, index, err = clarityValue(args, index)
			if err != nil {
				return
			}
			previousBytes, err = strconv.ParseInt(value, 10, 64)
			if err != nil || previousBytes < 0 {
				err = fmt.Errorf("--previous-spec-bytes requires a non-negative integer")
				return
			}
		case "--round":
			var value string
			value, index, err = clarityValue(args, index)
			if err != nil {
				return
			}
			parsed, parseErr := strconv.Atoi(value)
			if parseErr != nil || parsed < 1 {
				err = fmt.Errorf("--round requires a positive integer")
				return
			}
			round = parsed
		default:
			err = fmt.Errorf("unknown option %s", args[index])
			return
		}
	}
	return
}

func clarityValue(args []string, index int) (string, int, error) {
	if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") || args[index+1] == "" {
		return "", index, fmt.Errorf("%s requires a value", args[index])
	}
	return args[index+1], index + 1, nil
}

func loadClarityDocument(root, sddPath string) (sdd.Document, int64, error) {
	if sddPath == "" {
		return sdd.Document{}, 0, fmt.Errorf("--sdd is required")
	}
	raw, err := os.ReadFile(sddPath) // #nosec G304,G703 -- sddPath is the --sdd flag pointing at a living SDD
	if err != nil && root != "" && !filepath.IsAbs(sddPath) {
		raw, err = os.ReadFile(filepath.Join(root, sddPath)) // #nosec G304,G703 -- relative --sdd resolved against the project root
	}
	if err != nil {
		return sdd.Document{}, 0, err
	}
	document, err := sdd.Parse(raw)
	if err != nil {
		return sdd.Document{}, 0, err
	}
	return document, int64(len(raw)), nil
}

func runClarityProbe(root, sddPath string, jsonOut bool, stdout, stderr io.Writer) int {
	document, specBytes, err := loadClarityDocument(root, sddPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "clarity: %s\n", err)
		return 1
	}
	payload := map[string]any{
		"schema":     clarity.Schema(),
		"prompt":     clarity.ProbePrompt(),
		"spec_bytes": specBytes,
		"sdd_id":     sdd.SDDID(document),
	}
	if jsonOut {
		return writeClarityJSON(stdout, stderr, payload)
	}
	_, _ = fmt.Fprintln(stdout, clarity.ProbePrompt())
	_, _ = fmt.Fprintf(stdout, "spec_bytes=%d\n", specBytes)
	return 0
}

func runClarityIngest(root, sddPath string, jsonOut bool, previousBytes int64, round int, readbackPaths []string, stdout, stderr io.Writer, stdin io.Reader) int {
	document, specBytes, err := loadClarityDocument(root, sddPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "clarity: %s\n", err)
		return 1
	}
	var raw []byte
	if len(readbackPaths) == 1 {
		raw, err = os.ReadFile(readbackPaths[0]) // #nosec G304,G703 -- readback path is a CLI flag the owner passed to ingest
	} else if len(readbackPaths) == 0 {
		if stdin == nil {
			stdin = os.Stdin
		}
		raw, err = io.ReadAll(stdin)
	} else {
		return usageSurface(stderr, "clarity: ingest accepts at most one --readback", clarityUsage)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "clarity: %s\n", err)
		return 1
	}
	readback, ingestErr := clarity.Ingest(raw)
	if ingestErr != nil {
		_, _ = fmt.Fprintf(stderr, "clarity: %s\n", ingestErr)
		report := clarity.Report{Verdict: "fail", Round: round, SpecBytes: specBytes, Findings: []clarity.Error{asClarityError(ingestErr)}}
		if jsonOut {
			_ = writeClarityJSON(stdout, stderr, report)
		}
		return 1
	}
	report := clarity.Evaluate(document, specBytes, previousBytes, round, []clarity.Readback{readback})
	clarity.Emit(root, report)
	return writeClarityReport(stdout, stderr, jsonOut, report)
}

func runClarityVerdict(root, sddPath string, jsonOut bool, previousBytes int64, round int, paths []string, stdout, stderr io.Writer) int {
	if len(paths) != 3 {
		return usageSurface(stderr, "clarity: verdict requires exactly three --readback files", clarityUsage)
	}
	document, specBytes, err := loadClarityDocument(root, sddPath)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "clarity: %s\n", err)
		return 1
	}
	readbacks := make([]clarity.Readback, 0, 3)
	for _, path := range paths {
		raw, readErr := os.ReadFile(path) // #nosec G304,G703 -- path is a --readback file the owner passed to verdict
		if readErr != nil {
			_, _ = fmt.Fprintf(stderr, "clarity: %s\n", readErr)
			return 1
		}
		readback, ingestErr := clarity.Ingest(raw)
		if ingestErr != nil {
			_, _ = fmt.Fprintf(stderr, "clarity: %s\n", ingestErr)
			return 1
		}
		readbacks = append(readbacks, readback)
	}
	report := clarity.Evaluate(document, specBytes, previousBytes, round, readbacks)
	clarity.Emit(root, report)
	return writeClarityReport(stdout, stderr, jsonOut, report)
}

func writeClarityReport(stdout, stderr io.Writer, jsonOut bool, report clarity.Report) int {
	if jsonOut {
		if code := writeClarityJSON(stdout, stderr, report); code != 0 {
			return code
		}
	} else {
		_, _ = fmt.Fprintln(stdout, report.Verdict)
		for _, finding := range report.Findings {
			_, _ = fmt.Fprintln(stdout, finding.Error())
		}
	}
	if report.Verdict == "pass" {
		return 0
	}
	return 1
}

func writeClarityJSON(stdout, stderr io.Writer, payload any) int {
	encoded, err := json.Marshal(payload)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "clarity: %s\n", err)
		return 2
	}
	_, _ = stdout.Write(append(encoded, '\n'))
	return 0
}

func asClarityError(err error) clarity.Error {
	if typed, ok := err.(clarity.Error); ok {
		return typed
	}
	return clarity.Error{Code: clarity.CodeMalformed}
}
