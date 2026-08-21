package missioncompile

import (
	"context"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/jacu-dev/jacu-harness/internal/capability/preflight"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ToolName = "jacu_mission_compile"

type envelope[T any] struct {
	Status      string   `json:"status"`
	Summary     string   `json:"summary"`
	Data        T        `json:"data,omitempty"`
	Artifacts   []string `json:"artifacts"`
	Warnings    []string `json:"warnings"`
	NextActions []string `json:"next_actions"`
	TraceID     string   `json:"trace_id"`
}

func Compile(root string, in Input) (Mission, string, []string) {
	normalized := normalize(in)
	ceremony := classifyCeremony(normalized)
	lints := lint(root, normalized, ceremony)
	program, programLints := compileProgram(root, normalized.Program)
	lints = append(lints, programLints...)

	for _, item := range lints {
		if item.Level == "BLOCK" {
			mission := emptyMission()
			mission.Lint = lints
			return mission, "blocked", []string{}
		}
	}

	if ceremony == "direct" {
		mission := emptyMission()
		mission.Ceremony = "direct"
		mission.Lint = lints
		return mission, "ok", []string{"host answer suffices; answer the user directly"}
	}

	risk := effectiveRisk(derivedRisk(normalized), normalized.RiskHint)
	mission := Mission{
		MissionID:            missionID(normalized),
		Ceremony:             ceremony,
		Objective:            normalized.Objective,
		AcceptanceCriteria:   append([]string{}, normalized.AcceptanceCriteria...),
		VerificationCommands: append([][]string{}, normalized.VerificationCommands...),
		AllowedPaths:         append([]string{}, normalized.AllowedPaths...),
		ForbiddenPaths:       append([]string{}, normalized.ForbiddenPaths...),
		Risk:                 risk,
		Lint:                 lints,
		Program:              program,
	}
	preflightReport := preflight.Check(mission, preflight.ResolveEnvironment(root, mission))
	if preflightReport.Verdict == "block" {
		mission.Lint = append(mission.Lint, Lint{Level: "BLOCK", Rule: "preflight", Message: "predictable interruption detected"})
		return mission, "blocked", []string{}
	}
	if program != nil {
		program.MissionIDs = make([]string, 0, len(normalizeProgram(normalized.Program).Missions))
		for _, item := range normalizeProgram(normalized.Program).Missions {
			nested, nestedStatus, _ := Compile(root, Input{
				Objective: item.Objective, Context: item.Context, AcceptanceCriteria: item.AcceptanceCriteria,
				VerificationCommands: item.VerificationCommands, AllowedPaths: item.AllowedPaths,
				ForbiddenPaths: item.ForbiddenPaths, RiskHint: item.RiskHint,
			})
			if nestedStatus == "ok" {
				program.MissionIDs = append(program.MissionIDs, nested.MissionID)
			}
		}
	}

	nextActions := []string{}
	for _, item := range lints {
		if item.Level == "WARN" {
			nextActions = append(nextActions, "refine the fields identified by WARN lint and resubmit")
			break
		}
	}
	if _, blocked := admitMissionContext(root, normalized, &mission); blocked {
		return mission, "blocked", []string{}
	}
	return mission, "ok", nextActions
}

func RegisterTool(server *mcp.Server, root string) {
	destructive := false
	openWorld := false
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolName,
		Description: "Compile.",
		InputSchema: inputSchema(),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: &destructive,
			IdempotentHint:  true,
			OpenWorldHint:   &openWorld,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, envelope[Mission], error) {
		result := Run(ctx, root, input)
		data, _ := result.Data.(Mission)
		return nil, envelope[Mission]{
			Status:      result.Status,
			Summary:     result.Summary,
			Data:        data,
			Artifacts:   result.Artifacts,
			Warnings:    result.Warnings,
			NextActions: result.NextActions,
			TraceID:     result.TraceID,
		}, nil
	})
}

func emitPreflightTelemetry(root string, input Input, status string, mission Mission) {
	reports := preflightReports(root, input)
	for _, report := range reports {
		if status == "ok" || report.Verdict == "block" || hasPreflightBlock(mission) {
			preflight.EmitTelemetry(root, report)
		}
	}
}

func preflightReports(root string, input Input) []preflight.Report {
	normalized := normalize(input)
	if classifyCeremony(normalized) == "direct" {
		return nil
	}
	mission := Mission{
		Objective:            normalized.Objective,
		VerificationCommands: append([][]string{}, normalized.VerificationCommands...),
		AllowedPaths:         append([]string{}, normalized.AllowedPaths...),
		ForbiddenPaths:       append([]string{}, normalized.ForbiddenPaths...),
	}
	if normalized.Program != nil {
		mission.Program = &Program{OpenQuestions: append([]string{}, normalized.Program.OpenQuestions...)}
	}
	reports := []preflight.Report{preflight.Check(mission, preflight.ResolveEnvironment(root, mission))}
	if normalized.Program != nil {
		for _, nested := range normalized.Program.Missions {
			reports = append(reports, preflightReports(root, Input{
				Objective:            nested.Objective,
				Context:              nested.Context,
				AcceptanceCriteria:   nested.AcceptanceCriteria,
				VerificationCommands: nested.VerificationCommands,
				AllowedPaths:         nested.AllowedPaths,
				ForbiddenPaths:       nested.ForbiddenPaths,
				RiskHint:             nested.RiskHint,
			})...)
		}
	}
	return reports
}

func hasPreflightBlock(mission Mission) bool {
	for _, item := range mission.Lint {
		if item.Level == "BLOCK" && item.Rule == "preflight" {
			return true
		}
	}
	return false
}

func emptyMission() Mission {
	return Mission{
		AcceptanceCriteria:   []string{},
		VerificationCommands: [][]string{},
		AllowedPaths:         []string{},
		ForbiddenPaths:       []string{},
		Lint:                 []Lint{},
	}
}

// inputSchema announces the risk_hint enum to the host. Declaring the tier at
// the schema boundary is what stops a malformed value from ever reaching the
// lint: the SDK rejects it before the handler runs, so the host learns the
// contract from the tool description instead of from a refusal.
//
// The schema is inferred from Input and then patched, rather than hand-written,
// so a field added to the struct cannot silently fall out of the advertised
// contract.
func inputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[Input](nil)
	if err != nil {
		panic("missioncompile: infer input schema: " + err.Error())
	}
	riskHint, ok := schema.Properties["risk_hint"]
	if !ok {
		panic("missioncompile: input schema lost risk_hint")
	}
	riskHint.Enum = []any{"safe", "write", "destructive"}
	return schema
}
