package memory

import (
	"strings"
	"testing"
	"time"
)

func TestPromoteDerivedToConventionRequiresEvalAndPreservesHistory(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	source := testRecord("mem_0000000000000091", testProjectID, "decision", "Derived rule", "Use bounded changes")
	source.Source = "derived"
	source.Evidence = []string{"eval/report.json"}
	if err := store.Save(source, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := PromoteDerivedToConvention(store, source.MemoryID, false, time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "eval") {
		t.Fatalf("failed eval promotion error = %v; want eval block", err)
	}
	unchanged, ok := store.Get(source.MemoryID)
	if !ok || unchanged.Status != "active" {
		t.Fatalf("failed eval changed source: %#v, %v", unchanged, ok)
	}

	now := time.Date(2026, 8, 11, 12, 34, 56, 0, time.UTC)
	successor, err := PromoteDerivedToConvention(store, source.MemoryID, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if successor.Kind != "convention" || successor.Source != "derived" || successor.Status != "active" || successor.UpdatedAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("successor = %#v; want active convention with explicit timestamp", successor)
	}
	if successor.MemoryID == source.MemoryID {
		t.Fatal("promotion reused the decision identity")
	}
	old, ok := store.Get(source.MemoryID)
	if !ok || old.Status != "superseded" || old.SupersededBy != successor.MemoryID {
		t.Fatalf("source after promotion = %#v, %v; want superseded by successor", old, ok)
	}
}

func TestPromoteDerivedToConventionRejectsHumanAndZeroClock(t *testing.T) {
	root := t.TempDir()
	store := NewFileStore(root)
	human := testRecord("mem_0000000000000092", testProjectID, "decision", "Human rule", "Do not auto-promote")
	if err := store.Save(human, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := PromoteDerivedToConvention(store, human.MemoryID, true, time.Now()); err == nil || !strings.Contains(err.Error(), "derived") {
		t.Fatalf("human promotion error = %v; want derived block", err)
	}
	derived := testRecord("mem_0000000000000093", testProjectID, "decision", "Derived zero clock", "Body")
	derived.Source = "derived"
	derived.Evidence = []string{"eval.json"}
	if err := store.Save(derived, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := PromoteDerivedToConvention(store, derived.MemoryID, true, time.Time{}); err == nil || !strings.Contains(err.Error(), "timestamp") {
		t.Fatalf("zero clock error = %v; want timestamp block", err)
	}
}
