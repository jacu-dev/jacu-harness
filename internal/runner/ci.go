package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	CheckStatusPending = "pending"
	CheckStatusPassed  = "passed"
	CheckStatusFailed  = "failed"
	CheckStatusTimeout = "timed_out"
	maxCITimeout       = 5 * time.Minute
	defaultCITimeout   = 30 * time.Second
)

// CheckEvidenceRequest identifies one already-created PR. PullRequest accepts
// a numeric PR number or a validated branch/ref understood by gh.
type CheckEvidenceRequest struct {
	PullRequest  string
	Directory    string
	Timeout      time.Duration
	TailBytes    int
	PollInterval time.Duration
}

type CheckRun struct {
	Bucket   string `json:"bucket"`
	Link     string `json:"link"`
	Name     string `json:"name"`
	State    string `json:"state"`
	Workflow string `json:"workflow"`
}

type CheckAnnotation struct {
	Path            string `json:"path"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	AnnotationLevel string `json:"annotation_level"`
	Title           string `json:"title"`
	Message         string `json:"message"`
}

type CheckFailureEvidence struct {
	Check          CheckRun
	Repository     string
	RunID          string
	JobID          string
	LogTail        string
	LogTruncated   bool
	LogDigest      string
	Annotations    []CheckAnnotation
	EvidenceDigest string
}

type CheckEvidence struct {
	PullRequest string
	Status      string
	Checks      []CheckRun
	Failures    []CheckFailureEvidence
	Warnings    []string
	Digest      string
}

type checkCollector func(context.Context, CheckEvidenceRequest) (CheckEvidence, error)

// collectionTimeoutError identifies an individual gh collection that consumed
// its command budget. It is retryable only inside an active watch budget.
type collectionTimeoutError struct{}

func (collectionTimeoutError) Error() string { return "gh check collection timed out" }

// CollectCheckEvidence is the trusted host-side GitHub adapter. It invokes
// only gh with fixed subcommands; providers never receive these credentials or
// raw GitHub output.
func CollectCheckEvidence(ctx context.Context, request CheckEvidenceRequest) (CheckEvidence, error) {
	result := CheckEvidence{
		PullRequest: request.PullRequest,
		Checks:      []CheckRun{},
		Failures:    []CheckFailureEvidence{},
		Warnings:    []string{},
	}
	if err := validateCheckEvidenceRequest(request); err != nil {
		return result, err
	}
	checksCommand, err := runGH(ctx, withCITail(request, maximumTailBytes), "pr", "checks", request.PullRequest,
		"--json", "bucket,link,name,state,workflow")
	if err != nil {
		return result, err
	}
	if checksCommand.Status != StatusCompleted {
		if checksCommand.Status == StatusTimedOut {
			result.Status = CheckStatusTimeout
			return result, collectionTimeoutError{}
		}
		return result, errors.New("gh check collection failed")
	}
	if checksCommand.Truncated {
		return result, errors.New("gh check collection exceeded its output limit")
	}
	if err := decodeJSON(checksCommand.StdoutTail, &result.Checks); err != nil {
		return result, errors.New("gh check collection returned invalid JSON")
	}

	pending := false
	for _, check := range result.Checks {
		if isFailedCheck(check) {
			failure := CheckFailureEvidence{Check: check, Annotations: []CheckAnnotation{}}
			if warning := enrichFailureEvidence(ctx, request, &failure); warning != "" {
				result.Warnings = append(result.Warnings, warning)
			}
			result.Failures = append(result.Failures, failure)
			continue
		}
		if isPendingCheck(check) {
			pending = true
		}
	}
	if len(result.Checks) == 0 {
		result.Status = CheckStatusPending
	} else if len(result.Failures) > 0 {
		result.Status = CheckStatusFailed
	} else if pending {
		result.Status = CheckStatusPending
	} else {
		result.Status = CheckStatusPassed
	}
	result.Digest = digestJSON(struct {
		Checks   []CheckRun
		Failures []CheckFailureEvidence
	}{Checks: result.Checks, Failures: result.Failures}, checksCommand.Digest)
	return result, nil
}

// WatchCheckEvidence polls a PR until all checks are terminal or the bounded
// watch context expires. A pending check is never classified as a failure.
func WatchCheckEvidence(ctx context.Context, request CheckEvidenceRequest) (CheckEvidence, error) {
	return watchCheckEvidence(ctx, request, CollectCheckEvidence)
}

// watchCheckEvidence contains polling policy separately from the gh adapter so
// timeout behaviour can be tested without processes or wall-clock sleeps.
func watchCheckEvidence(ctx context.Context, request CheckEvidenceRequest, collect checkCollector) (CheckEvidence, error) {
	total := request.Timeout
	if total <= 0 {
		total = maxCITimeout
	}
	watchCtx, cancel := context.WithTimeout(ctx, total)
	defer cancel()
	var last CheckEvidence
	for {
		remaining := time.Until(deadlineFromContext(watchCtx))
		if remaining <= 0 {
			last.Status = CheckStatusTimeout
			return last, nil
		}
		commandRequest := request
		commandRequest.Timeout = remaining
		if commandRequest.Timeout > 10*time.Second {
			commandRequest.Timeout = 10 * time.Second
		}
		evidence, err := collect(watchCtx, commandRequest)
		if err != nil {
			var timeoutErr collectionTimeoutError
			if errors.As(err, &timeoutErr) && watchCtx.Err() == nil {
				// An individual command timeout is a normal retryable observation.
				// Keep the latest partial evidence and proceed to the polling wait.
				last = evidence
			} else if watchCtx.Err() != nil {
				if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return last, ctx.Err()
				}
				last.Status = CheckStatusTimeout
				return last, nil
			} else {
				return evidence, err
			}
		} else {
			last = evidence
		}
		if last.Status != CheckStatusPending && last.Status != CheckStatusTimeout {
			return last, nil
		}
		interval := request.PollInterval
		if interval <= 0 {
			interval = 10 * time.Second
		}
		if interval > remaining {
			interval = remaining
		}
		timer := time.NewTimer(interval)
		select {
		case <-watchCtx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			if ctx.Err() != nil && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return last, ctx.Err()
			}
			if last.Status == CheckStatusPending {
				last.Status = CheckStatusTimeout
			}
			return last, nil
		case <-timer.C:
		}
	}
}

func deadlineFromContext(ctx context.Context) time.Time {
	deadline, ok := ctx.Deadline()
	if !ok {
		return time.Now().Add(maxCITimeout)
	}
	return deadline
}

func validateCheckEvidenceRequest(request CheckEvidenceRequest) error {
	if !validPullRequestRef(request.PullRequest) {
		return errors.New("pull request ref is invalid")
	}
	if request.Directory != "" {
		if !filepath.IsAbs(request.Directory) {
			return errors.New("GitHub command directory must be absolute")
		}
		info, err := os.Stat(request.Directory)
		if err != nil || !info.IsDir() {
			return errors.New("GitHub command directory is unavailable")
		}
	}
	if request.Timeout < 0 || request.Timeout > maxCITimeout {
		return errors.New("GitHub check timeout exceeds the runner limit")
	}
	return nil
}

func enrichFailureEvidence(ctx context.Context, request CheckEvidenceRequest, failure *CheckFailureEvidence) string {
	repository, runID, jobID, err := parseGitHubJobLink(failure.Check.Link)
	if err != nil {
		return "check " + failure.Check.Name + ": evidence link is invalid"
	}
	failure.Repository, failure.RunID, failure.JobID = repository, runID, jobID
	logResult, logErr := runGH(ctx, request, "run", runID, "--log-failed")
	if logErr != nil || logResult.Status != StatusCompleted {
		return "check " + failure.Check.Name + ": failed log could not be collected"
	}
	failure.LogTail = logResult.StdoutTail
	failure.LogTruncated = logResult.Truncated
	failure.LogDigest = logResult.Digest
	annotationResult, annotationErr := runGH(ctx, withCITail(request, maximumTailBytes), "api",
		"repos/"+repository+"/check-runs/"+jobID+"/annotations", "--paginate", "--slurp")
	if annotationErr != nil || annotationResult.Status != StatusCompleted {
		failure.EvidenceDigest = digestJSON(failure, logResult.Digest)
		return "check " + failure.Check.Name + ": annotations could not be collected"
	}
	if err := decodeAnnotations(annotationResult.StdoutTail, &failure.Annotations); err != nil {
		failure.EvidenceDigest = digestJSON(failure, logResult.Digest, annotationResult.Digest)
		return "check " + failure.Check.Name + ": annotations were invalid"
	}
	failure.EvidenceDigest = digestJSON(failure, logResult.Digest, annotationResult.Digest)
	return ""
}

func withCITail(request CheckEvidenceRequest, tailBytes int) CheckEvidenceRequest {
	request.TailBytes = tailBytes
	return request
}

func isFailedCheck(check CheckRun) bool {
	bucket := strings.ToLower(check.Bucket)
	state := strings.ToLower(check.State)
	return bucket == "fail" || bucket == "failure" || state == "failure" || state == "error" || state == "cancelled"
}

func isPendingCheck(check CheckRun) bool {
	bucket := strings.ToLower(check.Bucket)
	state := strings.ToLower(check.State)
	return bucket == "pending" || bucket == "queued" || state == "pending" || state == "in_progress" || state == "queued"
}

func validPullRequestRef(value string) bool {
	if value == "" || len(value) > 128 || strings.HasPrefix(value, "-") || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	if _, err := strconv.ParseUint(value, 10, 32); err == nil {
		return true
	}
	if !strings.HasPrefix(value, "jacu/run-") {
		return false
	}
	for _, char := range strings.TrimPrefix(value, "jacu/run-") {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func parseGitHubJobLink(raw string) (repository, runID, jobID string, err error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" {
		return "", "", "", errors.New("not a GitHub Actions link")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 7 || parts[2] != "actions" || parts[3] != "runs" || parts[5] != "job" {
		return "", "", "", errors.New("invalid GitHub Actions link shape")
	}
	if !validRepoPart(parts[0]) || !validRepoPart(parts[1]) || !decimalID(parts[4]) || !decimalID(parts[6]) {
		return "", "", "", errors.New("invalid GitHub Actions link identifiers")
	}
	return parts[0] + "/" + parts[1], parts[4], parts[6], nil
}

func validRepoPart(value string) bool {
	if value == "" || len(value) > 100 || value == "." || value == ".." {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func decimalID(value string) bool {
	if value == "" || len(value) > 20 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

type ghResult struct {
	Status     string
	StdoutTail string
	StderrTail string
	Truncated  bool
	Digest     string
	DurationMs int64
}

func runGH(ctx context.Context, request CheckEvidenceRequest, args ...string) (ghResult, error) {
	result := ghResult{Status: StatusBlocked}
	binary, err := exec.LookPath("gh")
	if err != nil {
		return result, errors.New("gh is not installed")
	}
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = defaultCITimeout
	}
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	// #nosec G204 -- gh is fixed and all subcommands are built by this file.
	command := exec.CommandContext(commandCtx, binary, args...)
	command.Dir = request.Directory
	command.Env = ghEnvironment()
	configureProcessGroup(command)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return result, errors.New("gh stdout setup failed")
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return result, errors.New("gh stderr setup failed")
	}
	if err := command.Start(); err != nil {
		return result, errors.New("gh failed to start")
	}
	started := time.Now()
	out := newCapture(request.TailBytes)
	errOut := newCapture(request.TailBytes)
	var readers sync.WaitGroup
	readers.Add(2)
	go drain(&readers, stdout, out)
	go drain(&readers, stderr, errOut)
	readers.Wait()
	waitErr := command.Wait()
	stopProcessGroup(command)
	result.DurationMs = time.Since(started).Milliseconds()
	result.StdoutTail = out.tail.String()
	result.StderrTail = errOut.tail.String()
	result.Truncated = out.tail.truncated || errOut.tail.truncated
	result.Digest = digestCaptures(out, errOut)
	switch {
	case errors.Is(commandCtx.Err(), context.DeadlineExceeded):
		result.Status = StatusTimedOut
	case ctx.Err() != nil:
		result.Status = StatusCancelled
	case waitErr == nil:
		result.Status = StatusCompleted
	default:
		result.Status = StatusFailed
	}
	return result, nil
}

func ghEnvironment() []string {
	allowed := map[string]struct{}{
		"PATH": {}, "HOME": {}, "GH_TOKEN": {}, "GITHUB_TOKEN": {}, "GH_HOST": {}, "XDG_CONFIG_HOME": {},
	}
	environment := make([]string, 0, len(allowed)+3)
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, keep := allowed[name]; keep {
				environment = append(environment, entry)
			}
		}
	}
	return append(environment, "GH_PAGER=cat", "PAGER=cat", "NO_COLOR=1")
}

func decodeJSON(input string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON")
		}
		return err
	}
	return nil
}

func decodeAnnotations(input string, annotations *[]CheckAnnotation) error {
	var direct []CheckAnnotation
	if err := json.Unmarshal([]byte(input), &direct); err == nil {
		*annotations = direct
		return nil
	}
	var pages [][]CheckAnnotation
	if err := json.Unmarshal([]byte(input), &pages); err != nil {
		return err
	}
	flattened := make([]CheckAnnotation, 0)
	for _, page := range pages {
		flattened = append(flattened, page...)
	}
	*annotations = flattened
	return nil
}

func digestJSON(value any, streamDigests ...string) string {
	hasher := sha256.New()
	encoded, _ := json.Marshal(value)
	_, _ = hasher.Write(encoded)
	for _, digest := range streamDigests {
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(digest))
	}
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil))
}
