package context

import "testing"

func TestCheckAnchorsFailsWhenMissionAnchorMissingFromPack(t *testing.T) {
	spec := Spec{Objective: "keep the contract", Acceptance: []string{"done"}}
	pack, err := PackRoot(t.TempDir(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if lost := CheckAnchors(pack); lost != 0 {
		t.Fatalf("complete pack lost %d anchors", lost)
	}
	pack.Items = pack.Items[1:]
	if lost := CheckAnchors(pack); lost == 0 {
		t.Fatal("dropped objective anchor was not detected")
	}
}
