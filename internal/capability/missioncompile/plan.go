package missioncompile

// PlanReady is the mechanical plan-mode gate. Programs use open_questions as
// their decision points so the compile schema remains within the ADR-008
// context budget; an empty list is the only executable state.
func PlanReady(program *Program) bool {
	return program == nil || len(program.OpenQuestions) == 0
}
