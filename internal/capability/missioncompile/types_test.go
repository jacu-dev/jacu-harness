package missioncompile

import (
	"reflect"
	"testing"
)

func TestMissionID(t *testing.T) {
	tests := []struct {
		name     string
		left     Input
		right    Input
		wantSame bool
	}{
		{
			name: "same content with different order and spaces",
			left: Input{
				Objective:          "  Fix the API bug  ",
				AcceptanceCriteria: []string{"  tests pass", "bug fixed  "},
				AllowedPaths:       []string{"internal/api", "cmd"},
				ForbiddenPaths:     []string{"secrets", ".env"},
			},
			right: Input{
				Objective:          "Fix the API bug",
				AcceptanceCriteria: []string{"bug fixed", "tests pass"},
				AllowedPaths:       []string{"cmd", "internal/api"},
				ForbiddenPaths:     []string{".env", "secrets"},
			},
			wantSame: true,
		},
		{
			name:     "different content",
			left:     Input{Objective: "Fix the API bug"},
			right:    Input{Objective: "Add the API feature"},
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			leftID := missionID(tt.left)
			rightID := missionID(tt.right)
			if gotSame := leftID == rightID; gotSame != tt.wantSame {
				t.Fatalf("missionID equality = %v; want %v: left=%q right=%q", gotSame, tt.wantSame, leftID, rightID)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want Input
	}{
		{
			name: "trims objective and deduplicates sorted lists",
			in: Input{
				Objective:          "  Fix the API bug  ",
				AcceptanceCriteria: []string{" tests pass ", "bug fixed", "tests pass"},
				AllowedPaths:       []string{" internal/api ", "cmd", "cmd"},
				ForbiddenPaths:     []string{"secrets", " .env ", "secrets"},
			},
			want: Input{
				Objective:          "Fix the API bug",
				AcceptanceCriteria: []string{"bug fixed", "tests pass"},
				AllowedPaths:       []string{"cmd", "internal/api"},
				ForbiddenPaths:     []string{".env", "secrets"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalize(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("normalize() = %#v; want %#v", got, tt.want)
			}
		})
	}
}
