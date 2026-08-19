package missioncompile

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func TestCompileBlocksMissionWithPreflightGap(t *testing.T) {
	mission, status, _ := Compile(t.TempDir(), Input{
		Objective:            "run the governed check",
		AcceptanceCriteria:   []string{"the check is governed"},
		VerificationCommands: [][]string{{"not-allowlisted"}},
	})
	if status != "blocked" {
		t.Fatalf("compile status = %q, want blocked: %+v", status, mission)
	}
}

func TestCompileTreatsLegacyNetworkPseudoCommandAsUngoverned(t *testing.T) {
	mission, status, _ := Compile(t.TempDir(), Input{
		Objective:            "run the governed check",
		AcceptanceCriteria:   []string{"the check is governed"},
		VerificationCommands: [][]string{{"network:required"}},
	})
	if status != "blocked" {
		t.Fatalf("compile status = %q; want blocked: %+v", status, mission)
	}
}

func TestCompileHandlerEmitsPreflightInterruptionTelemetry(t *testing.T) {
	root := t.TempDir()
	t.Setenv("JACU_HOME", t.TempDir())
	raw, err := json.Marshal(Input{
		Objective:            "run the governed check",
		VerificationCommands: [][]string{{"not-allowlisted"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := compileHandler(root)(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q; want blocked", result.Status)
	}
	events, err := telemetry.NewStore().ReadSince(time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	var check, interruption bool
	for _, event := range events {
		if event.Event == telemetry.EventPreflightCheck && event.Result == "fail" {
			check = true
		}
		if event.Event == telemetry.EventMissionInterruption && event.FailureClass == "allowlist" {
			interruption = true
		}
	}
	if !check || !interruption {
		t.Fatalf("preflight telemetry = %+v; want failed check and interruption", events)
	}
}

func TestCompileHandlerEmitsNestedPreflightInterruptionTelemetry(t *testing.T) {
	root := t.TempDir()
	t.Setenv("JACU_HOME", t.TempDir())
	raw, err := json.Marshal(Input{
		Objective: "run the governed program",
		Program: &ProgramInput{
			OpenQuestions: []string{},
			Missions: []ProgramMissionInput{{
				Objective:            "run the nested check",
				VerificationCommands: [][]string{{"not-allowlisted"}},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := compileHandler(root)(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "blocked" {
		t.Fatalf("status = %q; want blocked", result.Status)
	}
	events, err := telemetry.NewStore().ReadSince(time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Event == telemetry.EventMissionInterruption && event.FailureClass == "allowlist" {
			return
		}
	}
	t.Fatalf("nested preflight telemetry = %+v; want allowlist interruption", events)
}

func compileWithDeclaredPaths(t *testing.T, input Input) (Mission, string, []string) {
	t.Helper()
	root := t.TempDir()
	for _, declared := range input.AllowedPaths {
		if declared == "." || declared == "" {
			continue
		}
		if err := os.MkdirAll(filepath.Join(root, declared), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if input.Program != nil {
		for _, nested := range input.Program.Missions {
			for _, declared := range nested.AllowedPaths {
				if declared == "." || declared == "" {
					continue
				}
				if err := os.MkdirAll(filepath.Join(root, declared), 0o700); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	return Compile(root, input)
}
