package missioncompile

import "testing"

func TestClassifyCeremonyDirect(t *testing.T) {
	tests := []struct {
		name     string
		riskHint string
	}{
		{name: "risk absent"},
		{name: "risk safe", riskHint: "safe"},
		// Um hint inválido é ignorado, e não pode empurrar a cerimônia para
		// cima: objetivo de leitura continua sendo resposta direta.
		{name: "risk invalid is ignored", riskHint: "banana"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyCeremony(Input{
				Objective: "Explain how this project works",
				RiskHint:  tt.riskHint,
			})
			if got != "direct" {
				t.Fatalf("classifyCeremony() = %q; want direct", got)
			}
		})
	}
}

func TestClassifyCeremonyFull(t *testing.T) {
	tests := []struct {
		name string
		in   Input
	}{
		{
			name: "destructive risk",
			in: Input{
				Objective: "Remove the obsolete project files",
				RiskHint:  "destructive",
			},
		},
		{
			name: "exactly three acceptance criteria",
			in: Input{
				Objective:          "Fix the parser output now",
				AcceptanceCriteria: []string{"One", "Two", "Three"},
				RiskHint:           "write",
			},
		},
		{
			name: "project root allowed",
			in: Input{
				Objective:    "Fix the parser output now",
				AllowedPaths: []string{"."},
				RiskHint:     "write",
			},
		},
		{
			name: "glob allowed",
			in: Input{
				Objective:    "Fix the parser output now",
				AllowedPaths: []string{"**"},
				RiskHint:     "write",
			},
		},
		{
			name: "three distinct directories",
			in: Input{
				Objective:    "Fix the parser output now",
				AllowedPaths: []string{"cmd", "internal", "pkg"},
				RiskHint:     "write",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyCeremony(tt.in); got != "full" {
				t.Fatalf("classifyCeremony() = %q; want full", got)
			}
		})
	}
}

func TestClassifyCeremonyLight(t *testing.T) {
	tests := []struct {
		name string
		in   Input
	}{
		{
			name: "mutation verb without paths",
			in: Input{
				Objective: "Fix the parser output now",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyCeremony(tt.in); got != "light" {
				t.Fatalf("classifyCeremony() = %q; want light", got)
			}
		})
	}
}
