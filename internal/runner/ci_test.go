//go:build unix

package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"testing/synctest"
)

func TestCollectCheckEvidenceCapturesFailedJobLogAndAnnotations(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gh-calls.log")
	writeExecutable(t, filepath.Join(binDir, "gh"), `#!/bin/sh
printf '%s\n' "$*" >> `+logPath+`
if [ "$1" = "pr" ]; then
  printf 'PR_REF=%s\n' "$3" >> `+logPath+`
  printf '%s' '[{"bucket":"fail","link":"https://github.com/acme/widget/actions/runs/123/job/456","name":"lint","state":"FAILURE","workflow":"CI"}]'
  exit 0
fi
if [ "$1" = "run" ]; then
  printf 'RUN_ID=%s RUN_FLAG=%s\n' "$2" "$3" >> `+logPath+`
  awk 'BEGIN { for (i = 0; i < 1000; i++) print "lint failure tail" }'
  exit 0
fi
if [ "$1" = "api" ]; then
  printf 'API_ENDPOINT=%s\n' "$2" >> `+logPath+`
	  printf '%s' '[{"path":"internal/runner/ci.go","start_line":10,"end_line":12,"annotation_level":"failure","message":"fix lint","blob_href":"https://github.com/acme/widget/blob/main/internal/runner/ci.go"}]'
  exit 0
fi
exit 9
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+"/usr/bin:/bin")

	evidence, err := CollectCheckEvidence(context.Background(), CheckEvidenceRequest{PullRequest: "jacu/run-0123456789abcdef", TailBytes: 128})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Status != CheckStatusFailed || len(evidence.Failures) != 1 {
		t.Fatalf("evidence = %#v; want one failed check", evidence)
	}
	failure := evidence.Failures[0]
	if failure.RunID != "123" || failure.JobID != "456" || failure.Repository != "acme/widget" {
		t.Fatalf("failure identity = %#v", failure)
	}
	if !failure.LogTruncated || len(failure.LogTail) > 128 || !strings.Contains(failure.LogTail, "lint failure tail") {
		t.Fatalf("bounded log = %#v", failure)
	}
	if len(failure.Annotations) != 1 || failure.Annotations[0].Path != "internal/runner/ci.go" {
		t.Fatalf("annotations = %#v", failure.Annotations)
	}
	if !strings.HasPrefix(evidence.Digest, "sha256:") {
		t.Fatalf("digest = %q", evidence.Digest)
	}
	calls, err := os.ReadFile(logPath) // #nosec G304 -- test-owned temporary call log.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(calls), "PR_REF=jacu/run-0123456789abcdef") ||
		!strings.Contains(string(calls), "RUN_ID=123 RUN_FLAG=--log-failed") {
		t.Fatalf("unexpected gh argv: %q", string(calls))
	}
}

func TestCollectCheckEvidenceDoesNotTurnPendingIntoFailure(t *testing.T) {
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "gh"), `#!/bin/sh
printf '%s' '[{"bucket":"pending","link":"https://github.com/acme/widget/actions/runs/123/job/456","name":"verify","state":"IN_PROGRESS","workflow":"CI"}]'
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+"/usr/bin:/bin")
	evidence, err := CollectCheckEvidence(context.Background(), CheckEvidenceRequest{PullRequest: "31"})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Status != CheckStatusPending || len(evidence.Failures) != 0 {
		t.Fatalf("evidence = %#v; pending must not fail", evidence)
	}
}

func TestCollectCheckEvidenceRejectsMalformedActionLinkWithoutAnnotationCall(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "gh-calls.log")
	writeExecutable(t, filepath.Join(binDir, "gh"), `#!/bin/sh
printf '%s\n' "$*" >> `+logPath+`
printf '%s' '[{"bucket":"fail","link":"https://evil.example/actions/runs/not-a-number/job/456","name":"lint","state":"FAILURE","workflow":"CI"}]'
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+"/usr/bin:/bin")
	evidence, err := CollectCheckEvidence(context.Background(), CheckEvidenceRequest{PullRequest: "31"})
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Status != CheckStatusFailed || len(evidence.Failures) != 1 || len(evidence.Warnings) == 0 {
		t.Fatalf("evidence = %#v; want bounded collection warning", evidence)
	}
	calls, err := os.ReadFile(logPath) // #nosec G304 -- test-owned temporary call log.
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(calls), "api ") || strings.Contains(string(calls), "run view") {
		t.Fatalf("malformed link triggered follow-up gh call: %s", calls)
	}
}

func TestWatchCheckEvidencePendingBecomesPassedWithFakeTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		evidence, err := watchCheckEvidence(t.Context(), CheckEvidenceRequest{
			PullRequest:  "31",
			Timeout:      3 * time.Second,
			PollInterval: time.Second,
		}, func(_ context.Context, request CheckEvidenceRequest) (CheckEvidence, error) {
			calls++
			if calls == 1 {
				return CheckEvidence{PullRequest: request.PullRequest, Status: CheckStatusPending, Digest: "pending"}, nil
			}
			return CheckEvidence{PullRequest: request.PullRequest, Status: CheckStatusPassed, Digest: "passed"}, nil
		})
		if err != nil || evidence.Status != CheckStatusPassed || evidence.Digest != "passed" || calls != 2 {
			t.Fatalf("evidence=%#v err=%v calls=%d; want deterministic pending-to-passed", evidence, err, calls)
		}
	})
}

func TestWatchCheckEvidenceRetriesCollectionTimeoutWithFakeTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		evidence, err := watchCheckEvidence(t.Context(), CheckEvidenceRequest{
			PullRequest:  "31",
			Timeout:      3 * time.Second,
			PollInterval: time.Second,
		}, func(_ context.Context, request CheckEvidenceRequest) (CheckEvidence, error) {
			calls++
			if request.Timeout <= 0 || request.Timeout > 10*time.Second {
				t.Fatalf("command timeout = %s", request.Timeout)
			}
			if calls == 1 {
				return CheckEvidence{PullRequest: request.PullRequest, Status: CheckStatusTimeout}, collectionTimeoutError{}
			}
			return CheckEvidence{PullRequest: request.PullRequest, Status: CheckStatusPassed}, nil
		})
		if err != nil || evidence.Status != CheckStatusPassed || calls != 2 {
			t.Fatalf("evidence=%#v err=%v calls=%d; want passed after retry", evidence, err, calls)
		}
	})
}

func TestWatchCheckEvidenceDeadlineReturnsLatestEvidenceWithoutError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		evidence, err := watchCheckEvidence(t.Context(), CheckEvidenceRequest{
			PullRequest:  "31",
			Timeout:      2 * time.Second,
			PollInterval: time.Second,
		}, func(_ context.Context, request CheckEvidenceRequest) (CheckEvidence, error) {
			calls++
			return CheckEvidence{PullRequest: request.PullRequest, Status: CheckStatusPending, Digest: "latest"}, nil
		})
		if err != nil || evidence.Status != CheckStatusTimeout || evidence.Digest != "latest" || calls < 1 {
			t.Fatalf("evidence=%#v err=%v calls=%d; want latest timed_out without error", evidence, err, calls)
		}
	})
}

func TestWatchCheckEvidenceReturnsNonTimeoutErrorImmediately(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		want := errors.New("authentication failed")
		_, err := watchCheckEvidence(t.Context(), CheckEvidenceRequest{PullRequest: "31", Timeout: time.Second}, func(context.Context, CheckEvidenceRequest) (CheckEvidence, error) {
			return CheckEvidence{}, want
		})
		if !errors.Is(err, want) {
			t.Fatalf("error = %v; want %v", err, want)
		}
	})
}
