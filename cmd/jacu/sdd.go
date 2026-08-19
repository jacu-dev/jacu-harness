package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/capability/cleanexit"
	sddcap "github.com/jacu-dev/jacu-harness/internal/capability/sdd"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func runSDD(root string, args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		if err := printLine(stderr, "sdd: usage is sdd new <slug> | sdd lint [<directory>] [--all] [--json] [--write-lock] | sdd status | sdd close <directory>"); err != nil {
			return 2
		}
		return 2
	}
	switch args[0] {
	case "new":
		return sddNew(root, args[1:], stdout, stderr)
	case "lint":
		return sddLint(root, args[1:], stdout, stderr)
	case "status":
		return sddStatus(root, stdout, stderr)
	case "close":
		return sddClose(root, args[1:], stdout, stderr)
	default:
		if err := printFormat(stderr, "sdd: unknown subcommand %q\n", args[0]); err != nil {
			return 2
		}
		return 2
	}
}

func sddClose(root string, args []string, stdout, stderr *os.File) int {
	if len(args) != 1 {
		_ = printLine(stderr, "sdd close: usage is sdd close <directory>")
		return 2
	}
	directory := args[0]
	if !filepath.IsAbs(directory) {
		directory = filepath.Join(root, directory)
	}
	directory = filepath.Clean(directory)
	if !pathWithinProject(root, directory) {
		_ = printLine(stderr, "sdd close: target is outside the project root")
		return 2
	}
	projectRootHandle, rootErr := os.OpenRoot(root)
	if rootErr != nil {
		_ = printLine(stderr, "sdd close: open project root:", rootErr)
		return 1
	}
	defer func() { _ = projectRootHandle.Close() }()
	sddRootHandle, rootErr := projectRootHandle.OpenRoot("docs/sdd")
	if rootErr != nil {
		_ = printLine(stderr, "sdd close: open docs/sdd root:", rootErr)
		return 1
	}
	defer func() { _ = sddRootHandle.Close() }()
	base := filepath.Join(root, "docs", "sdd")
	relative, relErr := filepath.Rel(base, directory)
	if relErr != nil || filepath.IsAbs(relative) || strings.HasPrefix(relative, "..") {
		_ = printLine(stderr, "sdd close: target is outside docs/sdd")
		return 2
	}
	content, readErr := sddRootHandle.ReadFile(filepath.Join(relative, "sdd.md"))
	if readErr != nil {
		_ = printLine(stderr, "sdd close: read document:", readErr)
		return 1
	}
	lock, _ := sddRootHandle.ReadFile(filepath.Join(relative, "sdd.lock.json"))
	findings := sddcap.LintSelectedSDDContentWithLock(directory, content, lock)
	for _, finding := range findings {
		if finding.Severity == sddcap.SeverityBlock {
			_ = printFormat(stderr, "sdd close: %s %s: %s\n", finding.Severity, finding.Code, finding.Message)
			return 2
		}
	}
	archiveRel := filepath.Join("archive", filepath.Base(directory))
	if _, err := sddRootHandle.Stat(archiveRel); err != nil {
		_ = printFormat(stderr, "sdd close: archive required at %s\n", filepath.ToSlash(filepath.Join(root, "docs", "sdd", archiveRel)))
		return 2
	}
	report := cleanexit.Detect(root)
	if report.Verdict != "pass" {
		result := cleanexit.Remove(root, report)
		cleanexit.EmitTelemetry(root, result)
		if _, err := cleanexit.WriteReceipt(root, result); err != nil {
			_ = printFormat(stderr, "sdd close: write receipt: %s\n", err)
		}
		_ = printLine(stderr, "sdd close: clean exit failed")
		return 1
	}
	result := cleanexit.RemovalReport{Verdict: "pass", Removed: []string{}, Findings: []cleanexit.Finding{}}
	cleanexit.EmitTelemetry(root, result)
	if _, err := cleanexit.WriteReceipt(root, result); err != nil {
		_ = printFormat(stderr, "sdd close: write receipt: %s\n", err)
		return 1
	}
	_ = printLine(stdout, "sdd close: OK")
	return 0
}

func sddNew(root string, args []string, stdout, stderr *os.File) int {
	if len(args) != 1 || !validSlug(args[0]) {
		if err := printLine(stderr, "sdd new: usage is sdd new <lowercase-slug>"); err != nil {
			return 2
		}
		return 2
	}
	projectRootHandle, err := os.OpenRoot(root)
	if err != nil {
		_ = printLine(stderr, "sdd new: open project root:", err)
		return 1
	}
	defer func() { _ = projectRootHandle.Close() }()
	sddRootHandle, err := projectRootHandle.OpenRoot("docs/sdd")
	if err != nil {
		_ = printLine(stderr, "sdd new: open docs/sdd root:", err)
		return 1
	}
	defer func() { _ = sddRootHandle.Close() }()
	number, err := nextSDDNumberRoot(sddRootHandle)
	if err != nil {
		if writeErr := printLine(stderr, "sdd new:", err); writeErr != nil {
			return 1
		}
		return 1
	}
	name := fmt.Sprintf("%03d-%s", number, args[0])
	template, err := sddRootHandle.ReadFile(filepath.Join("templates", "sdd.md"))
	if err != nil {
		if writeErr := printLine(stderr, "sdd new: read template:", err); writeErr != nil {
			return 1
		}
		return 1
	}
	content := strings.ReplaceAll(string(template), "NNN-slug", name)
	content = strings.ReplaceAll(content, "NNN —", fmt.Sprintf("%03d —", number))
	if err := sddRootHandle.Mkdir(name, 0o750); err != nil {
		if writeErr := printLine(stderr, "sdd new: create directory:", err); writeErr != nil {
			return 1
		}
		return 1
	}
	if err := sddRootHandle.WriteFile(filepath.Join(name, "sdd.md"), []byte(content), 0o600); err != nil {
		if writeErr := printLine(stderr, "sdd new: write document:", err); writeErr != nil {
			return 1
		}
		return 1
	}
	if err := printLine(stdout, filepath.ToSlash(filepath.Join("docs", "sdd", name, "sdd.md"))); err != nil {
		return 1
	}
	return 0
}

func sddLint(root string, args []string, stdout, stderr *os.File) int {
	all, asJSON, writeLock := false, false, false
	target := ""
	for _, arg := range args {
		switch arg {
		case "--all":
			all = true
		case "--json":
			asJSON = true
		case "--write-lock":
			writeLock = true
		default:
			if strings.HasPrefix(arg, "-") || target != "" {
				if err := printLine(stderr, "sdd lint: usage is sdd lint [<directory>] [--all] [--json] [--write-lock]"); err != nil {
					return 2
				}
				return 2
			}
			target = arg
		}
	}
	if all && target != "" {
		if err := printLine(stderr, "sdd lint: usage is sdd lint [<directory>] [--all] [--json] [--write-lock]"); err != nil {
			return 2
		}
		return 2
	}
	projectRootHandle, rootErr := os.OpenRoot(root)
	if rootErr != nil {
		_ = printLine(stderr, "sdd lint: open project root:", rootErr)
		return 1
	}
	defer func() { _ = projectRootHandle.Close() }()
	sddRootHandle, rootErr := projectRootHandle.OpenRoot("docs/sdd")
	if rootErr != nil {
		_ = printLine(stderr, "sdd lint: open docs/sdd root:", rootErr)
		return 1
	}
	defer func() { _ = sddRootHandle.Close() }()
	directories, err := sddDirectoriesRoot(root, all, target, sddRootHandle)
	if err != nil {
		if writeErr := printLine(stderr, "sdd lint:", err); writeErr != nil {
			return 1
		}
		return 1
	}
	allFindings := make([]sddcap.Finding, 0)
	for _, directory := range directories {
		if err := confinedSDDPath(root, directory); err != nil {
			allFindings = append(allFindings, sddcap.Finding{Severity: sddcap.SeverityBlock, Code: "sdd_path_unavailable", Target: directory, Message: "native SDD path is not confined"})
			continue
		}
		relative, relErr := filepath.Rel(filepath.Join(root, "docs", "sdd"), directory)
		if relErr != nil || filepath.IsAbs(relative) || strings.HasPrefix(relative, "..") {
			allFindings = append(allFindings, sddcap.Finding{Severity: sddcap.SeverityBlock, Code: "sdd_path_unavailable", Target: directory, Message: "native SDD path is not confined"})
			continue
		}
		content, readErr := sddRootHandle.ReadFile(filepath.Join(relative, "sdd.md"))
		if readErr != nil {
			code, message := "sdd_path_unavailable", "native SDD document is not confined"
			if !validSDDDirectory(filepath.Base(directory)) {
				code, message = "sdd_bad_directory", "directory must match NNN-slug"
			}
			allFindings = append(allFindings, sddcap.Finding{Severity: sddcap.SeverityBlock, Code: code, Target: directory, Message: message})
			continue
		}
		document, parseErr := sddcap.Parse(content)
		if parseErr != nil {
			allFindings = append(allFindings, sddcap.Finding{Severity: sddcap.SeverityBlock, Code: "sdd_document_invalid", Target: directory, Message: "native SDD document invalid"})
			continue
		}
		if writeLock {
			if err := sddcap.WriteLockRoot(sddRootHandle, relative, document); err != nil {
				if writeErr := printLine(stderr, "sdd lint: write lock:", err); writeErr != nil {
					return 1
				}
				return 1
			}
		}
		lock, _ := sddRootHandle.ReadFile(filepath.Join(relative, "sdd.lock.json"))
		if target != "" {
			allFindings = append(allFindings, sddcap.LintSelectedSDDContentWithLock(directory, content, lock)...)
		} else {
			// Repository-wide lint only applies a write scope to the SDD whose
			// own document changed, preventing unrelated inactive SDDs from
			// claiming the same implementation changes.
			allFindings = append(allFindings, sddcap.LintSDDContentWithLock(directory, content, lock)...)
		}
	}
	if asJSON {
		if err := json.NewEncoder(stdout).Encode(allFindings); err != nil {
			if writeErr := printLine(stderr, "sdd lint: encode findings:", err); writeErr != nil {
				return 1
			}
			return 1
		}
	} else {
		for _, finding := range allFindings {
			if err := printFormat(stderr, "%s %s %s: %s\n", finding.Severity, finding.Code, finding.Target, finding.Message); err != nil {
				return 1
			}
		}
		if len(allFindings) == 0 {
			if err := printLine(stdout, "sdd lint: OK"); err != nil {
				return 1
			}
		}
	}
	for _, finding := range allFindings {
		if finding.Severity == sddcap.SeverityBlock {
			return 1
		}
	}
	return 0
}

func sddStatus(root string, stdout, stderr *os.File) int {
	projectRootHandle, rootErr := os.OpenRoot(root)
	if rootErr != nil {
		_ = printLine(stderr, "sdd status: open project root:", rootErr)
		return 1
	}
	defer func() { _ = projectRootHandle.Close() }()
	sddRootHandle, rootErr := projectRootHandle.OpenRoot("docs/sdd")
	if rootErr != nil {
		_ = printLine(stderr, "sdd status: open docs/sdd root:", rootErr)
		return 1
	}
	defer func() { _ = sddRootHandle.Close() }()
	directories, err := sddDirectoriesRoot(root, true, "", sddRootHandle)
	if err != nil {
		if writeErr := printLine(stderr, "sdd status:", err); writeErr != nil {
			return 1
		}
		return 1
	}
	summary, err := sddRepositoryRunSummary(root)
	if err != nil {
		_ = printLine(stderr, "sdd status: read run summary:", err)
		return 1
	}
	if err := printFormat(stdout, "repository_runs=%d open=%d reviewed=%d applied=%d discarded=%d corrupted=%d\n", summary.Total, summary.Open, summary.Reviewed, summary.Applied, summary.Discarded, summary.Corrupted); err != nil {
		return 1
	}
	for _, directory := range directories {
		relative, relErr := filepath.Rel(filepath.Join(root, "docs", "sdd"), directory)
		if relErr != nil || filepath.IsAbs(relative) || strings.HasPrefix(relative, "..") {
			_ = printLine(stderr, "sdd status: SDD path is outside docs/sdd")
			return 1
		}
		document, readErr := sddRootHandle.ReadFile(filepath.Join(relative, "sdd.md"))
		if readErr != nil {
			_ = printLine(stderr, "sdd status: read document:", readErr)
			return 1
		}
		lock, readErr := sddRootHandle.ReadFile(filepath.Join(relative, "sdd.lock.json"))
		if readErr != nil {
			_ = printLine(stderr, "sdd status: read lock:", readErr)
			return 1
		}
		status, err := sddcap.DeriveStatusWithLock(directory, document, lock)
		if err != nil {
			if writeErr := printLine(stderr, "sdd status:", err); writeErr != nil {
				return 1
			}
			return 1
		}
		if err := printFormat(stdout, "%s tasks_total=%d tasks_done=%d tasks_doing=%d blocks=%d changed_paths=%d\n", filepath.Base(directory), status.TasksTotal, status.TasksDone, status.TasksDoing, status.Blocks, status.ChangedPaths); err != nil {
			return 1
		}
	}
	return 0
}

type sddRunSummary struct{ Total, Open, Reviewed, Applied, Discarded, Corrupted int }

func sddRepositoryRunSummary(root string) (sddRunSummary, error) {
	runs, err := runstate.List(root)
	if err != nil {
		return sddRunSummary{}, err
	}
	summary := sddRunSummary{Total: len(runs)}
	for _, run := range runs {
		switch run.Status {
		case runstate.StatusOpen:
			summary.Open++
		case runstate.StatusReviewed:
			summary.Reviewed++
		case runstate.StatusApplied:
			summary.Applied++
		case runstate.StatusDiscarded:
			summary.Discarded++
		case runstate.StatusCorrupted:
			summary.Corrupted++
		}
	}
	return summary, nil
}

// sddDirectoriesRoot performs directory discovery through an os.Root opened
// at docs/sdd. Absolute paths are used only as stable display identifiers;
// all filesystem metadata reads are root-confined.
func sddDirectoriesRoot(root string, all bool, target string, sddRoot *os.Root) ([]string, error) {
	base := filepath.Join(root, "docs", "sdd")
	if target != "" {
		directory := target
		if !filepath.IsAbs(directory) {
			directory = filepath.Join(root, directory)
		}
		directory = filepath.Clean(directory)
		if !pathWithinProject(root, directory) || !pathWithinProject(base, directory) {
			return nil, fmt.Errorf("target is outside docs/sdd: %s", target)
		}
		relative, err := filepath.Rel(base, directory)
		if err != nil || filepath.IsAbs(relative) || strings.HasPrefix(relative, "..") {
			return nil, fmt.Errorf("target is outside docs/sdd: %s", target)
		}
		info, err := sddRoot.Stat(relative)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("target is not a directory: %s", target)
		}
		if err := confinedSDDPath(root, directory); err != nil {
			return nil, err
		}
		return []string{directory}, nil
	}
	entries, err := fs.ReadDir(sddRoot.FS(), ".")
	if err != nil {
		return nil, err
	}
	directories := make([]string, 0)
	reserved := map[string]struct{}{"archive": {}, "specs": {}, "templates": {}}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, isReserved := reserved[entry.Name()]; isReserved {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if !all && !strings.HasPrefix(entry.Name(), "001-") {
			continue
		}
		directory := filepath.Join(base, entry.Name())
		if err := confinedSDDPath(root, directory); err != nil {
			continue
		}
		directories = append(directories, directory)
	}
	sort.Strings(directories)
	if len(directories) == 0 {
		return nil, fmt.Errorf("no native SDD directory found")
	}
	return directories, nil
}

// confinedSDDPath rejects symlinked components before any native SDD I/O. The
// project root and target are local, repository-owned paths; all components are
// inspected with Lstat so a link cannot redirect the document or lock outside.
func confinedSDDPath(root, directory string) error {
	base := filepath.Join(root, "docs", "sdd")
	if !pathWithinProject(root, directory) || !pathWithinProject(base, directory) {
		return fmt.Errorf("native SDD path is outside docs/sdd")
	}
	current := base
	relative, err := filepath.Rel(base, directory)
	if err != nil {
		return err
	}
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("native SDD path contains symlink")
		}
	}
	directoryRoot, openErr := os.OpenRoot(directory)
	if openErr != nil {
		return openErr
	}
	defer func() { _ = directoryRoot.Close() }()
	for _, name := range []string{"sdd.md", "sdd.lock.json"} {
		info, statErr := directoryRoot.Lstat(name)
		if statErr != nil {
			if os.IsNotExist(statErr) && (name == "sdd.lock.json" || name == "sdd.md") {
				continue
			}
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("native SDD file contains symlink")
		}
	}
	return nil
}

func nextSDDNumberRoot(root *os.Root) (int, error) {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return 0, err
	}
	used := make(map[int]bool)
	for _, entry := range entries {
		if len(entry.Name()) < 4 || entry.Name()[3] != '-' {
			continue
		}
		number, parseErr := strconv.Atoi(entry.Name()[:3])
		if parseErr == nil {
			used[number] = true
		}
	}
	for number := 1; number < 1000; number++ {
		if !used[number] {
			return number, nil
		}
	}
	return 0, fmt.Errorf("no free three-digit SDD number")
}

func validSlug(slug string) bool {
	if slug == "" || slug != strings.ToLower(slug) {
		return false
	}
	for _, character := range slug {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func validSDDDirectory(name string) bool {
	if len(name) < 5 || name[3] != '-' || strings.TrimSpace(name[4:]) == "" {
		return false
	}
	for _, character := range name[:3] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func pathWithinProject(root, path string) bool {
	absoluteRoot, rootErr := filepath.Abs(root)
	absolutePath, pathErr := filepath.Abs(path)
	if rootErr != nil || pathErr != nil {
		return false
	}
	relative, err := filepath.Rel(absoluteRoot, absolutePath)
	if err != nil || filepath.IsAbs(relative) {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))
}

func printLine(writer io.Writer, args ...any) error {
	_, err := fmt.Fprintln(writer, args...)
	return err
}

func printFormat(writer io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(writer, format, args...)
	return err
}
