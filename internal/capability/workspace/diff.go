package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"time"
	"unicode/utf8"

	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
	capabilityruntime "github.com/jacu-dev/jacu-harness/internal/runtime"
	"github.com/jacu-dev/jacu-harness/internal/scope"
)

const (
	maxInlineDiffBytes           = 16 * 1024
	maxDiffOutputBytes     int64 = 32 * 1024
	diffTruncatedMarker          = "\n... diff truncated ...\n"
	diffOutputLimitWarning       = "diff truncated to fit 32KB encoded output limit"
	diffTraceIDBudget            = "tr_0000000000000000"
)

var errDiffMetadataExceedsOutputLimit = errors.New("workspace diff metadata exceeds 32KB output limit")

type DiffInput struct {
	RunID string `json:"run_id"`
}

type DiffData struct {
	Digest     string   `json:"digest"`
	Files      []string `json:"files"`
	Added      int      `json:"added"`
	Deleted    int      `json:"deleted"`
	InScope    []string `json:"in_scope"`
	OutOfScope []string `json:"out_of_scope"`
	Diff       string   `json:"diff"`
}

type DiffResult struct {
	Status   string
	Summary  string
	Data     DiffData
	Warnings []string
}

func WorkspaceDiff(ctx context.Context, root string, in DiffInput) (DiffResult, error) {
	var result DiffResult
	err := runstate.WithLock(root, func() error {
		var err error
		result, err = workspaceDiffUnlocked(ctx, root, in)
		return err
	})
	return result, err
}

func workspaceDiffUnlocked(ctx context.Context, root string, in DiffInput) (DiffResult, error) {
	run, err := runstate.Load(root, in.RunID)
	if err != nil {
		return DiffResult{}, err
	}
	if identityErr := validateRunIdentity(root, run); identityErr != nil {
		emitWorkspaceGate(root, "block", "jacu_diff", run)
		return DiffResult{
			Status:   "blocked",
			Summary:  "run identity check failed: " + identityErr.Error(),
			Warnings: []string{},
		}, nil
	}
	git, err := gitx.New()
	if err != nil {
		return DiffResult{}, err
	}
	snapshot, err := git.DiffSnapshot(ctx, run.Worktree, run.BaseSHA)
	if err != nil {
		return DiffResult{}, err
	}
	fullDiff := snapshot.Patch
	digest := diffDigest(fullDiff)
	data := DiffData{
		Digest:     digest,
		Files:      snapshot.Files,
		Added:      snapshot.Added,
		Deleted:    snapshot.Deleted,
		InScope:    []string{},
		OutOfScope: []string{},
		Diff:       fullDiff,
	}
	warnings := []string{}
	for _, path := range snapshot.Files {
		if scope.MatchesAny(path, run.Mission.AllowedPaths) {
			data.InScope = append(data.InScope, path)
		} else {
			data.OutOfScope = append(data.OutOfScope, path)
			warnings = append(warnings, "out-of-scope change: "+path)
		}
		if scope.MatchesAny(path, run.Mission.ForbiddenPaths) {
			warnings = append(warnings, "FORBIDDEN path modified: "+path)
		}
	}
	if fullDiff == "" {
		warnings = append(warnings, "no changes yet")
	}
	gateVerdict := "pass"
	if len(data.OutOfScope) > 0 {
		gateVerdict = "warn"
	}
	emitWorkspaceGate(root, gateVerdict, "jacu_diff", run)
	preview := fullDiff
	if len(fullDiff) > maxInlineDiffBytes {
		preview = utf8Prefix(fullDiff, maxInlineDiffBytes)
		data.Diff = preview + diffTruncatedMarker
		warnings = append(warnings, "diff exceeds 16KB; inline output truncated")
	}
	result := DiffResult{Status: "ok", Summary: "Workspace diff reviewed.", Data: data, Warnings: warnings}
	if err := fitDiffResultOutput(&result, preview, maxDiffOutputBytes); err != nil {
		return DiffResult{}, err
	}
	if err := run.Transition(runstate.StatusReviewed); err != nil {
		return DiffResult{}, err
	}
	run.ReviewedDigest = digest
	run.ReviewedAt = time.Now().UTC()
	if err := runstate.SaveLocked(root, run); err != nil {
		return DiffResult{}, err
	}
	return result, nil
}

func fitDiffResultOutput(result *DiffResult, preview string, maxBytes int64) error {
	encodedBytes, err := encodedDiffResultBytes(*result)
	if err != nil {
		return err
	}
	if encodedBytes <= maxBytes {
		return nil
	}

	result.Warnings = append(result.Warnings, diffOutputLimitWarning)
	result.Data.Diff = diffTruncatedMarker
	encodedBytes, err = encodedDiffResultBytes(*result)
	if err != nil {
		return err
	}
	if encodedBytes > maxBytes {
		return errDiffMetadataExceedsOutputLimit
	}

	previewRunes := []rune(preview)
	best := result.Data.Diff
	for low, high := 0, len(previewRunes); low <= high; {
		middle := low + (high-low)/2
		candidate := string(previewRunes[:middle]) + diffTruncatedMarker
		result.Data.Diff = candidate
		encodedBytes, err = encodedDiffResultBytes(*result)
		if err != nil {
			return err
		}
		if encodedBytes <= maxBytes {
			best = candidate
			low = middle + 1
		} else {
			high = middle - 1
		}
	}
	result.Data.Diff = best
	return nil
}

func encodedDiffResultBytes(result DiffResult) (int64, error) {
	runtimeResult := workspaceDiffRuntimeResult(result)
	runtimeResult.TraceID = diffTraceIDBudget
	encoded, err := json.Marshal(runtimeResult)
	return int64(len(encoded)), err
}

func workspaceDiffRuntimeResult(result DiffResult) capabilityruntime.Result {
	return capabilityruntime.Result{
		Status: result.Status, Summary: result.Summary, Data: result.Data,
		Artifacts: []string{}, Warnings: nonNilStrings(result.Warnings), NextActions: []string{},
	}
}

func utf8Prefix(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}
