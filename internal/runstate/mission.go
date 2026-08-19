package runstate

// MissionInput is the persisted shape of the input used to compile a mission.
// It lives in runstate so the on-disk contract does not depend on a capability.
type MissionInput struct {
	Objective            string        `json:"objective"`
	Context              Context       `json:"context,omitempty"`
	AcceptanceCriteria   []string      `json:"acceptance_criteria,omitempty"`
	VerificationCommands [][]string    `json:"verification_commands,omitempty"`
	AllowedPaths         []string      `json:"allowed_paths,omitempty"`
	ForbiddenPaths       []string      `json:"forbidden_paths,omitempty"`
	RiskHint             string        `json:"risk_hint,omitempty"`
	Program              *ProgramInput `json:"program,omitempty"`
}

type ProgramInput struct {
	OpenQuestions []string              `json:"open_questions"`
	Missions      []ProgramMissionInput `json:"missions"`
}

type ProgramMissionInput struct {
	Objective            string     `json:"objective"`
	Context              Context    `json:"context,omitempty"`
	AcceptanceCriteria   []string   `json:"acceptance_criteria,omitempty"`
	VerificationCommands [][]string `json:"verification_commands,omitempty"`
	AllowedPaths         []string   `json:"allowed_paths,omitempty"`
	ForbiddenPaths       []string   `json:"forbidden_paths,omitempty"`
	RiskHint             string     `json:"risk_hint,omitempty"`
	After                []int      `json:"after,omitempty"`
}

type Context struct {
	ProjectID string   `json:"project_id,omitempty"`
	Refs      []string `json:"refs,omitempty"`
}

type Lint struct {
	Level   string `json:"level"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

type MissionSnapshot struct {
	MissionID            string     `json:"mission_id"`
	Ceremony             string     `json:"ceremony"`
	Objective            string     `json:"objective"`
	AcceptanceCriteria   []string   `json:"acceptance_criteria"`
	VerificationCommands [][]string `json:"verification_commands"`
	AllowedPaths         []string   `json:"allowed_paths"`
	ForbiddenPaths       []string   `json:"forbidden_paths"`
	Risk                 string     `json:"risk"`
	Lint                 []Lint     `json:"lint"`
	Program              *Program   `json:"program,omitempty"`
}

type Program struct {
	ProgramID     string   `json:"program_id"`
	MissionIDs    []string `json:"mission_ids"`
	OpenQuestions []string `json:"open_questions"`
}
