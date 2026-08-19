package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReviewReceiptSignsAndConsumesOnce(t *testing.T) {
	root := t.TempDir()
	key := []byte("local-test-key")
	receipt := ReviewReceipt{
		RunID:      "run_0123456789abcdef",
		DiffDigest: "sha256:tree",
		Verdict:    "approve",
		Reasons:    []string{"batch review"},
		CreatedAt:  time.Now().UTC(),
	}
	signed, err := SignReviewReceipt(receipt, key)
	if err != nil {
		t.Fatal(err)
	}
	path, err := WriteReviewReceipt(root, signed)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != receipt.RunID+".json" {
		t.Fatalf("receipt path = %q", path)
	}
	consumed, err := ConsumeReviewReceipt(root, receipt.RunID, receipt.DiffDigest, key, time.Now().UTC())
	if err != nil || consumed.Verdict != "approve" {
		t.Fatalf("consume = %#v err %v", consumed, err)
	}
	if _, err := ConsumeReviewReceipt(root, receipt.RunID, receipt.DiffDigest, key, time.Now().UTC()); err == nil {
		t.Fatal("receipt was consumed twice")
	}
}

func TestReviewReceiptRejectsTamperDigestVerdictAndTimestamp(t *testing.T) {
	key := []byte("local-test-key")
	base := ReviewReceipt{RunID: "run_0123456789abcdef", DiffDigest: "sha256:tree", Verdict: "approve", CreatedAt: time.Now().UTC()}
	signed, err := SignReviewReceipt(base, key)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutated := range map[string]ReviewReceipt{
		"tampered reason":  func() ReviewReceipt { r := signed; r.Reasons = []string{"forged"}; return r }(),
		"digest mismatch":  func() ReviewReceipt { r := signed; r.DiffDigest = "sha256:other"; return r }(),
		"invalid verdict":  func() ReviewReceipt { r := signed; r.Verdict = "pass"; return r }(),
		"future timestamp": func() ReviewReceipt { r := signed; r.CreatedAt = time.Now().Add(10 * time.Minute).UTC(); return r }(),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateReviewReceipt(mutated, key, base.RunID, base.DiffDigest, time.Now().UTC()); err == nil {
				t.Fatal("mutated receipt unexpectedly validated")
			}
		})
	}
}

func TestReviewReceiptDoesNotClaimSessionSeparation(t *testing.T) {
	if _, ok := any(ReviewReceipt{}).(interface{ ReviewerSessionID() string }); ok {
		t.Fatal("receipt exposes a session-separation claim")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git", "jacu"), 0o700); err != nil {
		t.Fatal(err)
	}
}
