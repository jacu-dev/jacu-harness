package runtime

import (
	"errors"
	"regexp"
	"time"
)

type RiskLevel string

const (
	RiskSafe        RiskLevel = "safe"
	RiskWrite       RiskLevel = "write"
	RiskDestructive RiskLevel = "destructive"
)

type ToolSpec struct {
	Name           string
	Version        string
	Risk           RiskLevel
	ReadOnly       bool
	Idempotent     bool
	OpenWorld      bool
	Timeout        time.Duration
	MaxInputBytes  int64
	MaxOutputBytes int64
}

type Result struct {
	Status      string   `json:"status"`
	Summary     string   `json:"summary"`
	Data        any      `json:"data,omitempty"`
	Artifacts   []string `json:"artifacts"`
	Warnings    []string `json:"warnings"`
	NextActions []string `json:"next_actions"`
	TraceID     string   `json:"trace_id"`
}

var toolNamePattern = regexp.MustCompile(`^jacu_[a-z0-9_]+$`)

func (s ToolSpec) Validate() error {
	if !toolNamePattern.MatchString(s.Name) {
		return errors.New("tool name must match ^jacu_[a-z0-9_]+$")
	}
	if s.Timeout <= 0 {
		return errors.New("tool timeout must be positive")
	}
	if s.MaxInputBytes <= 0 || s.MaxOutputBytes <= 0 {
		return errors.New("tool input and output caps must be positive")
	}
	switch s.Risk {
	case RiskSafe:
		if !s.ReadOnly {
			return errors.New("safe tool must be read-only")
		}
	case RiskWrite, RiskDestructive:
	default:
		return errors.New("unknown tool risk")
	}
	return nil
}
