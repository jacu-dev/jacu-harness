package orchestration

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/scope"
)

func TestScheduleWavesInheritedContract(t *testing.T) {
	tests := []struct {
		name  string
		nodes []Node
		edges []Edge
		want  [][]string
		err   string
	}{
		{
			name:  "two tasks disjoint share one wave",
			nodes: nodesWithPaths(map[string][]string{"a": {"src/a"}, "b": {"src/b"}}),
			want:  [][]string{{"a", "b"}},
		},
		{
			name:  "overlapping prefixes are sequential",
			nodes: nodesWithPaths(map[string][]string{"a": {"src"}, "b": {"src/inner"}}),
			want:  [][]string{{"a"}, {"b"}},
		},
		{
			name:  "edge places successor later",
			nodes: nodesWithPaths(map[string][]string{"a": {"x"}, "b": {"y"}}),
			edges: []Edge{{From: "a", To: "b"}},
			want:  [][]string{{"a"}, {"b"}},
		},
		{
			name:  "diamond collapses middle",
			nodes: nodesWithPaths(map[string][]string{"a": {"a"}, "b": {"b"}, "c": {"c"}, "d": {"d"}}),
			edges: []Edge{{From: "a", To: "b"}, {From: "a", To: "c"}, {From: "b", To: "d"}, {From: "c", To: "d"}},
			want:  [][]string{{"a"}, {"b", "c"}, {"d"}},
		},
		{
			name:  "successor waits for latest predecessor",
			nodes: nodesWithPaths(map[string][]string{"a": {"src"}, "b": {"src/inner"}, "c": {"docs"}}),
			edges: []Edge{{From: "a", To: "c"}, {From: "b", To: "c"}},
			want:  [][]string{{"a"}, {"b"}, {"c"}},
		},
		{
			name:  "duplicate node id first occurrence wins",
			nodes: []Node{{ID: "a", Uses: "mission", AllowedPaths: []string{"first"}}, {ID: "a", Uses: "mission", AllowedPaths: []string{"second"}}},
			want:  [][]string{{"a"}},
		},
		{
			name:  "empty scope takes own wave",
			nodes: nodesWithPaths(map[string][]string{"a": {"x"}, "b": nil, "c": {"y"}}),
			want:  [][]string{{"a", "c"}, {"b"}},
		},
		{
			name:  "bare glob takes own wave",
			nodes: nodesWithPaths(map[string][]string{"a": {"src/a"}, "b": {"*"}, "c": {"src/c"}}),
			want:  [][]string{{"a", "c"}, {"b"}},
		},
		{
			name:  "sibling prefixes share wave",
			nodes: nodesWithPaths(map[string][]string{"a": {"crates/auth"}, "b": {"crates/authz"}}),
			want:  [][]string{{"a", "b"}},
		},
		{
			name:  "cycle is rejected",
			nodes: nodesWithPaths(map[string][]string{"a": {"a"}, "b": {"b"}}),
			edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "a"}},
			err:   "cycle",
		},
		{
			name:  "dangling edge is rejected",
			nodes: nodesWithPaths(map[string][]string{"a": {"a"}}),
			edges: []Edge{{From: "a", To: "ghost"}},
			err:   "dangling",
		},
		{
			name: "empty graph is okay",
			want: [][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ScheduleWaves(Flow{Nodes: tt.nodes, Edges: tt.edges})
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("error = %v; want %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("schedule: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("waves = %#v; want %#v", got, tt.want)
			}
		})
	}
}

func TestScheduleWavesIsDeterministicAcrossInputPermutations(t *testing.T) {
	first := Flow{
		Nodes: nodesWithPaths(map[string][]string{"d": {"d"}, "c": {"c"}, "b": {"b"}, "a": {"a"}}),
		Edges: []Edge{{From: "c", To: "d"}, {From: "a", To: "c"}, {From: "b", To: "d"}, {From: "a", To: "b"}},
	}
	second := Flow{
		Nodes: nodesWithPaths(map[string][]string{"a": {"a"}, "b": {"b"}, "c": {"c"}, "d": {"d"}}),
		Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "d"}, {From: "a", To: "c"}, {From: "c", To: "d"}},
	}
	want, err := ScheduleWaves(first)
	if err != nil {
		t.Fatalf("first schedule: %v", err)
	}
	for i := 0; i < 100; i++ {
		got, repeatErr := ScheduleWaves(second)
		if repeatErr != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("repeat %d = %#v, err=%v; want %#v", i, got, repeatErr, want)
		}
	}
}

func TestScopeConflictNormalizesGlobsAndDirectoryBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		left, right []string
		want        bool
	}{
		{name: "star glob prefix", left: []string{"src/*"}, right: []string{"src/token.go"}, want: true},
		{name: "double star prefix", left: []string{"a/**/z"}, right: []string{"a/b"}, want: true},
		{name: "sibling prefix", left: []string{"crates/auth"}, right: []string{"crates/authz"}, want: false},
		{name: "directory child", left: []string{"crates/auth"}, right: []string{"crates/auth/src/token.go"}, want: true},
		{name: "unknown empty", left: nil, right: []string{"src/a"}, want: true},
		{name: "unknown bare glob", left: []string{"*"}, right: []string{"src/a"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scope.ListsConflict(tt.left, tt.right); got != tt.want {
				t.Fatalf("ListsConflict(%v,%v) = %v; want %v", tt.left, tt.right, got, tt.want)
			}
		})
	}
}

func TestFlowValidationBlocksUnsafeOrIncompleteGraphs(t *testing.T) {
	validVerdicts := []Edge{
		{From: "verify", To: "stop", When: "verdict == pass"},
		{From: "verify", To: "stop", When: "verdict == fail"},
		{From: "verify", To: "stop", When: "verdict == timeout"},
		{From: "verify", To: "stop", When: "verdict == blocked"},
		{From: "verify", To: "stop", When: "verdict == not_run"},
	}
	tests := []struct {
		name string
		flow Flow
		want string
	}{
		{
			name: "unknown capability",
			flow: Flow{Nodes: []Node{{ID: "x", Uses: "unknown"}}},
			want: "unknown_capability",
		},
		{
			name: "dangling edge",
			flow: Flow{Nodes: []Node{{ID: "x", Uses: "mission"}}, Edges: []Edge{{From: "x", To: "ghost"}}},
			want: "dangling_edge",
		},
		{
			name: "invalid condition",
			flow: Flow{Nodes: []Node{{ID: "x", Uses: "verify"}, {ID: "y", Uses: "review"}}, Edges: []Edge{{From: "x", To: "y", When: "verdict != pass"}}},
			want: "invalid_condition",
		},
		{
			name: "missing verdict coverage",
			flow: Flow{Nodes: []Node{{ID: "verify", Uses: "verify"}, {ID: "stop", Uses: "review"}}, Edges: []Edge{{From: "verify", To: "stop", When: "verdict == pass"}}},
			want: "incomplete_verdict_edges",
		},
		{
			name: "not run cannot apply",
			flow: Flow{Nodes: []Node{{ID: "verify", Uses: "verify"}, {ID: "apply", Uses: "apply"}}, Edges: append(validVerdicts[:0:0], Edge{From: "verify", To: "apply", When: "verdict == not_run"})},
			want: "not_run_apply",
		},
		{
			name: "cycle requires bound",
			flow: Flow{Nodes: []Node{{ID: "a", Uses: "review"}, {ID: "b", Uses: "review"}}, Edges: []Edge{{From: "a", To: "b", When: "default"}, {From: "b", To: "a", When: "default"}}},
			want: "unbounded_cycle",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Validate(tt.flow)
			if !result.Blocked() {
				t.Fatalf("validation = %#v; want blocked", result)
			}
			if !hasFinding(result, tt.want) {
				t.Fatalf("findings = %#v; want rule %q", result.Findings, tt.want)
			}
		})
	}
}

func TestFlowValidationAllowsBoundedCycle(t *testing.T) {
	flow := Flow{
		Nodes: []Node{{ID: "a", Uses: "review", MaxVisits: 2}, {ID: "b", Uses: "review"}},
		Edges: []Edge{{From: "a", To: "b", When: "default"}, {From: "b", To: "a", When: "default"}},
	}
	if result := Validate(flow); result.Blocked() {
		t.Fatalf("bounded cycle blocked: %#v", result.Findings)
	}
}

func TestPanelMarkdownIsStableAndDoesNotCountDuplicateSessions(t *testing.T) {
	opinions := []Opinion{
		{SessionID: "s2", Model: "reviewer-b", Verdict: "reject", Reason: "scope"},
		{SessionID: "s1", Model: "reviewer-a", Verdict: "approve", Reason: "tests"},
		{SessionID: "s1", Model: "reviewer-a", Verdict: "approve", Reason: "duplicate"},
	}
	got := RenderPanelMarkdown(opinions)
	want := "| session | model | verdict | reason |\n|---|---|---|---|\n| s1 | reviewer-a | approve | tests |\n| s2 | reviewer-b | reject | scope |\n"
	if got != want {
		t.Fatalf("panel = %q; want %q", got, want)
	}
}

func nodesWithPaths(paths map[string][]string) []Node {
	ids := make([]string, 0, len(paths))
	for id := range paths {
		ids = append(ids, id)
	}
	// Intentionally leave insertion order unspecified: the scheduler owns the
	// deterministic ordering contract.
	result := make([]Node, 0, len(paths))
	for id, allowed := range paths {
		result = append(result, Node{ID: id, Uses: "mission", AllowedPaths: allowed})
	}
	_ = ids
	return result
}

func hasFinding(result Validation, rule string) bool {
	for _, finding := range result.Findings {
		if finding.Rule == rule {
			return true
		}
	}
	return false
}
