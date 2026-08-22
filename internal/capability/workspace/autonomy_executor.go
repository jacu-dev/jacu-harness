package workspace

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/runner"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

const (
	MissionPending   = "pending"
	MissionApplied   = "applied"
	MissionEscalated = "escalated"
	MissionBlocked   = "blocked"
)

type ProgramMission struct {
	Index int
	After []int
}

type MissionOutcome struct {
	Status         string
	Objective      string
	DiffDigest     string
	Verdict        string
	EvidenceDigest string
	ReceiptRef     string
	Iterations     int
	Warnings       []string
}

func ExecuteProgram(ctx context.Context, missions []ProgramMission, execute func(int) (MissionOutcome, error)) ([]runstate.ProgramMissionState, error) {
	return ExecuteCompiledProgram(ctx, nil, missions, execute, nil)
}

func ExecuteCompiledProgram(ctx context.Context, program *runstate.Program, missions []ProgramMission, execute func(int) (MissionOutcome, error), deliver func() error) ([]runstate.ProgramMissionState, error) {
	deliverAtEnd := program != nil && program.DeliverAtEnd
	return ExecuteProgramWithDelivery(ctx, missions, execute, deliverAtEnd, deliver)
}

func ExecuteProgramWithDelivery(ctx context.Context, missions []ProgramMission, execute func(int) (MissionOutcome, error), deliverAtEnd bool, deliver func() error) ([]runstate.ProgramMissionState, error) {
	if err := validateProgramQueue(missions); err != nil {
		return nil, err
	}
	states := make(map[int]runstate.ProgramMissionState, len(missions))
	for _, mission := range missions {
		states[mission.Index] = runstate.ProgramMissionState{Index: mission.Index, Status: MissionPending}
	}
	ordered := append([]ProgramMission{}, missions...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Index < ordered[j].Index })
	remaining := len(ordered)
	for remaining > 0 {
		progress := false
		for _, mission := range ordered {
			state := states[mission.Index]
			if state.Status != MissionPending {
				continue
			}
			dependencyPending := false
			dependencyEscalated := false
			for _, dependency := range mission.After {
				dependencyState := states[dependency]
				switch dependencyState.Status {
				case MissionPending:
					dependencyPending = true
				case MissionEscalated, MissionBlocked:
					dependencyEscalated = true
				}
			}
			if dependencyPending {
				continue
			}
			if dependencyEscalated {
				state.Status = MissionEscalated
				state.Warnings = []string{"dependency escalated; mission stopped"}
				states[mission.Index] = state
				remaining--
				progress = true
				continue
			}
			if err := ctx.Err(); err != nil {
				state.Status = MissionEscalated
				state.Warnings = []string{"program cancelled: " + err.Error()}
				states[mission.Index] = state
				remaining--
				progress = true
				continue
			}
			outcome, err := execute(mission.Index)
			if err != nil {
				outcome.Status = MissionEscalated
				outcome.Warnings = append(outcome.Warnings, "mission executor error: "+err.Error())
			}
			if outcome.Status == "" {
				outcome.Status = MissionEscalated
			}
			state.Status = outcome.Status
			state.Iterations = outcome.Iterations
			state.Warnings = append([]string{}, outcome.Warnings...)
			state.Audit = &runstate.AuditPackage{
				Objective:      outcome.Objective,
				DiffDigest:     outcome.DiffDigest,
				Verdict:        outcome.Verdict,
				EvidenceDigest: outcome.EvidenceDigest,
				ReceiptRef:     outcome.ReceiptRef,
				Iterations:     outcome.Iterations,
				Warnings:       append([]string{}, outcome.Warnings...),
			}
			states[mission.Index] = state
			remaining--
			progress = true
		}
		if !progress {
			return nil, errors.New("program queue made no progress; dependency cycle or invalid state")
		}
	}
	result := make([]runstate.ProgramMissionState, 0, len(ordered))
	for _, mission := range ordered {
		result = append(result, states[mission.Index])
	}
	if deliverAtEnd && deliver != nil && programReadyToDeliver(result) {
		if err := deliver(); err != nil {
			return result, err
		}
	}
	return result, nil
}

func programReadyToDeliver(states []runstate.ProgramMissionState) bool {
	if len(states) == 0 {
		return false
	}
	for _, state := range states {
		if state.Status != MissionApplied {
			return false
		}
		if state.Audit == nil || state.Audit.Verdict != "pass" {
			return false
		}
	}
	return true
}

func PersistProgramExecution(root string, run *runstate.Run, states []runstate.ProgramMissionState) error {
	run.ProgramMissions = append([]runstate.ProgramMissionState{}, states...)
	run.ProgramCursor = len(states)
	run.ProgramEscalated = run.ProgramEscalated[:0]
	for _, state := range states {
		if state.Status == MissionEscalated || state.Status == MissionBlocked {
			run.ProgramEscalated = append(run.ProgramEscalated, state.Index)
		}
	}
	return runstate.Save(root, *run)
}

func validateProgramQueue(missions []ProgramMission) error {
	if len(missions) == 0 {
		return errors.New("program has no missions")
	}
	seen := make(map[int]bool, len(missions))
	for _, mission := range missions {
		if mission.Index < 0 || seen[mission.Index] {
			return fmt.Errorf("invalid or duplicate mission index %d", mission.Index)
		}
		seen[mission.Index] = true
	}
	for _, mission := range missions {
		for _, dependency := range mission.After {
			if dependency == mission.Index || !seen[dependency] {
				return fmt.Errorf("mission %d has invalid dependency %d", mission.Index, dependency)
			}
		}
	}
	return nil
}

type CheckClass string

const (
	CheckLint  CheckClass = "lint"
	CheckTest  CheckClass = "test"
	CheckBuild CheckClass = "build"
	CheckVuln  CheckClass = "vuln"
	CheckFlaky CheckClass = "flaky"
	CheckOther CheckClass = "other"
)

func ClassifyCheckFailure(check, evidence string) CheckClass {
	lower := strings.ToLower(check + " " + evidence)
	if strings.Contains(lower, "flaky") || strings.Contains(lower, "transient") {
		return CheckFlaky
	}
	switch {
	case strings.Contains(lower, "lint"):
		return CheckLint
	case strings.Contains(lower, "test") || strings.Contains(lower, "e2e"):
		return CheckTest
	case strings.Contains(lower, "build"):
		return CheckBuild
	case strings.Contains(lower, "vuln") || strings.Contains(lower, "security"):
		return CheckVuln
	default:
		return CheckOther
	}
}

type RemediationAction string

const (
	RemediationRerun    RemediationAction = "rerun"
	RemediationMission  RemediationAction = "remediation_mission"
	RemediationEscalate RemediationAction = "escalate"
)

type RemediationDecision struct {
	Action RemediationAction
	Check  string
	Budget int
	Reason string
}

type RemediationMissionSpec struct {
	Objective        string
	Check            string
	Class            CheckClass
	AllowedPaths     []string
	BudgetIterations int
	EvidenceDigest   string
	Reason           string
}

type RemediationPlan struct {
	Decision RemediationDecision
	Mission  *RemediationMissionSpec
}

func NextRemediation(check string, class CheckClass, priorFailures int) RemediationDecision {
	if priorFailures < 0 {
		priorFailures = 0
	}
	if priorFailures >= 2 {
		return RemediationDecision{Action: RemediationEscalate, Check: check, Reason: "same check failed twice after correction"}
	}
	if class == CheckFlaky && priorFailures == 0 {
		return RemediationDecision{Action: RemediationRerun, Check: check, Budget: 1, Reason: "flaky check receives one rerun"}
	}
	return RemediationDecision{Action: RemediationMission, Check: check, Budget: 1, Reason: "create a policy-gated remediation mission"}
}

func BuildRemediationPlan(failure runner.CheckFailureEvidence, priorFailures int) RemediationPlan {
	evidence := strings.Join([]string{
		failure.Check.Name,
		failure.Check.Workflow,
		failure.Check.State,
		failure.LogTail,
		annotationText(failure.Annotations),
	}, " ")
	class := ClassifyCheckFailure(failure.Check.Name, evidence)
	decision := NextRemediation(failure.Check.Name, class, priorFailures)
	if decision.Action != RemediationMission {
		return RemediationPlan{Decision: decision}
	}
	paths := remediationPaths(failure.Annotations)
	if len(paths) == 0 {
		decision.Action = RemediationEscalate
		decision.Budget = 0
		decision.Reason = "remediation evidence has no safe relative path"
		return RemediationPlan{Decision: decision}
	}
	if failure.EvidenceDigest == "" {
		decision.Action = RemediationEscalate
		decision.Budget = 0
		decision.Reason = "remediation evidence has no digest"
		return RemediationPlan{Decision: decision}
	}
	return RemediationPlan{
		Decision: decision,
		Mission: &RemediationMissionSpec{
			Objective:        "make check " + failure.Check.Name + " green",
			Check:            failure.Check.Name,
			Class:            class,
			AllowedPaths:     paths,
			BudgetIterations: decision.Budget,
			EvidenceDigest:   failure.EvidenceDigest,
			Reason:           decision.Reason,
		},
	}
}

func annotationText(annotations []runner.CheckAnnotation) string {
	parts := make([]string, 0, len(annotations)*2)
	for _, annotation := range annotations {
		parts = append(parts, annotation.Path, annotation.Message)
	}
	return strings.Join(parts, " ")
}

func remediationPaths(annotations []runner.CheckAnnotation) []string {
	seen := make(map[string]struct{}, len(annotations))
	paths := make([]string, 0, len(annotations))
	for _, annotation := range annotations {
		candidate := strings.TrimSpace(strings.ReplaceAll(annotation.Path, "\\", "/"))
		if candidate == "" || strings.HasPrefix(candidate, "/") || strings.HasPrefix(candidate, "~") || strings.HasPrefix(candidate, ".git/") {
			continue
		}
		clean := path.Clean(candidate)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, ":") {
			continue
		}
		if _, exists := seen[clean]; exists {
			continue
		}
		seen[clean] = struct{}{}
		paths = append(paths, clean)
	}
	sort.Strings(paths)
	return paths
}
