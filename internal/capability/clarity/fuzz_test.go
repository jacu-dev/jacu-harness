package clarity

import "testing"

func FuzzIngestNeverPanicsOrStoresPayload(f *testing.F) {
	f.Add([]byte(`{"write_scope":["a"],"forbidden_paths":[],"requirements":[],"out_of_scope":[],"tasks":[]}`))
	f.Add([]byte("narrative prose from a model"))
	f.Add([]byte(`{"unexpected":true}`))
	f.Add([]byte{0, 1, 2, '{', '}'})
	f.Fuzz(func(t *testing.T, raw []byte) {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("Ingest panicked: %v", recovered)
			}
		}()
		readback, err := Ingest(raw)
		if err == nil {
			_ = Normalize(readback)
			return
		}
		typed, ok := err.(Error)
		if !ok {
			t.Fatalf("untyped error %T %v", err, err)
		}
		if typed.Code == "" {
			t.Fatal("typed finding has empty code")
		}
		if string(raw) != "" && typed.Error() == string(raw) {
			t.Fatal("payload was stored as the finding")
		}
	})
}
