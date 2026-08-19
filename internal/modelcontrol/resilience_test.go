package modelcontrol

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDemotionUsesStrictTenSamplesAndFortyPercent(t *testing.T) {
	bad := EvaluateDemotion(ModelMetric{ProfileID: "bad", TaskKind: "code", TotalRuns: 12, AcceptedRuns: 5})
	if !bad.Demoted || bad.FailureRate <= .40 {
		t.Fatalf("bad demotion = %#v", bad)
	}
	if EvaluateDemotion(ModelMetric{ProfileID: "noisy", TaskKind: "code", TotalRuns: 9, AcceptedRuns: 0}).Demoted {
		t.Fatal("nine samples must not demote")
	}
	if EvaluateDemotion(ModelMetric{ProfileID: "edge", TaskKind: "code", TotalRuns: 10, AcceptedRuns: 6}).Demoted {
		t.Fatal("exactly forty percent must not demote")
	}
}

func TestDemotionNeverStrandsLastCandidate(t *testing.T) {
	candidates := []HostProfile{{ID: "only"}, {ID: "good"}}
	kept, events := ApplyDemotionBias(candidates, []ModelMetric{{ProfileID: "only", TaskKind: "code", TotalRuns: 12, AcceptedRuns: 0}, {ProfileID: "good", TaskKind: "code", TotalRuns: 12, AcceptedRuns: 0}}, "code")
	if !reflect.DeepEqual(kept, candidates) || len(events) != 2 || strings.Contains(events[0], "secret") {
		t.Fatalf("kept=%#v events=%#v; want original candidates and sanitized events", kept, events)
	}
}

func TestEscalationIsBoundedAndBudgetIsTerminal(t *testing.T) {
	policy := EscalationPolicy{MaxAttempts: 3, Tiers: []TierTarget{{Lane: LaneCheap, ProfileID: "cheap"}, {Lane: LaneMedium, ProfileID: "mid"}, {Lane: LanePlanner, ProfileID: "planner"}}, OnExhaust: RequireHuman}
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	if next := policy.Next(1, FailureVerification); next.Kind != Retry || next.Next.ProfileID != "mid" {
		t.Fatalf("retry = %#v; want mid", next)
	}
	if next := policy.Next(1, FailureBudget); next.Kind != Exhausted || next.Flag != RequireHuman {
		t.Fatalf("budget = %#v; want human exhaustion", next)
	}
}

func TestCircuitBreakerOpensAllowsOneProbeAndClosesOnSuccess(t *testing.T) {
	now := time.Unix(100, 0)
	breaker := NewCircuitBreaker(3, time.Second)
	for i := 0; i < 3; i++ {
		if !breaker.Allow(now) {
			t.Fatal("closed breaker denied call")
		}
		breaker.Record(now, false)
	}
	if breaker.Allow(now) {
		t.Fatal("open breaker allowed call before cooldown")
	}
	if !breaker.Allow(now.Add(time.Second)) || breaker.Allow(now.Add(time.Second)) {
		t.Fatal("half-open breaker did not allow exactly one probe")
	}
	breaker.Record(now.Add(time.Second), true)
	if !breaker.Allow(now.Add(time.Second)) {
		t.Fatal("successful probe did not close breaker")
	}
}

func TestGuardedInvokeBlocksBeforeCallbackAndRecordsFailure(t *testing.T) {
	breaker := NewCircuitBreaker(1, time.Minute)
	profile := HostProfile{ID: "p", CLI: SignedCLI{Path: "/bin/p", SHA256: testDigest, Signer: "host", Signature: "sig"}}
	called := false
	_, err := GuardedInvoke(context.Background(), profile, "sha256:bad", func(SignedCLI) bool { return true }, breaker, time.Unix(1, 0), func(context.Context, HostProfile) (InvocationResult, error) {
		called = true
		return InvocationResult{}, nil
	})
	if err == nil || called || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("err=%v called=%v; want digest block before callback", err, called)
	}
}
