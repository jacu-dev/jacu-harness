package clarity

import "testing"

func TestRewriteRoundLargerThanPreviousIsRefused(t *testing.T) {
	delta, err := SpecBytesDelta(100, 140)
	if err == nil || delta != 40 {
		t.Fatalf("delta=%d err=%v; want 40 and refusal", delta, err)
	}
	if typed, ok := err.(Error); !ok || typed.Code != CodeSpecGrew {
		t.Fatalf("error = %#v; want %s", err, CodeSpecGrew)
	}
	if _, err := SpecBytesDelta(140, 140); err != nil {
		t.Fatalf("equal spec bytes refused: %v", err)
	}
	if _, err := SpecBytesDelta(140, 120); err != nil {
		t.Fatalf("smaller rewrite refused: %v", err)
	}
}
