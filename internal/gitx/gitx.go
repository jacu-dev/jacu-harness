package gitx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Git struct {
	bin    string
	runner CommandRunner
}

type CommandRunner interface {
	Run(ctx context.Context, bin, repo string, env []string, input string, args ...string) (string, string, error)
}

type execCommandRunner struct{}

type Snapshot struct {
	Patch   string
	Added   int
	Deleted int
	Files   []string
}

type WorktreeInfo struct {
	Registered bool
	Locked     bool
	Branch     string
}

func New() (*Git, error) {
	bin, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git executable not found: %w", err)
	}
	return NewWithRunner(bin, execCommandRunner{}), nil
}

func NewWithRunner(bin string, runner CommandRunner) *Git {
	if runner == nil {
		runner = execCommandRunner{}
	}
	return &Git{bin: bin, runner: runner}
}

func (g *Git) Version() (string, error) {
	stdout, _, err := g.run(context.Background(), "", "--version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout), nil
}

func (g *Git) RevParseHead(ctx context.Context, repo string) (string, error) {
	stdout, _, err := g.run(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout), nil
}

func (g *Git) InsideWorkTree(ctx context.Context, dir string) bool {
	stdout, _, err := g.run(ctx, dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(stdout) == "true"
}

func (g *Git) HasCommits(ctx context.Context, repo string) bool {
	_, _, err := g.run(ctx, repo, "rev-parse", "--verify", "HEAD")
	return err == nil
}

func (g *Git) HasSubmodules(ctx context.Context, repo string) bool {
	if ctx.Err() != nil {
		return false
	}
	info, err := os.Stat(filepath.Join(repo, ".gitmodules"))
	return err == nil && !info.IsDir()
}

func (g *Git) UsesLFS(ctx context.Context, repo string) bool {
	if ctx.Err() != nil {
		return false
	}
	// #nosec G304 -- repo is the already selected project root and the filename is fixed.
	content, err := os.ReadFile(filepath.Join(repo, ".gitattributes"))
	return err == nil && strings.Contains(string(content), "filter=lfs")
}

func (g *Git) WorktreeAdd(ctx context.Context, repo, path, branch, baseSHA string) error {
	_, _, err := g.run(ctx, repo, "worktree", "add", "-b", branch, path, baseSHA)
	return err
}

// WorktreeAddOwnedBranch atomically creates branch before attaching it to a
// worktree. The boolean reports whether this invocation created the branch, so
// callers can avoid deleting a branch owned by another invocation on failure.
func (g *Git) WorktreeAddOwnedBranch(ctx context.Context, repo, path, branch, baseSHA string) (bool, error) {
	if _, _, err := g.run(ctx, repo, "update-ref", "refs/heads/"+branch, baseSHA, ""); err != nil {
		return false, err
	}
	_, _, err := g.run(ctx, repo, "worktree", "add", path, branch)
	return true, err
}

func (g *Git) WorktreeLock(ctx context.Context, repo, path string) error {
	_, _, err := g.run(ctx, repo, "worktree", "lock", path)
	return err
}

func (g *Git) WorktreeUnlock(ctx context.Context, repo, path string) error {
	_, _, err := g.run(ctx, repo, "worktree", "unlock", path)
	return err
}

func (g *Git) WorktreeRemove(ctx context.Context, repo, path string) error {
	_, _, err := g.run(ctx, repo, "worktree", "remove", "--force", path)
	return err
}

func (g *Git) WorktreePrune(ctx context.Context, repo string) error {
	_, _, err := g.run(ctx, repo, "worktree", "prune")
	return err
}

func (g *Git) WorktreeInfo(ctx context.Context, repo, path string) (WorktreeInfo, error) {
	stdout, _, err := g.run(ctx, repo, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return WorktreeInfo{}, err
	}
	target := canonicalWorktreePath(path)
	info := WorktreeInfo{}
	matched := false
	for _, field := range strings.Split(stdout, "\x00") {
		if strings.HasPrefix(field, "worktree ") {
			if matched {
				return info, nil
			}
			matched = canonicalWorktreePath(strings.TrimPrefix(field, "worktree ")) == target
			if matched {
				info.Registered = true
			}
			continue
		}
		if !matched {
			continue
		}
		switch {
		case field == "locked" || strings.HasPrefix(field, "locked "):
			info.Locked = true
		case strings.HasPrefix(field, "branch refs/heads/"):
			info.Branch = strings.TrimPrefix(field, "branch refs/heads/")
		}
	}
	return info, nil
}

func canonicalWorktreePath(path string) string {
	path, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	current := filepath.Clean(path)
	missing := []string{}
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(path)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func (g *Git) Diff(ctx context.Context, worktree, baseSHA string) (string, error) {
	return g.diffWithTemporaryIndex(ctx, worktree, baseSHA, "--binary", "--no-color", "--no-ext-diff", "--no-renames", baseSHA, "--")
}

func (g *Git) DiffNumstat(ctx context.Context, worktree, baseSHA string) (int, int, []string, error) {
	stdout, err := g.diffWithTemporaryIndex(ctx, worktree, baseSHA, "--numstat", "-z", "--no-ext-diff", "--no-renames", baseSHA, "--")
	if err != nil {
		return 0, 0, nil, err
	}
	return parseNumstat(stdout)
}

func (g *Git) ReadOnlyNumstat(ctx context.Context, worktree, baseSHA string) (int, int, []string, error) {
	stdout, err := g.diffWithTemporaryIndex(ctx, worktree, baseSHA, "--numstat", "-z", "--no-ext-diff", "--no-renames", baseSHA, "--")
	if err != nil {
		return 0, 0, nil, err
	}
	return parseNumstat(stdout)
}

func (g *Git) DiffSnapshot(ctx context.Context, worktree, baseSHA string) (Snapshot, error) {
	stdout, err := g.diffWithTemporaryIndex(ctx, worktree, baseSHA, "--numstat", "-z", "--patch", "--binary", "--no-color", "--no-ext-diff", "--no-renames", baseSHA, "--")
	if err != nil {
		return Snapshot{}, err
	}
	numstat, patch := splitSnapshotOutput(stdout)
	added, deleted, files, err := parseNumstat(numstat)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Patch: patch, Added: added, Deleted: deleted, Files: files}, nil
}

func (g *Git) diffWithTemporaryIndex(ctx context.Context, worktree, baseSHA string, diffArgs ...string) (stdout string, resultErr error) {
	temporaryDir, err := os.MkdirTemp("", "jacu-review-index-")
	if err != nil {
		return "", fmt.Errorf("create temporary Git index directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(temporaryDir); resultErr == nil && err != nil {
			resultErr = fmt.Errorf("remove temporary Git index directory: %w", err)
		}
	}()

	extraEnv := []string{"GIT_INDEX_FILE=" + filepath.Join(temporaryDir, "index")}
	if _, _, err := g.runWithEnv(ctx, worktree, extraEnv, "read-tree", baseSHA); err != nil {
		return "", err
	}
	if _, _, err := g.runWithEnv(ctx, worktree, extraEnv, "add", "--intent-to-add", "-A"); err != nil {
		return "", err
	}
	stdout, _, resultErr = g.runWithEnv(ctx, worktree, extraEnv, append([]string{"diff"}, diffArgs...)...)
	return stdout, resultErr
}

func splitSnapshotOutput(output string) (string, string) {
	const marker = "\x00diff --git "
	if strings.HasPrefix(output, "diff --git ") {
		return "", output
	}
	markerAt := strings.Index(output, marker)
	if markerAt < 0 {
		return output, ""
	}
	return output[:markerAt+1], output[markerAt+1:]
}

func parseNumstat(output string) (int, int, []string, error) {
	added := 0
	deleted := 0
	files := []string{}
	for len(output) > 0 {
		recordEnd := strings.IndexByte(output, 0)
		if recordEnd < 0 {
			return 0, 0, nil, fmt.Errorf("invalid NUL-delimited git numstat output")
		}
		record := output[:recordEnd]
		output = output[recordEnd+1:]
		if record == "" {
			continue
		}
		parts := strings.SplitN(record, "\t", 3)
		if len(parts) != 3 {
			return 0, 0, nil, fmt.Errorf("invalid git numstat record %q", record)
		}
		lineAdded, err := parseNumstatCount(parts[0])
		if err != nil {
			return 0, 0, nil, fmt.Errorf("invalid git numstat additions %q: %w", parts[0], err)
		}
		lineDeleted, err := parseNumstatCount(parts[1])
		if err != nil {
			return 0, 0, nil, fmt.Errorf("invalid git numstat deletions %q: %w", parts[1], err)
		}
		added += lineAdded
		deleted += lineDeleted
		files = append(files, parts[2])
	}
	return added, deleted, files, nil
}

func parseNumstatCount(value string) (int, error) {
	if value == "-" {
		return 0, nil
	}
	return strconv.Atoi(value)
}

func (g *Git) StageAll(ctx context.Context, worktree string) error {
	_, _, err := g.run(ctx, worktree, "add", "-A")
	return err
}

func (g *Git) StageTree(ctx context.Context, worktree string) (string, error) {
	if err := g.StageAll(ctx, worktree); err != nil {
		return "", err
	}
	stdout, _, err := g.run(ctx, worktree, "write-tree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout), nil
}

func (g *Git) DiffStaged(ctx context.Context, worktree, baseSHA string) (string, error) {
	stdout, _, err := g.run(ctx, worktree, "diff", "--cached", "--binary", "--no-color", "--no-ext-diff", "--no-renames", baseSHA, "--")
	return stdout, err
}

// CaptureStagedPatch stages every file before reading the binary patch. The
// staging step is part of the fan-in contract: without it, newly created files
// are not blobs in the shared object database and git apply --3way cannot
// reconstruct their ancestor.
func (g *Git) CaptureStagedPatch(ctx context.Context, worktree, baseSHA string) (string, error) {
	if err := g.StageAll(ctx, worktree); err != nil {
		return "", err
	}
	return g.DiffStaged(ctx, worktree, baseSHA)
}

// ApplyPatch3Way applies one agent patch and reports a content conflict as a
// value. Mechanical git failures remain errors so the caller can distinguish
// an expected fan-in collision from a broken repository or command.
func (g *Git) ApplyPatch3Way(ctx context.Context, repo, patch string) (bool, error) {
	stdout, stderr, err := g.runWithInput(ctx, repo, patch, "apply", "--3way", "--index", "--whitespace=nowarn", "-")
	if err == nil {
		return true, nil
	}
	combined := stdout + "\n" + stderr
	if strings.Contains(combined, "with conflicts") || strings.Contains(combined, "patch failed") {
		return false, nil
	}
	return false, err
}

// ResetHard restores an owned fan-in target to its known base after a
// conflict. Fan-in callers must pass the run's authoritative base SHA.
func (g *Git) ResetHard(ctx context.Context, repo, baseSHA string) error {
	_, _, err := g.run(ctx, repo, "reset", "--hard", "--quiet", baseSHA)
	return err
}

func (g *Git) DiffTree(ctx context.Context, worktree, baseSHA, treeSHA string) (string, error) {
	stdout, _, err := g.run(ctx, worktree, "diff", "--binary", "--no-color", "--no-ext-diff", "--no-renames", baseSHA, treeSHA, "--")
	return stdout, err
}

func (g *Git) ResetMixed(ctx context.Context, worktree string) error {
	_, _, err := g.run(ctx, worktree, "reset", "--mixed", "--quiet", "HEAD")
	return err
}

func (g *Git) CommitAll(ctx context.Context, worktree, message string) (string, error) {
	expectedParent, err := g.RevParseHead(ctx, worktree)
	if err != nil {
		return "", err
	}
	treeSHA, err := g.StageTree(ctx, worktree)
	if err != nil {
		return "", err
	}
	return g.CommitTree(ctx, worktree, expectedParent, treeSHA, message)
}

func (g *Git) CommitTree(ctx context.Context, worktree, expectedParent, treeSHA, message string) (string, error) {
	stdout, _, err := g.runWithInput(ctx, worktree, message, "commit-tree", treeSHA, "-p", expectedParent)
	if err != nil {
		return "", err
	}
	commitSHA := strings.TrimSpace(stdout)
	if err := g.UpdateHeadCAS(ctx, worktree, commitSHA, expectedParent); err != nil {
		return "", err
	}
	return commitSHA, nil
}

func (g *Git) UpdateHeadCAS(ctx context.Context, repo, newSHA, expectedOldSHA string) error {
	_, _, err := g.run(ctx, repo, "update-ref", "HEAD", newSHA, expectedOldSHA)
	return err
}

func (g *Git) BranchDelete(ctx context.Context, repo, branch string) error {
	_, _, err := g.run(ctx, repo, "branch", "-D", "--", branch)
	return err
}

func (g *Git) BranchExists(ctx context.Context, repo, branch string) (bool, error) {
	stdout, _, err := g.run(ctx, repo, "for-each-ref", "--format=%(refname)", "refs/heads")
	if err != nil {
		return false, err
	}
	target := "refs/heads/" + branch
	for _, ref := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if ref == target {
			return true, nil
		}
	}
	return false, nil
}

func (g *Git) CountAhead(ctx context.Context, repo, baseSHA string) (int, error) {
	stdout, _, err := g.run(ctx, repo, "rev-list", "--count", baseSHA+"..HEAD")
	if err != nil {
		return 0, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(stdout))
	if err != nil {
		return 0, fmt.Errorf("invalid git ahead count %q: %w", strings.TrimSpace(stdout), err)
	}
	return count, nil
}

// Commit is one parsed git-log record. Field order matches the %x1f format in LogCommits.
type Commit struct {
	Hash           string
	AuthorName     string
	AuthorEmail    string
	CommitterName  string
	CommitterEmail string
	Subject        string
	Body           string
}

func (g *Git) LsFiles(ctx context.Context, repo string) ([]string, error) {
	stdout, _, err := g.run(ctx, repo, "ls-files", "-z")
	if err != nil {
		return nil, err
	}
	files := make([]string, 0)
	for _, path := range strings.Split(stdout, "\x00") {
		if path == "" {
			continue
		}
		files = append(files, path)
	}
	return files, nil
}

func (g *Git) LogCommits(ctx context.Context, repo, rng string) ([]Commit, error) {
	args := []string{"log", "--format=%H%x1f%an%x1f%ae%x1f%cn%x1f%ce%x1f%s%x1f%B%x1e"}
	if rng != "" {
		args = append(args, rng)
	}
	stdout, _, err := g.run(ctx, repo, args...)
	if err != nil {
		return nil, err
	}
	records := strings.Split(stdout, "\x1e")
	commits := make([]Commit, 0, len(records))
	for _, record := range records {
		record = strings.Trim(record, "\n")
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\x1f", 7)
		if len(fields) != 7 {
			return nil, fmt.Errorf("invalid git log record %q", record)
		}
		commits = append(commits, Commit{
			Hash:           fields[0],
			AuthorName:     fields[1],
			AuthorEmail:    fields[2],
			CommitterName:  fields[3],
			CommitterEmail: fields[4],
			Subject:        fields[5],
			Body:           strings.TrimSuffix(fields[6], "\n"),
		})
	}
	return commits, nil
}

func (g *Git) ArchiveHead(ctx context.Context, repo string, writer io.Writer) error {
	stdout, _, err := g.run(ctx, repo, "archive", "--format=tar", "HEAD")
	if err != nil {
		return err
	}
	_, err = io.WriteString(writer, stdout)
	return err
}

func (g *Git) run(ctx context.Context, repo string, args ...string) (string, string, error) {
	return g.runWithInput(ctx, repo, "", args...)
}

func (g *Git) runWithInput(ctx context.Context, repo, input string, args ...string) (string, string, error) {
	return g.runWithInputAndEnv(ctx, repo, input, nil, args...)
}

func (g *Git) runWithEnv(ctx context.Context, repo string, extraEnv []string, args ...string) (string, string, error) {
	return g.runWithInputAndEnv(ctx, repo, "", extraEnv, args...)
}

func (g *Git) runWithInputAndEnv(ctx context.Context, repo, input string, extraEnv []string, args ...string) (string, string, error) {
	// Ambient Git location variables override both cwd and explicit repository
	// arguments. Clean inherited values first, then append only the trusted
	// temporary-index override owned by this operation.
	runner := g.runner
	if runner == nil {
		runner = execCommandRunner{}
	}
	stdout, stderr, err := runner.Run(ctx, g.bin, repo, append(cleanGitEnv(filteredEnv()), extraEnv...), input, args...)
	if err != nil {
		return stdout, stderr, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr))
	}
	return stdout, stderr, nil
}

func (execCommandRunner) Run(ctx context.Context, bin, repo string, env []string, input string, args ...string) (string, string, error) {
	// #nosec G204 -- bin is resolved by exec.LookPath and callers supply argv
	// directly; no shell parses these arguments.
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = repo
	cmd.Env = env
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), stderr.String(), err
	}
	return stdout.String(), stderr.String(), nil
}

func filteredEnv() []string {
	keys := []string{"PATH", "HOME", "TMPDIR", "LANG"}
	cleaned := cleanGitEnv(os.Environ())
	env := make([]string, 0, len(keys))
	for _, entry := range cleaned {
		name, value, hasValue := strings.Cut(entry, "=")
		if !hasValue {
			continue
		}
		for _, key := range keys {
			if name == key {
				env = append(env, key+"="+value)
				break
			}
		}
	}
	return env
}

var gitLocationVariables = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_COMMON_DIR",
	"GIT_NAMESPACE",
	"GIT_PREFIX",
	"GIT_CEILING_DIRECTORIES",
}

func cleanGitEnv(environment []string) []string {
	blocked := make(map[string]struct{}, len(gitLocationVariables))
	for _, name := range gitLocationVariables {
		blocked[name] = struct{}{}
	}
	clean := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, hasValue := strings.Cut(entry, "=")
		if hasValue {
			if _, isGitLocation := blocked[name]; isGitLocation {
				continue
			}
		}
		clean = append(clean, entry)
	}
	return clean
}

// CleanGitEnv is the shared boundary for direct Git/gh integrations outside
// this package. It removes repository-location variables but preserves normal
// authentication and locale environment for the caller.
func CleanGitEnv(environment []string) []string {
	return cleanGitEnv(environment)
}
