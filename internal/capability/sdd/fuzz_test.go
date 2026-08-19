package sdd

import "testing"

func FuzzArbitraryMarkdownNeverPanics(f *testing.F) {
	f.Add([]byte("# Example\n## Requirements\n### Requirement: Safe\n"))
	f.Add([]byte("---\nsdd: broken\n---\n```\n### not a requirement\n"))
	f.Add([]byte{0, 1, 2, 3, '\n'})
	f.Fuzz(func(t *testing.T, input []byte) {
		document, findings := lintBytes(input)
		if document.Raw == nil && findings == nil {
			t.Fatal("fuzz processing returned neither a document nor findings")
		}
	})
}
