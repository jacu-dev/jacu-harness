package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	sddcap "github.com/jacu-dev/jacu-harness/internal/capability/sdd"
)

const (
	contextUsage            = "context: usage is context --sdd [--json]"
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
