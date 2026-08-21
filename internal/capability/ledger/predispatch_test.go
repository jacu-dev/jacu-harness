package ledger

import (
	"testing"

	ctxpack "github.com/jacu-dev/jacu-harness/internal/capability/context"
)

func TestRefuseHappensBeforeDispatch(t *testing.T) {
	pack := PackOverflow(t)
	dispatched := false
	decision := Decide(1, pack, func() { dispatched = true })
	if decision.Verdict != VerdictRefuse {
		t.Fatalf("verdict = %s", decision.Verdict)
	}
	if dispatched || decision.ToolCalls != 0 {
		t.Fatal("dispatch ran on refuse")
	}
}

func PackOverflow(t *testing.T) ctxpack.Pack {
	t.Helper()
	return ctxpack.Pack{Items: []ctxpack.Item{
		{ID: "anchor:mission/objective", Path: "mission/objective", Bytes: 8, Required: true},
		{ID: "file:huge", Path: "huge.bin", Bytes: 4096, Required: true},
	}, Bytes: 4104, Anchors: []string{"anchor:mission/objective"}}
}
