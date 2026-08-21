package context

import "testing"

func TestDigestChangesWhenIncludedContentChangesOneByte(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{"note.txt": "hello"})
	spec := Spec{Objective: "digest", AllowedPaths: []string{"note.txt"}}
	before, err := PackRoot(root, spec)
	if err != nil {
		t.Fatal(err)
	}
	writeTree(t, root, map[string]string{"note.txt": "hellp"})
	after, err := PackRoot(root, spec)
	if err != nil {
		t.Fatal(err)
	}
	if before.Digest == after.Digest {
		t.Fatal("content change did not change digest")
	}
	if before.Items[len(before.Items)-1].Path == after.Items[len(after.Items)-1].Path && before.Digest == after.Digest {
		t.Fatal("digest ignored content")
	}
}
