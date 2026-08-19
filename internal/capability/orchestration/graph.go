package orchestration

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	UseMission   = "mission"
	UseWorkspace = "workspace"
	UseVerify    = "verify"
	UseReview    = "review"
	UseApply     = "apply"
)

var knownUses = map[string]struct{}{
	UseMission: {}, UseWorkspace: {}, UseVerify: {}, UseReview: {}, UseApply: {},
}

var verifyVerdicts = []string{"pass", "fail", "timeout", "blocked", "not_run"}

// Flow is the declarative graph submitted to jacu_flow_run.
type Flow struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type Node struct {
	ID           string         `json:"id"`
	Uses         string         `json:"uses"`
	With         map[string]any `json:"with,omitempty"`
	AllowedPaths []string       `json:"allowed_paths,omitempty"`
	MaxVisits    int            `json:"max_visits,omitempty"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	When string `json:"when"`
}

type Finding struct {
	Level   string `json:"level"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
	NodeID  string `json:"node_id,omitempty"`
	Field   string `json:"field,omitempty"`
}

type Validation struct {
	Findings []Finding `json:"findings"`
}

func (v Validation) Blocked() bool {
	for _, finding := range v.Findings {
		if finding.Level == "BLOCK" {
			return true
		}
	}
	return false
}

// Validate performs the submit-time, side-effect-free flow lint.
func Validate(flow Flow) Validation {
	findings := make([]Finding, 0)
	if len(flow.Nodes) == 0 {
		return Validation{Findings: []Finding{{Level: "BLOCK", Rule: "empty_flow", Message: "flow must contain at least one node", Field: "flow.nodes"}}}
	}

	nodes := make(map[string]Node, len(flow.Nodes))
	for _, node := range flow.Nodes {
		id := strings.TrimSpace(node.ID)
		if id == "" {
			findings = append(findings, Finding{Level: "BLOCK", Rule: "empty_node_id", Message: "node id is required", Field: "flow.nodes.id"})
			continue
		}
		if _, exists := nodes[id]; exists {
			findings = append(findings, Finding{Level: "BLOCK", Rule: "duplicate_node_id", Message: "node id is duplicated", NodeID: id})
			continue
		}
		node.ID = id
		if _, known := knownUses[node.Uses]; !known {
			findings = append(findings, Finding{Level: "BLOCK", Rule: "unknown_capability", Message: "flow node uses an unknown capability", NodeID: id, Field: "uses"})
		}
		if node.MaxVisits < 0 {
			findings = append(findings, Finding{Level: "BLOCK", Rule: "invalid_max_visits", Message: "max_visits must not be negative", NodeID: id})
		}
		nodes[id] = node
	}

	adj := make(map[string][]string, len(nodes))
	indegree := make(map[string]int, len(nodes))
	degree := make(map[string]int, len(nodes))
	for id := range nodes {
		indegree[id] = 0
	}
	seenEdges := make(map[string]struct{}, len(flow.Edges))
	outgoing := make(map[string][]parsedCondition, len(nodes))
	for _, edge := range flow.Edges {
		from, to := strings.TrimSpace(edge.From), strings.TrimSpace(edge.To)
		fromNode, fromOK := nodes[from]
		_, toOK := nodes[to]
		if !fromOK || !toOK {
			findings = append(findings, Finding{Level: "BLOCK", Rule: "dangling_edge", Message: "edge must reference existing nodes", Field: "flow.edges"})
			continue
		}
		key := from + "\x00" + to
		if _, duplicate := seenEdges[key]; !duplicate {
			seenEdges[key] = struct{}{}
			adj[from] = append(adj[from], to)
			indegree[to]++
			degree[from]++
			degree[to]++
		}
		condition, err := parseCondition(edge.When)
		if err != nil {
			findings = append(findings, Finding{Level: "BLOCK", Rule: "invalid_condition", Message: err.Error(), Field: "flow.edges.when"})
			continue
		}
		if condition.Field == "verdict" && fromNode.Uses != UseVerify {
			findings = append(findings, Finding{Level: "BLOCK", Rule: "verdict_source_not_verify", Message: "verdict conditions require a verify node", NodeID: from})
		}
		if condition.Field == "verdict" && condition.Value == "not_run" && nodes[to].Uses == UseApply {
			findings = append(findings, Finding{Level: "BLOCK", Rule: "not_run_apply", Message: "not_run must never route to apply", NodeID: to})
		}
		if condition.Default && fromNode.Uses == UseVerify && nodes[to].Uses == UseApply {
			findings = append(findings, Finding{Level: "BLOCK", Rule: "default_apply", Message: "verify default edge must not route to apply", NodeID: to})
		}
		outgoing[from] = append(outgoing[from], condition)
	}

	for id, node := range nodes {
		if len(flow.Edges) > 0 && len(nodes) > 1 && degree[id] == 0 {
			findings = append(findings, Finding{Level: "BLOCK", Rule: "orphan_node", Message: "node is disconnected from the flow", NodeID: id})
		}
		if node.Uses != UseVerify || len(outgoing[id]) == 0 {
			continue
		}
		covered := make(map[string]bool, len(verifyVerdicts))
		defaulted := false
		for _, condition := range outgoing[id] {
			if condition.Default {
				defaulted = true
			}
			if condition.Field == "verdict" {
				covered[condition.Value] = true
			}
		}
		if !defaulted {
			missing := make([]string, 0)
			for _, verdict := range verifyVerdicts {
				if !covered[verdict] {
					missing = append(missing, verdict)
				}
			}
			if len(missing) > 0 {
				findings = append(findings, Finding{Level: "BLOCK", Rule: "incomplete_verdict_edges", Message: "verify edges must cover: " + strings.Join(missing, ", "), NodeID: id})
			}
		}
	}

	if cycleNodes := residualCycleNodes(adj, indegree); len(cycleNodes) > 0 {
		bounded := false
		for _, id := range cycleNodes {
			if nodes[id].MaxVisits > 0 {
				bounded = true
				break
			}
		}
		if !bounded {
			findings = append(findings, Finding{Level: "BLOCK", Rule: "unbounded_cycle", Message: "cycle requires max_visits", Field: "flow.nodes.max_visits"})
		}
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Rule != findings[j].Rule {
			return findings[i].Rule < findings[j].Rule
		}
		return findings[i].NodeID < findings[j].NodeID
	})
	return Validation{Findings: findings}
}

type parsedCondition struct {
	Field   string
	Value   string
	Default bool
}

func parseCondition(raw string) (parsedCondition, error) {
	raw = strings.TrimSpace(raw)
	if raw == "default" {
		return parsedCondition{Default: true}, nil
	}
	parts := strings.Fields(raw)
	if len(parts) != 3 || parts[1] != "==" {
		return parsedCondition{}, errors.New("condition must be a closed equality or default")
	}
	if parts[0] != "verdict" && parts[0] != "policy" {
		return parsedCondition{}, errors.New("condition field must be verdict or policy")
	}
	if parts[0] == "verdict" {
		found := false
		for _, verdict := range verifyVerdicts {
			if parts[2] == verdict {
				found = true
				break
			}
		}
		if !found {
			return parsedCondition{}, errors.New("condition verdict is outside the verify contract")
		}
	}
	if parts[0] == "policy" {
		allowed := map[string]bool{"pass": true, "require_approval": true, "blocked": true, "escalate": true}
		if !allowed[parts[2]] {
			return parsedCondition{}, errors.New("condition policy is outside the policy contract")
		}
	}
	return parsedCondition{Field: parts[0], Value: parts[2]}, nil
}

func residualCycleNodes(adj map[string][]string, indegree map[string]int) []string {
	degrees := make(map[string]int, len(indegree))
	ready := make([]string, 0)
	for id, degree := range indegree {
		degrees[id] = degree
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	for len(ready) > 0 {
		sort.Strings(ready)
		id := ready[0]
		ready = ready[1:]
		for _, successor := range adj[id] {
			degrees[successor]--
			if degrees[successor] == 0 {
				ready = append(ready, successor)
			}
		}
	}
	cycleNodes := make([]string, 0)
	for id, degree := range degrees {
		if degree > 0 {
			cycleNodes = append(cycleNodes, id)
		}
	}
	sort.Strings(cycleNodes)
	return cycleNodes
}

type ScheduleError struct {
	Kind string
	From string
	To   string
	Node string
}

func (e *ScheduleError) Error() string {
	switch e.Kind {
	case "dangling":
		return fmt.Sprintf("dangling edge %q -> %q", e.From, e.To)
	case "cycle":
		return fmt.Sprintf("cycle involving %q", e.Node)
	default:
		return e.Kind
	}
}

// ScheduleWaves is the inherited deterministic scheduler. It intentionally
// accepts duplicate node ids and keeps the first occurrence: validation is the
// submit-time contract, while this pure helper preserves the migrated behavior
// and remains useful to callers that already linted elsewhere.
func ScheduleWaves(flow Flow) ([][]string, error) {
	nodes := make(map[string]Node, len(flow.Nodes))
	for _, node := range flow.Nodes {
		if _, exists := nodes[node.ID]; !exists {
			nodes[node.ID] = node
		}
	}
	indegree := make(map[string]int, len(nodes))
	successors := make(map[string]map[string]struct{}, len(nodes))
	predecessors := make(map[string]map[string]struct{}, len(nodes))
	for id := range nodes {
		indegree[id] = 0
		successors[id] = make(map[string]struct{})
		predecessors[id] = make(map[string]struct{})
	}
	for _, edge := range flow.Edges {
		if _, ok := nodes[edge.From]; !ok {
			return nil, &ScheduleError{Kind: "dangling", From: edge.From, To: edge.To}
		}
		if _, ok := nodes[edge.To]; !ok {
			return nil, &ScheduleError{Kind: "dangling", From: edge.From, To: edge.To}
		}
		if _, exists := successors[edge.From][edge.To]; exists {
			continue
		}
		successors[edge.From][edge.To] = struct{}{}
		predecessors[edge.To][edge.From] = struct{}{}
		indegree[edge.To]++
	}

	ready := make([]string, 0)
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	order := make([]string, 0, len(nodes))
	for len(ready) > 0 {
		sort.Strings(ready)
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		successorIDs := make([]string, 0, len(successors[id]))
		for successor := range successors[id] {
			successorIDs = append(successorIDs, successor)
		}
		sort.Strings(successorIDs)
		for _, successor := range successorIDs {
			indegree[successor]--
			if indegree[successor] == 0 {
				ready = append(ready, successor)
			}
		}
	}
	if len(order) != len(nodes) {
		residual := make([]string, 0)
		for id, degree := range indegree {
			if degree > 0 {
				residual = append(residual, id)
			}
		}
		sort.Strings(residual)
		return nil, &ScheduleError{Kind: "cycle", Node: residual[0]}
	}

	waves := make([][]string, 0)
	waveOf := make(map[string]int, len(nodes))
	for _, id := range order {
		minWave := 0
		for predecessor := range predecessors[id] {
			if waveOf[predecessor]+1 > minWave {
				minWave = waveOf[predecessor] + 1
			}
		}
		wave := minWave
		for {
			if wave == len(waves) {
				waves = append(waves, []string{})
			}
			conflict := false
			for _, other := range waves[wave] {
				if scopesConflict(nodes[id].AllowedPaths, nodes[other].AllowedPaths) {
					conflict = true
					break
				}
			}
			if !conflict {
				waves[wave] = append(waves[wave], id)
				waveOf[id] = wave
				break
			}
			wave++
		}
	}
	for index := range waves {
		sort.Strings(waves[index])
	}
	return waves, nil
}

func normalizeScope(path string) string {
	path = strings.TrimSpace(path)
	if index := strings.IndexAny(path, "*?"); index >= 0 {
		path = path[:index]
	}
	return strings.TrimSuffix(path, "/")
}

func scopesConflict(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return true
	}
	for _, raw := range left {
		if normalizeScope(raw) == "" {
			return true
		}
	}
	for _, raw := range right {
		if normalizeScope(raw) == "" {
			return true
		}
	}
	for _, leftPath := range left {
		for _, rightPath := range right {
			leftPath, rightPath = normalizeScope(leftPath), normalizeScope(rightPath)
			if leftPath == rightPath || dirPrefix(leftPath, rightPath) || dirPrefix(rightPath, leftPath) {
				return true
			}
		}
	}
	return false
}

func dirPrefix(prefix, path string) bool {
	return prefix != "" && len(path) > len(prefix) && strings.HasPrefix(path, prefix) && path[len(prefix)] == '/'
}
