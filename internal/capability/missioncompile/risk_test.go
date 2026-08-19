package missioncompile

import "testing"

// The mission risk used to be whatever the caller declared: `risk = RiskHint`
// or "write". That let the governed object grade its own homework, and it made
// a malformed hint load-bearing enough to stop the mission — the 2026-08-04
// incident, where a host sent risk_hint "high", the lint answered BLOCK, and the
// model's recovery plan was to resend with "destructive". Two defects: the
// invalid value should never have reached the lint, and a hint must never be
// able to block.
//
// The rule these tests pin: effective risk is max(derived, hint). The hint may
// only raise the risk, never lower it.

func TestDerivedRiskClassifiesByMissionContent(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want string
	}{
		{
			name: "read only question derives safe",
			in:   Input{Objective: "Explain how the parser works"},
			want: "safe",
		},
		{
			name: "mutation verb derives write",
			in:   Input{Objective: "Fix the parser output", AllowedPaths: []string{"parser.go"}},
			want: "write",
		},
		{
			name: "declared paths alone derive write",
			in:   Input{Objective: "Update the readme", AllowedPaths: []string{"README.md"}},
			want: "write",
		},
		{
			name: "removal verb derives destructive",
			in:   Input{Objective: "Remove the legacy parser package", AllowedPaths: []string{"parser"}},
			want: "destructive",
		},
		{
			name: "portuguese removal verb derives destructive",
			in:   Input{Objective: "Apagar os arquivos temporarios do build", AllowedPaths: []string{"build"}},
			want: "destructive",
		},
		{
			name: "cleanup objective derives destructive",
			in:   Input{Objective: "Limpar o diretorio de cache antigo", AllowedPaths: []string{"cache"}},
			want: "destructive",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := derivedRisk(normalize(tt.in)); got != tt.want {
				t.Fatalf("derivedRisk() = %q; want %q", got, tt.want)
			}
		})
	}
}

// One case per line of the composition rule.
func TestEffectiveRiskComposesDerivedAndHint(t *testing.T) {
	tests := []struct {
		name     string
		in       Input
		wantRisk string
		wantWarn string
	}{
		{
			name:     "invalid hint is ignored and warned, derived wins",
			in:       Input{Objective: "Fix the parser output", AllowedPaths: []string{"parser.go"}, RiskHint: "high"},
			wantRisk: "write",
			wantWarn: "invalid_risk_hint",
		},
		{
			name:     "hint weaker than derived is ignored and warned",
			in:       Input{Objective: "Remove the legacy parser package", AllowedPaths: []string{"parser"}, RiskHint: "safe"},
			wantRisk: "destructive",
			wantWarn: "risk_hint_below_derived",
		},
		{
			name:     "hint stronger than derived is accepted without warning",
			in:       Input{Objective: "Update the readme", AllowedPaths: []string{"README.md"}, RiskHint: "destructive"},
			wantRisk: "destructive",
			wantWarn: "",
		},
		{
			name:     "hint equal to derived is silent",
			in:       Input{Objective: "Fix the parser output", AllowedPaths: []string{"parser.go"}, RiskHint: "write"},
			wantRisk: "write",
			wantWarn: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mission, status, _ := compileWithDeclaredPaths(t, tt.in)
			if status != "ok" {
				t.Fatalf("status = %q; a hint must never block. lint = %#v", status, mission.Lint)
			}
			if mission.Risk != tt.wantRisk {
				t.Fatalf("risk = %q; want %q", mission.Risk, tt.wantRisk)
			}
			warned := ""
			for _, item := range mission.Lint {
				if item.Rule == "invalid_risk_hint" || item.Rule == "risk_hint_below_derived" {
					if item.Level != "WARN" {
						t.Fatalf("lint %s level = %q; want WARN — a hint must never block", item.Rule, item.Level)
					}
					warned = item.Rule
				}
			}
			if warned != tt.wantWarn {
				t.Fatalf("risk hint lint = %q; want %q", warned, tt.wantWarn)
			}
		})
	}
}

// TestCompileSurvivesTheRiskHintIncident is the regression of the field report:
// the host sent an invalid hint on a destructive cleanup, the lint blocked, and
// the model's recovery was to escalate the value it sent. The mission must
// compile, warn about the hint, and derive destructive on its own.
func TestCompileSurvivesTheRiskHintIncident(t *testing.T) {
	mission, status, _ := compileWithDeclaredPaths(t, Input{
		Objective:    "Remove the stale build artifacts from the workspace",
		AllowedPaths: []string{"build"},
		RiskHint:     "high",
	})
	if status != "ok" {
		t.Fatalf("status = %q; the incident is exactly this blocking. lint = %#v", status, mission.Lint)
	}
	if mission.Risk != "destructive" {
		t.Fatalf("risk = %q; want destructive derived from the objective, not from the hint", mission.Risk)
	}
	if mission.MissionID == "" {
		t.Fatal("mission_id empty; the mission has to remain compilable without the host resending")
	}
	found := false
	for _, item := range mission.Lint {
		if item.Rule == "invalid_risk_hint" {
			found = true
			if item.Level != "WARN" || item.Message != "unknown risk_hint, ignored" {
				t.Fatalf("lint = %#v; want WARN with the ignored-hint message", item)
			}
		}
	}
	if !found {
		t.Fatalf("no invalid_risk_hint lint; the host has to learn the value was dropped. lint = %#v", mission.Lint)
	}
}
