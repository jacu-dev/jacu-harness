package orchestration

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestExecuteRoutesOnlyMatchingVerdictAndKeepsTraceDeterministic(t *testing.T) {
	flow := Flow{
		Nodes: []Node{
			{ID: "verify", Uses: UseVerify, AllowedPaths: []string{"src"}},
			{ID: "apply", Uses: UseApply, AllowedPaths: []string{"src"}},
			{ID: "stop", Uses: UseReview, AllowedPaths: []string{"docs"}},
		},
		Edges: []Edge{
			{From: "verify", To: "apply", When: "verdict == pass"},
			{From: "verify", To: "stop", When: "verdict == fail"},
			{From: "verify", To: "stop", When: "verdict == timeout"},
			{From: "verify", To: "stop", When: "verdict == blocked"},
			{From: "verify", To: "stop", When: "verdict == not_run"},
		},
	}
	called := make([]string, 0)
	result, err := Execute(context.Background(), flow, nil, func(_ context.Context, node Node, _ map[string]NodeResult) (NodeResult, error) {
		called = append(called, node.ID)
		if node.ID == "verify" {
			return NodeResult{Status: "ok", Verdict: "pass", Output: map[string]any{"run_id": "run_1111111111111111"}}, nil
		}
		return NodeResult{Status: "ok", Policy: "pass"}, nil
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != "ok" || !reflect.DeepEqual(called, []string{"verify", "apply"}) {
		t.Fatalf("result = %#v, called = %v", result, called)
	}
	if len(result.Trace) != 3 || result.Trace[1].NodeID != "apply" || result.Trace[2].NodeID != "stop" || result.Trace[2].Decision != "skipped" {
		t.Fatalf("trace = %#v; want skipped stop and executed apply", result.Trace)
	}
}

func TestExecuteRunsIndependentWaveConcurrentlyButEmitsSortedTrace(t *testing.T) {
	flow := Flow{Nodes: []Node{
		{ID: "b", Uses: UseReview, AllowedPaths: []string{"b"}},
		{ID: "a", Uses: UseReview, AllowedPaths: []string{"a"}},
	}}
	result, err := Execute(context.Background(), flow, nil, func(_ context.Context, node Node, _ map[string]NodeResult) (NodeResult, error) {
		if node.ID == "a" {
			time.Sleep(15 * time.Millisecond)
		}
		return NodeResult{Status: "ok", Policy: "pass"}, nil
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !reflect.DeepEqual(result.Waves, [][]string{{"a", "b"}}) {
		t.Fatalf("waves = %#v", result.Waves)
	}
	if len(result.Trace) != 2 || result.Trace[0].NodeID != "a" || result.Trace[1].NodeID != "b" {
		t.Fatalf("trace order = %#v", result.Trace)
	}
}

func TestExecuteStopsOnPolicyEscalation(t *testing.T) {
	flow := Flow{Nodes: []Node{
		{ID: "review", Uses: UseReview, AllowedPaths: []string{"review"}},
		{ID: "apply", Uses: UseApply, AllowedPaths: []string{"apply"}},
	}, Edges: []Edge{{From: "review", To: "apply", When: "policy == pass"}}}
	called := 0
	result, err := Execute(context.Background(), flow, nil, func(_ context.Context, node Node, _ map[string]NodeResult) (NodeResult, error) {
		called++
		return NodeResult{Status: "blocked", Policy: "require_approval"}, nil
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != "escalated" || called != 1 {
		t.Fatalf("result = %#v, called = %d", result, called)
	}
}

func TestExecuteRunsBoundedClosedCycleFromDeterministicSeed(t *testing.T) {
	flow := Flow{
		Nodes: []Node{
			{ID: "a", Uses: UseReview, MaxVisits: 2},
			{ID: "b", Uses: UseReview},
		},
		Edges: []Edge{
			{From: "a", To: "b", When: "default"},
			{From: "b", To: "a", When: "default"},
		},
	}
	called := make([]string, 0, 3)
	result, err := Execute(context.Background(), flow, map[string]any{"seed": "ok"}, func(_ context.Context, node Node, _ map[string]NodeResult) (NodeResult, error) {
		called = append(called, node.ID)
		return NodeResult{Status: "ok", Policy: "pass"}, nil
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != "ok" || !reflect.DeepEqual(called, []string{"a", "b", "a"}) {
		t.Fatalf("result = %#v, called = %v", result, called)
	}
	if len(result.Trace) != 3 || result.Trace[0].Visit != 1 || result.Trace[2].Visit != 2 {
		t.Fatalf("trace = %#v; want bounded visits", result.Trace)
	}
}

func TestExecuteBlocksWaveWiderThanFanOutCap(t *testing.T) {
	nodes := make([]Node, MaxWaveWidth+1)
	for i := range nodes {
		id := string(rune('a' + i))
		nodes[i] = Node{ID: id, Uses: UseReview, AllowedPaths: []string{id}}
	}
	called := 0
	result, err := Execute(context.Background(), Flow{Nodes: nodes}, nil, func(context.Context, Node, map[string]NodeResult) (NodeResult, error) {
		called++
		return NodeResult{Status: "ok"}, nil
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Status != "blocked" || called != 0 || len(result.Findings) == 0 || result.Findings[0].Rule != "fan_out" {
		t.Fatalf("result = %#v; called = %d; want blocked fan_out and no execution", result, called)
	}
}
