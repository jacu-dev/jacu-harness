package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	ctxpack "github.com/jacu-dev/jacu-harness/internal/capability/context"
	"github.com/jacu-dev/jacu-harness/internal/capability/ledger"
	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	sddcap "github.com/jacu-dev/jacu-harness/internal/capability/sdd"
)

const (
	contextUsage            = "context: usage is context --sdd [--json] | context pack|explain [--json] [--input JSON] [--budget N]"
	contextNoActiveCode     = "no_active_sdd"
	contextMultipleCode     = "multiple_active_sdd"
	contextUnreadableCode   = "sdd_unreadable"
	contextProgramFile      = "docs/sdd/PROGRAM.md"
	contextLivingPathPrefix = "docs/sdd/"
)

type admittedSDD struct {
	Path     string `json:"path"`
	Document string `json:"document"`
}

func runContext(root string, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "pack" || args[0] == "explain") {
		return runContextAdmission(root, args[0], args[1:], stdout, stderr)
	}
	jsonOut := false
	sddFlag := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOut = true
		case "--sdd":
			sddFlag = true
		default:
			return usageSurface(stderr, "context: unknown option "+arg, contextUsage)
		}
	}
	if !sddFlag {
		return usageSurface(stderr, "", contextUsage)
	}
	admitted, code, err := admitActiveLivingSDD(root)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "context: %s: %s\n", code, err.Error())
		return 1
	}
	if jsonOut {
		encoded, encodeErr := json.Marshal(admitted)
		if encodeErr != nil {
			_, _ = fmt.Fprintf(stderr, "context: %s: %s\n", contextUnreadableCode, encodeErr.Error())
			return 1
		}
		_, _ = stdout.Write(append(encoded, '\n'))
		return 0
	}
	_, _ = fmt.Fprintln(stdout, admitted.Path)
	_, _ = fmt.Fprintln(stdout)
	_, _ = io.WriteString(stdout, admitted.Document)
	if !strings.HasSuffix(admitted.Document, "\n") {
		_, _ = fmt.Fprintln(stdout)
	}
	return 0
}

func admitActiveLivingSDD(root string) (admittedSDD, string, error) {
	projectRootHandle, err := os.OpenRoot(root)
	if err != nil {
		return admittedSDD{}, contextUnreadableCode, err
	}
	defer func() { _ = projectRootHandle.Close() }()
	sddRootHandle, err := projectRootHandle.OpenRoot("docs/sdd")
	if err != nil {
		return admittedSDD{}, contextNoActiveCode, fmt.Errorf("no living SDD is active")
	}
	defer func() { _ = sddRootHandle.Close() }()
	directories, err := sddDirectoriesRoot(root, true, "", sddRootHandle)
	if err != nil {
		return admittedSDD{}, contextNoActiveCode, fmt.Errorf("no living SDD is active")
	}
	living := make([]admittedSDD, 0, len(directories))
	doing := make([]admittedSDD, 0)
	for _, directory := range directories {
		relative, relErr := filepath.Rel(filepath.Join(root, "docs", "sdd"), directory)
		if relErr != nil || filepath.IsAbs(relative) || strings.HasPrefix(relative, "..") {
			continue
		}
		document, readErr := sddRootHandle.ReadFile(filepath.Join(relative, "sdd.md"))
		if readErr != nil {
			return admittedSDD{}, contextUnreadableCode, readErr
		}
		lockBytes, readErr := sddRootHandle.ReadFile(filepath.Join(relative, "sdd.lock.json"))
		if readErr != nil && !os.IsNotExist(readErr) {
			return admittedSDD{}, contextUnreadableCode, readErr
		}
		admitted := admittedSDD{
			Path:     filepath.ToSlash(filepath.Join(contextLivingPathPrefix, relative, "sdd.md")),
			Document: string(document),
		}
		living = append(living, admitted)
		if lockHasDoing(lockBytes) {
			doing = append(doing, admitted)
		}
	}
	switch len(doing) {
	case 1:
		return doing[0], "", nil
	case 0:
		return admitFromProgram(projectRootHandle, living)
	default:
		names := make([]string, 0, len(doing))
		for _, item := range doing {
			names = append(names, item.Path)
		}
		sort.Strings(names)
		return admittedSDD{}, contextMultipleCode, fmt.Errorf("living SDDs %s", strings.Join(names, ", "))
	}
}

func lockHasDoing(content []byte) bool {
	if len(content) == 0 {
		return false
	}
	var lock sddcap.Lock
	if err := json.Unmarshal(content, &lock); err != nil {
		return false
	}
	for _, task := range lock.Tasks {
		if task.Status == "doing" {
			return true
		}
	}
	return false
}

var programQueueRow = regexp.MustCompile("(?m)^\\|\\s*\\**([0-9]{3})\\**\\s*\\|\\s*\\**`([^`]+)`\\**\\s*\\|[^|\\n]*\\|\\s*(.+?)\\s*\\|$")

func admitFromProgram(root *os.Root, living []admittedSDD) (admittedSDD, string, error) {
	program, err := root.ReadFile(contextProgramFile)
	if err != nil {
		if os.IsNotExist(err) {
			return admittedSDD{}, contextNoActiveCode, fmt.Errorf("no living SDD is active")
		}
		return admittedSDD{}, contextUnreadableCode, err
	}
	byDir := make(map[string]admittedSDD, len(living))
	for _, item := range living {
		byDir[filepath.Base(filepath.Dir(item.Path))] = item
	}
	var doing, next []admittedSDD
	for _, match := range programQueueRow.FindAllSubmatch(program, -1) {
		number := string(match[1])
		slug := string(match[2])
		state := strings.ToLower(string(match[3]))
		item, ok := byDir[number+"-"+slug]
		if !ok {
			continue
		}
		if strings.Contains(state, "doing") {
			doing = append(doing, item)
		}
		if strings.Contains(state, "next") {
			next = append(next, item)
		}
	}
	switch {
	case len(doing) == 1:
		return doing[0], "", nil
	case len(doing) > 1:
		return doing[0], "", nil
	case len(next) >= 1:
		return next[0], "", nil
	default:
		return admittedSDD{}, contextNoActiveCode, fmt.Errorf("no living SDD is active")
	}
}

func runContextAdmission(root, command string, args []string, stdout, stderr io.Writer) int {
	jsonOut := false
	budget := ctxpack.DefaultBudget
	inputJSON := ""
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			jsonOut = true
		case "--budget":
			if index+1 >= len(args) {
				return usageSurface(stderr, "--budget requires a value", contextUsage)
			}
			parsed, err := strconv.ParseInt(args[index+1], 10, 64)
			if err != nil || parsed < 0 {
				return usageSurface(stderr, "--budget requires a non-negative integer", contextUsage)
			}
			budget = parsed
			index++
		case "--input":
			if index+1 >= len(args) {
				return usageSurface(stderr, "--input requires a value", contextUsage)
			}
			inputJSON = args[index+1]
			index++
		default:
			return usageSurface(stderr, "context: unknown option "+args[index], contextUsage)
		}
	}
	var in missioncompile.Input
	if inputJSON == "" {
		return usageSurface(stderr, "context: --input is required", contextUsage)
	}
	if err := json.Unmarshal([]byte(inputJSON), &in); err != nil {
		_, _ = fmt.Fprintf(stderr, "context: malformed input\n")
		return 2
	}
	spec := ctxpack.Spec{
		Objective:      in.Objective,
		Acceptance:     in.AcceptanceCriteria,
		AllowedPaths:   in.AllowedPaths,
		ForbiddenPaths: in.ForbiddenPaths,
		RequiredPaths:  append([]string{}, in.Context.Refs...),
		Verification:   in.VerificationCommands,
		BudgetBytes:    budget,
	}
	pack, err := ctxpack.PackRoot(root, spec)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "context: %s\n", err)
		return 1
	}
	lost := ctxpack.CheckAnchors(pack)
	ctxpack.EmitAnchor(root, lost)
	decision := ledger.Decide(budget, pack, nil)
	ledger.Emit(root, decision)
	ctxpack.EmitPack(root, pack, decision.CoverageBPS, decision.ItemsRequired, decision.ItemsIncluded)
	if lost > 0 {
		decision.Verdict = ledger.VerdictRefuse
		decision.Reason = ledger.ReasonAnchors
	}
	if decision.Verdict != ledger.VerdictRefuse {
		ctxpack.EmitHandoff(root, decision.ItemsIncluded)
	}
	payload := map[string]any{"pack": pack, "decision": decision, "command": command}
	if jsonOut {
		encoded, encodeErr := json.Marshal(payload)
		if encodeErr != nil {
			_, _ = fmt.Fprintf(stderr, "context: %s\n", encodeErr)
			return 2
		}
		_, _ = stdout.Write(append(encoded, '\n'))
	} else {
		_, _ = fmt.Fprintln(stdout, decision.Verdict)
		_, _ = fmt.Fprintf(stdout, "coverage_bps=%d items_required=%d items_included=%d\n", decision.CoverageBPS, decision.ItemsRequired, decision.ItemsIncluded)
		if command == "explain" {
			for _, item := range decision.Included {
				_, _ = fmt.Fprintf(stdout, "included %s %d\n", item.Path, item.Bytes)
			}
		}
	}
	if decision.Verdict == ledger.VerdictRefuse {
		return 1
	}
	return 0
}
