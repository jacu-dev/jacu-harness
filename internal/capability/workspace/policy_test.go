package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvaluateAutoApplyPolicyVerdictMatrix(t *testing.T) {
	policy := AutonomyPolicy{
		Require:       []string{"verify_pass", "cross_review"},
		RiskMax:       "write",
		MaxIterations: 3,
		OnViolation:   "escalate",
	}
	tests := []struct {
		name      string
		verdict   string
		risk      string
		receipt   bool
		iteration int
		allowed   bool
	}{
		{name: "pass", verdict: "pass", risk: "safe", receipt: true, iteration: 1, allowed: true},
		{name: "fail", verdict: "fail", risk: "safe", receipt: true, iteration: 1},
		{name: "timeout", verdict: "timeout", risk: "safe", receipt: true, iteration: 1},
		{name: "blocked", verdict: "blocked", risk: "safe", receipt: true, iteration: 1},
		{name: "not_run", verdict: "not_run", risk: "safe", receipt: true, iteration: 1},
		{name: "missing receipt", verdict: "pass", risk: "safe", receipt: false, iteration: 1},
		{name: "risk ceiling", verdict: "pass", risk: "destructive", receipt: true, iteration: 1},
		{name: "iteration budget", verdict: "pass", risk: "safe", receipt: true, iteration: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := EvaluateAutoApplyPolicy(policy, tt.verdict, tt.risk, tt.receipt, tt.iteration)
			if decision.Allowed != tt.allowed {
				t.Fatalf("decision = %#v; allowed = %v", decision, tt.allowed)
			}
		})
	}
}

func TestLoadAutonomyPolicyIsStrictAndRootScoped(t *testing.T) {
	root := t.TempDir()
	if _, ok, err := LoadAutonomyPolicy(root); err != nil || ok {
		t.Fatalf("absent policy = ok %v err %v; want absent", ok, err)
	}
	policyPath := filepath.Join(root, ".jacu", "autonomy-policy.json")
	if err := os.MkdirAll(filepath.Dir(policyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	valid := []byte(`{"policy":{"auto_apply":{"require":["verify_pass","cross_review"],"risk_max":"write","max_iterations":3,"on_violation":"escalate"}}}`)
	if err := os.WriteFile(policyPath, valid, 0o600); err != nil {
		t.Fatal(err)
	}
	if policy, ok, err := LoadAutonomyPolicy(root); err != nil || !ok || policy.RiskMax != "write" {
		t.Fatalf("valid policy = %#v ok %v err %v", policy, ok, err)
	}
	if err := os.WriteFile(policyPath, []byte(`{"policy":{"auto_apply":{"require":["verify_pass"],"risk_max":"write","unknown":true}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadAutonomyPolicy(root); err == nil {
		t.Fatal("unknown or incomplete policy unexpectedly accepted")
	}
}

func TestEvaluateAutoApplyPolicyOnViolationModes(t *testing.T) {
	for _, mode := range []string{"block", "escalate"} {
		policy := AutonomyPolicy{Require: []string{"verify_pass", "cross_review"}, RiskMax: "write", MaxIterations: 1, OnViolation: mode}
		decision := EvaluateAutoApplyPolicy(policy, "fail", "safe", true, 1)
		if decision.Allowed || decision.Escalate != (mode == "escalate") {
			t.Fatalf("mode %q decision = %#v", mode, decision)
		}
	}
}
