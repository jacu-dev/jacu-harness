package ledger

import (
	"testing"

	ctxpack "github.com/jacu-dev/jacu-harness/internal/capability/context"
)

func TestDecideRefusesRequiredOverflowWithoutToolCall(t *testing.T) {
	pack := ctxpack.Pack{Items: []ctxpack.Item{
		{ID: "anchor:mission/objective", Path: "mission/objective", Bytes: 8, Required: true, SHA256: "aa"},
		{ID: "file:big", Path: "big.bin", Bytes: 1000, Required: true, SHA256: "bb"},
	}, Bytes: 1008, Anchors: []string{"anchor:mission/objective"}}
	called := 0
	decision := Decide(16, pack, func() { called++ })
	if decision.Verdict != VerdictRefuse || !decision.RequiredOverflow || decision.Reason != ReasonOverflow {
		t.Fatalf("decision = %#v", decision)
	}
	if decision.ToolCalls != 0 || called != 0 {
		t.Fatalf("tool calls = %d called=%d; refuse must not dispatch", decision.ToolCalls, called)
	}
}

func TestDecideDegradesWhenOnlyOptionalOverflows(t *testing.T) {
	pack := ctxpack.Pack{Items: []ctxpack.Item{
		{ID: "anchor:mission/objective", Path: "mission/objective", Bytes: 4, Required: true, SHA256: "aa"},
		{ID: "file:opt", Path: "opt.bin", Bytes: 1000, Required: false, SHA256: "bb"},
	}, Bytes: 1004, Anchors: []string{"anchor:mission/objective"}}
	called := 0
	decision := Decide(16, pack, func() { called++ })
	if decision.Verdict != VerdictDegrade || decision.DroppedOptional != 1 || called != 1 {
		t.Fatalf("decision = %#v called=%d", decision, called)
	}
}

func TestDecideAdmitsWhenEverythingFits(t *testing.T) {
	pack := ctxpack.Pack{Items: []ctxpack.Item{
		{ID: "anchor:mission/objective", Path: "mission/objective", Bytes: 4, Required: true, SHA256: "aa"},
	}, Bytes: 4, Anchors: []string{"anchor:mission/objective"}}
	decision := Decide(32, pack, func() {})
	if decision.Verdict != VerdictAdmit || decision.Reason != ReasonFit || decision.RequiredOverflow {
		t.Fatalf("decision = %#v", decision)
	}
}
