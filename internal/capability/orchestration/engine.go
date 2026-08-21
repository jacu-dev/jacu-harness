package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Input struct {
	Flow    Flow           `json:"flow"`
	Context map[string]any `json:"context,omitempty"`
	RunID   string         `json:"run_id,omitempty"`
	Async   bool           `json:"async,omitempty"`
}

type NodeResult struct {
	Status   string         `json:"status"`
	Verdict  string         `json:"verdict,omitempty"`
	Policy   string         `json:"policy,omitempty"`
	Output   map[string]any `json:"output,omitempty"`
	Opinions []Opinion      `json:"opinions,omitempty"`
}

type TraceEvent struct {
	NodeID   string `json:"node_id"`
	Uses     string `json:"uses"`
	Wave     int    `json:"wave"`
	Visit    int    `json:"visit"`
	Status   string `json:"status"`
	Verdict  string `json:"verdict,omitempty"`
	Policy   string `json:"policy,omitempty"`
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

type FlowResult struct {
	Status        string       `json:"status"`
	Summary       string       `json:"summary"`
	Waves         [][]string   `json:"waves"`
	Trace         []TraceEvent `json:"trace"`
	PanelMarkdown string       `json:"panel_markdown,omitempty"`
	Findings      []Finding    `json:"findings,omitempty"`
}

// NodeExecutor is the only effectful seam of the graph engine. The MCP tool
// supplies the capability-backed implementation; tests can supply a fake and
// prove path selection without processes or filesystem state.
type NodeExecutor func(context.Context, Node, map[string]NodeResult) (NodeResult, error)

// MaxWaveWidth is the scheduler fan-out cap: a wave wider than this blocks
// before any of its nodes run.
const MaxWaveWidth = 4

func fanOutFinding(waves [][]string) *Finding {
	for _, wave := range waves {
		if len(wave) > MaxWaveWidth {
			msg := fmt.Sprintf("wave width %d exceeds cap %d", len(wave), MaxWaveWidth)
			return &Finding{Level: "BLOCK", Rule: "fan_out", Message: msg}
		}
	}
	return nil
}

func Execute(ctx context.Context, flow Flow, initial map[string]any, executor NodeExecutor) (FlowResult, error) {
	validation := Validate(flow)
	if validation.Blocked() {
		return FlowResult{Status: "blocked", Summary: "Flow validation blocked.", Waves: [][]string{}, Trace: []TraceEvent{}, Findings: validation.Findings}, nil
	}
	if executor == nil {
		return FlowResult{}, errors.New("flow node executor is nil")
	}
	waves, scheduleErr := ScheduleWaves(flow)
	if scheduleErr != nil {
		// A bounded cycle is valid by the flow contract, but cannot be represented
		// as a DAG wave. The bounded walker preserves deterministic semantics.
		if cycle, ok := scheduleErr.(*ScheduleError); !ok || cycle.Kind != "cycle" {
			return FlowResult{Status: "blocked", Summary: "Flow scheduling blocked.", Waves: [][]string{}, Trace: []TraceEvent{{Decision: "blocked", Reason: scheduleErr.Error()}}, Findings: []Finding{{Level: "BLOCK", Rule: "schedule", Message: scheduleErr.Error()}}}, nil
		}
		return executeBoundedCycle(ctx, flow, initial, executor)
	}
	if finding := fanOutFinding(waves); finding != nil {
		return FlowResult{Status: "blocked", Summary: "Flow fan-out exceeds cap.", Waves: waves, Trace: []TraceEvent{}, Findings: []Finding{*finding}}, nil
	}

	nodes := make(map[string]Node, len(flow.Nodes))
	for _, node := range flow.Nodes {
		if _, exists := nodes[node.ID]; !exists {
			nodes[node.ID] = node
		}
	}
	edgesByTarget := make(map[string][]Edge, len(nodes))
	for _, edge := range flow.Edges {
		edgesByTarget[edge.To] = append(edgesByTarget[edge.To], edge)
	}
	results := make(map[string]NodeResult, len(nodes))
	trace := make([]TraceEvent, 0, len(nodes))
	allOpinions := make([]Opinion, 0)
	for waveIndex, wave := range waves {
		if err := ctx.Err(); err != nil {
			return FlowResult{Status: "cancelled", Summary: "Flow cancelled.", Waves: waves, Trace: trace, PanelMarkdown: RenderPanelMarkdown(allOpinions)}, nil
		}
		active := make([]Node, 0, len(wave))
		waveTrace := make([]TraceEvent, 0, len(wave))
		for _, nodeID := range wave {
			node := nodes[nodeID]
			decision, matched := edgeDecision(edgesByTarget[nodeID], results)
			if len(edgesByTarget[nodeID]) > 0 && !matched {
				waveTrace = append(waveTrace, TraceEvent{NodeID: nodeID, Uses: node.Uses, Wave: waveIndex, Decision: "skipped", Reason: decision})
				continue
			}
			active = append(active, node)
		}

		type nodeOutcome struct {
			node   Node
			result NodeResult
			err    error
		}
		outcomes := make(chan nodeOutcome, len(active))
		var wait sync.WaitGroup
		for _, node := range active {
			node := node
			wait.Add(1)
			go func() {
				defer wait.Done()
				resolved, resolveErr := resolveNode(node, results, initial)
				if resolveErr != nil {
					outcomes <- nodeOutcome{node: node, err: resolveErr}
					return
				}
				if resolved.Lane != "" {
					if _, routeErr := RouteNode(ctx, activePanel, resolved); routeErr != nil {
						outcomes <- nodeOutcome{node: resolved, result: NodeResult{Status: "blocked"}}
						return
					}
				}
				value, execErr := executor(ctx, resolved, results)
				outcomes <- nodeOutcome{node: resolved, result: value, err: execErr}
			}()
		}
		wait.Wait()
		close(outcomes)
		collected := make([]nodeOutcome, 0, len(active))
		for outcome := range outcomes {
			collected = append(collected, outcome)
		}
		sort.Slice(collected, func(i, j int) bool { return collected[i].node.ID < collected[j].node.ID })
		for _, outcome := range collected {
			if outcome.err != nil {
				return FlowResult{Status: "failed", Summary: "Flow node failed.", Waves: waves, Trace: trace, PanelMarkdown: RenderPanelMarkdown(allOpinions)}, outcome.err
			}
			results[outcome.node.ID] = outcome.result
			allOpinions = append(allOpinions, outcome.result.Opinions...)
			waveTrace = append(waveTrace, TraceEvent{NodeID: outcome.node.ID, Uses: outcome.node.Uses, Wave: waveIndex, Visit: 1, Status: outcome.result.Status, Verdict: outcome.result.Verdict, Policy: outcome.result.Policy, Decision: "executed"})
			if policyStops(outcome.result) {
				sort.Slice(waveTrace, func(i, j int) bool { return waveTrace[i].NodeID < waveTrace[j].NodeID })
				trace = append(trace, waveTrace...)
				return FlowResult{Status: "escalated", Summary: "Flow escalated by capability policy.", Waves: waves, Trace: trace, PanelMarkdown: RenderPanelMarkdown(allOpinions)}, nil
			}
		}
		sort.Slice(waveTrace, func(i, j int) bool { return waveTrace[i].NodeID < waveTrace[j].NodeID })
		trace = append(trace, waveTrace...)
	}
	return FlowResult{Status: "ok", Summary: "Flow completed.", Waves: waves, Trace: trace, PanelMarkdown: RenderPanelMarkdown(allOpinions)}, nil
}

func edgeDecision(edges []Edge, results map[string]NodeResult) (string, bool) {
	if len(edges) == 0 {
		return "root", true
	}
	for _, edge := range edges {
		result, exists := results[edge.From]
		if !exists {
			continue
		}
		condition, err := parseCondition(edge.When)
		if err != nil {
			return err.Error(), false
		}
		if condition.Default || (condition.Field == "verdict" && result.Verdict == condition.Value) || (condition.Field == "policy" && result.Policy == condition.Value) {
			return edge.When, true
		}
	}
	return "no incoming edge condition matched", false
}

func policyStops(result NodeResult) bool {
	return result.Policy == "blocked" || result.Policy == "escalate" || result.Policy == "require_approval"
}

func resolveNode(node Node, results map[string]NodeResult, initial map[string]any) (Node, error) {
	value, err := resolveValue(node.With, results, initial)
	if err != nil {
		return Node{}, err
	}
	if value == nil {
		return node, nil
	}
	with, ok := value.(map[string]any)
	if !ok {
		return Node{}, fmt.Errorf("node %s with must be an object", node.ID)
	}
	node.With = with
	return node, nil
}

func resolveValue(value any, results map[string]NodeResult, initial map[string]any) (any, error) {
	switch typed := value.(type) {
	case string:
		if !strings.HasPrefix(typed, "$node.") && !strings.HasPrefix(typed, "$context.") {
			return typed, nil
		}
		parts := strings.Split(strings.TrimPrefix(strings.TrimPrefix(typed, "$node."), "$context."), ".")
		if strings.HasPrefix(typed, "$context.") {
			if len(parts) != 1 {
				return nil, fmt.Errorf("invalid context reference %q", typed)
			}
			contextValue, ok := initial[parts[0]]
			if !ok {
				return nil, fmt.Errorf("missing context reference %q", typed)
			}
			return contextValue, nil
		}
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid node reference %q", typed)
		}
		result, ok := results[parts[0]]
		if !ok {
			return nil, fmt.Errorf("node reference %q is not complete", typed)
		}
		if parts[1] == "verdict" {
			return result.Verdict, nil
		}
		if parts[1] == "policy" {
			return result.Policy, nil
		}
		if result.Output == nil {
			return nil, fmt.Errorf("node reference %q has no output", typed)
		}
		resolved, ok := result.Output[parts[1]]
		if !ok {
			return nil, fmt.Errorf("node reference %q has no field", typed)
		}
		return resolved, nil
	case map[string]any:
		copyMap := make(map[string]any, len(typed))
		for key, item := range typed {
			resolved, err := resolveValue(item, results, initial)
			if err != nil {
				return nil, err
			}
			copyMap[key] = resolved
		}
		return copyMap, nil
	case []any:
		items := make([]any, len(typed))
		for index, item := range typed {
			resolved, err := resolveValue(item, results, initial)
			if err != nil {
				return nil, err
			}
			items[index] = resolved
		}
		return items, nil
	default:
		return value, nil
	}
}

func executeBoundedCycle(ctx context.Context, flow Flow, initial map[string]any, executor NodeExecutor) (FlowResult, error) {
	nodes := make(map[string]Node, len(flow.Nodes))
	for _, node := range flow.Nodes {
		if _, exists := nodes[node.ID]; !exists {
			nodes[node.ID] = node
		}
	}
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	results := make(map[string]NodeResult)
	visits := make(map[string]int)
	trace := make([]TraceEvent, 0)
	allOpinions := make([]Opinion, 0)
	for step := 0; step < len(nodes)*4+4; step++ {
		if err := ctx.Err(); err != nil {
			return FlowResult{Status: "cancelled", Summary: "Flow cancelled.", Waves: [][]string{}, Trace: trace, PanelMarkdown: RenderPanelMarkdown(allOpinions)}, nil
		}
		selected := ""
		for _, id := range ids {
			incoming := make([]Edge, 0)
			for _, edge := range flow.Edges {
				if edge.To == id {
					incoming = append(incoming, edge)
				}
			}
			_, matched := edgeDecision(incoming, results)
			if len(incoming) == 0 || matched {
				if visits[id] == 0 || visits[id] < maxVisits(nodes[id]) {
					selected = id
					break
				}
			}
		}
		// A bounded graph can be a closed cycle with no root. Seed it with the
		// lexical first node so the first visit remains deterministic.
		if selected == "" && len(results) == 0 && len(ids) > 0 && maxVisits(nodes[ids[0]]) > 0 {
			selected = ids[0]
		}
		if selected == "" {
			return FlowResult{Status: "ok", Summary: "Flow completed.", Waves: [][]string{}, Trace: trace, PanelMarkdown: RenderPanelMarkdown(allOpinions)}, nil
		}
		resolved, err := resolveNode(nodes[selected], results, initial)
		if err != nil {
			return FlowResult{}, err
		}
		result, err := executor(ctx, resolved, results)
		if err != nil {
			return FlowResult{}, err
		}
		visits[selected]++
		results[selected] = result
		allOpinions = append(allOpinions, result.Opinions...)
		trace = append(trace, TraceEvent{NodeID: selected, Uses: resolved.Uses, Visit: visits[selected], Status: result.Status, Verdict: result.Verdict, Policy: result.Policy, Decision: "executed"})
		if policyStops(result) {
			return FlowResult{Status: "escalated", Summary: "Flow escalated by capability policy.", Waves: [][]string{}, Trace: trace, PanelMarkdown: RenderPanelMarkdown(allOpinions)}, nil
		}
	}
	return FlowResult{Status: "failed", Summary: "Flow exceeded bounded visits.", Waves: [][]string{}, Trace: trace, Findings: []Finding{{Level: "BLOCK", Rule: "max_visits", Message: "flow exceeded max_visits"}}}, nil
}

func maxVisits(node Node) int {
	if node.MaxVisits > 0 {
		return node.MaxVisits
	}
	return 1
}

func mapData(value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil, err
	}
	return result, nil
}
