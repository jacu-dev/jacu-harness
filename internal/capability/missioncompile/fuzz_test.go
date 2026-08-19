package missioncompile

import (
	"encoding/json"
	"strings"
	"testing"
)

func FuzzMissionInput(f *testing.F) {
	largeObjective, err := json.Marshal(Input{Objective: strings.Repeat("x", 1<<20)})
	if err != nil {
		f.Fatal(err)
	}
	seeds := [][]byte{
		[]byte(`{}`),
		largeObjective,
		[]byte(`{"objective":"修正 🔥 \u0000 teste"}`),
		[]byte(`{"objective":"Fix the parser output now","verification_commands":[]}`),
		[]byte(`{"objective":"Fix the parser output now","verification_commands":null}`),
		[]byte(`{"verification_commands":[["sh","-c","rm -rf /"]]}`),
		[]byte(`{"risk_hint":"banana"}`),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	root := f.TempDir()
	f.Fuzz(func(t *testing.T, raw []byte) {
		var input Input
		if err := json.Unmarshal(raw, &input); err != nil {
			return
		}

		mission, status, nextActions := Compile(root, input)
		if status != "ok" && status != "blocked" {
			t.Fatalf("status = %q; want ok or blocked", status)
		}
		if status == "blocked" && (mission.MissionID != "" || mission.Ceremony != "") {
			t.Fatalf("blocked result contains mission identity: %#v", mission)
		}

		hasBlock := false
		for _, item := range mission.Lint {
			if item.Level == "BLOCK" {
				hasBlock = true
			}
		}
		if hasBlock != (status == "blocked") {
			t.Fatalf("status = %q lint = %#v; BLOCK and blocked must match", status, mission.Lint)
		}
		if input.RiskHint != "" && input.RiskHint != "safe" && input.RiskHint != "write" && input.RiskHint != "destructive" {
			assertMissionLintLevel(t, mission, "invalid_risk_hint", "WARN")
		}
		for _, command := range input.VerificationCommands {
			if isShellInterpreterCommand(command) {
				assertMissionLintRule(t, mission, "shell_interpreter_command")
			}
		}

		encoded, err := json.Marshal(envelope[Mission]{
			Status:      status,
			Summary:     "Mission compile fuzz result.",
			Data:        mission,
			Artifacts:   []string{},
			Warnings:    []string{},
			NextActions: nextActions,
			TraceID:     "tr_fuzz",
		})
		if err != nil {
			t.Fatalf("marshal envelope: %v", err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("decode envelope: %v", err)
		}
		if _, ok := decoded["data"].(map[string]any); !ok {
			t.Fatalf("data = %T; want object", decoded["data"])
		}
	})
}

// assertMissionLintLevel exige a regra no nível dado. Um hint inválido tem que
// aparecer como WARN: se voltar a ser BLOCK, o incidente de 2026-08-04 volta
// junto — hint malformado parando a missão.
func assertMissionLintLevel(t *testing.T, mission Mission, rule, level string) {
	t.Helper()
	for _, item := range mission.Lint {
		if item.Rule == rule {
			if item.Level != level {
				t.Fatalf("lint %s level = %q; want %q", rule, item.Level, level)
			}
			return
		}
	}
	t.Fatalf("lint = %#v; want rule %q", mission.Lint, rule)
}

func assertMissionLintRule(t *testing.T, mission Mission, rule string) {
	t.Helper()
	for _, item := range mission.Lint {
		if item.Rule == rule && item.Level == "BLOCK" {
			return
		}
	}
	t.Fatalf("lint = %#v; want BLOCK rule %q", mission.Lint, rule)
}
