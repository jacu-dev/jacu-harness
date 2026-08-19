package workspace

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
)

func FuzzAutonomyPolicyBytes(f *testing.F) {
	f.Add([]byte(`{"policy":{"auto_apply":{"require":["verify_pass","cross_review"],"risk_max":"write","max_iterations":3,"on_violation":"escalate"}}}`))
	f.Fuzz(func(t *testing.T, content []byte) {
		policy, err := decodeAutonomyPolicyJSON(content)
		if err != nil {
			return
		}
		decision := EvaluateAutoApplyPolicy(policy, "pass", policy.RiskMax, true, 1)
		if !decision.Allowed {
			t.Fatalf("validated policy denied its own safe pass: %#v", decision)
		}
	})
}

func FuzzReviewReceiptBytes(f *testing.F) {
	f.Add([]byte(`{"run_id":"run_0123456789abcdef","diff_digest":"sha256:tree","verdict":"approve","reasons":[],"created_at":"2026-08-11T17:00:00Z","signature":"00"}`))
	f.Fuzz(func(t *testing.T, content []byte) {
		receipt, err := decodeReviewReceipt(content)
		if err != nil {
			return
		}
		// A decoded receipt is never permission by itself; only a valid HMAC can
		// pass this check, and fuzz input does not control the local key.
		if err := ValidateReviewReceipt(receipt, []byte("local-key"), receipt.RunID, receipt.DiffDigest, time.Now().UTC()); err == nil && receipt.Signature == "" {
			t.Fatal("empty signature validated")
		}
	})
}

func FuzzMissionProgramInput(f *testing.F) {
	f.Add([]byte(`{"objective":"run","allowed_paths":["a.txt"],"program":{"open_questions":[],"missions":[{"objective":"fix","allowed_paths":["a.txt"]}]}}`))
	f.Fuzz(func(t *testing.T, content []byte) {
		var input missioncompile.Input
		if json.Unmarshal(content, &input) != nil {
			return
		}
		_, status, _ := missioncompile.Compile(t.TempDir(), input)
		if status != "ok" && status != "blocked" {
			t.Fatalf("invalid compile status %q", status)
		}
	})
}
