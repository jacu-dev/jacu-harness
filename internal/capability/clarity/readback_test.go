package clarity

import "testing"

func TestIngestRejectsProseAndUnknownFields(t *testing.T) {
	if _, err := Ingest([]byte("the spec requires a workspace apply")); err == nil {
		t.Fatal("prose readback was accepted")
	} else if typed, ok := err.(Error); !ok || typed.Code != CodeProse {
		t.Fatalf("prose error = %#v; want %s", err, CodeProse)
	}

	if _, err := Ingest([]byte(`{"write_scope":[],"forbidden_paths":[],"requirements":[],"out_of_scope":[],"tasks":[],"essay":"nope"}`)); err == nil {
		t.Fatal("unknown field was accepted")
	} else if typed, ok := err.(Error); !ok || typed.Code != CodeUnknownField || typed.Field != "essay" {
		t.Fatalf("unknown-field error = %#v", err)
	}
}

func TestIngestAcceptsClosedReadback(t *testing.T) {
	readback, err := Ingest([]byte(`{"write_scope":[" cmd/jacu/clarity.go ","cmd/jacu/clarity.go"],"forbidden_paths":[],"requirements":["The readback is a closed structure"],"out_of_scope":[],"tasks":["T1"]}`))
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if len(readback.WriteScope) != 1 || readback.WriteScope[0] != "cmd/jacu/clarity.go" {
		t.Fatalf("write_scope = %#v", readback.WriteScope)
	}
}
