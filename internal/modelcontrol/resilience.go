package modelcontrol

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type ModelMetric struct {
	ProfileID    string
	TaskKind     string
	TotalRuns    uint32
	AcceptedRuns uint32
}

type DemotionDecision struct {
	ProfileID   string
	TaskKind    string
	FailureRate float64
	Samples     uint32
	Demoted     bool
}

func EvaluateDemotion(metric ModelMetric) DemotionDecision {
	rate := 0.0
	if metric.TotalRuns > 0 {
		accepted := metric.AcceptedRuns
		if accepted > metric.TotalRuns {
			accepted = metric.TotalRuns
		}
		rate = float64(metric.TotalRuns-accepted) / float64(metric.TotalRuns)
	}
	return DemotionDecision{ProfileID: metric.ProfileID, TaskKind: metric.TaskKind, FailureRate: rate, Samples: metric.TotalRuns, Demoted: metric.TotalRuns >= 10 && rate > 0.40}
}

func ApplyDemotionBias(candidates []HostProfile, metrics []ModelMetric, taskKind string) ([]HostProfile, []string) {
	events := make([]string, 0)
	demoted := make(map[string]bool)
	for _, metric := range metrics {
		if metric.TaskKind != taskKind {
			continue
		}
		decision := EvaluateDemotion(metric)
		if decision.Demoted {
			demoted[decision.ProfileID] = true
			events = append(events, fmt.Sprintf("profile %s demoted for %s (fail %.0f%%, n=%d)", sanitize(metric.ProfileID), sanitize(taskKind), decision.FailureRate*100, decision.Samples))
		}
	}
	kept := make([]HostProfile, 0, len(candidates))
	for _, candidate := range candidates {
		if !demoted[candidate.ID] {
			kept = append(kept, candidate)
		}
	}
	if len(kept) == 0 {
		return candidates, events
	}
	return kept, events
}

type FailureClass string

const (
	FailureVerification FailureClass = "verification_failed"
	FailureReview       FailureClass = "review_rejected"
	FailureProvider     FailureClass = "provider_error"
	FailureBudget       FailureClass = "budget_exceeded"
	FailureTimeout      FailureClass = "timeout"
)

type HumanFlag string

const (
	RequireHuman HumanFlag = "require_human"
	FailClosed   HumanFlag = "fail_closed"
)

type TierTarget struct {
	Lane      Lane
	ProfileID string
}

type EscalationPolicy struct {
	MaxAttempts uint8
	Tiers       []TierTarget
	OnExhaust   HumanFlag
}

type EscalationKind string

const (
	Retry     EscalationKind = "retry"
	Exhausted EscalationKind = "exhausted"
)

type Escalation struct {
	Kind   EscalationKind
	Next   TierTarget
	Flag   HumanFlag
	Reason string
}

func (policy EscalationPolicy) Validate() error {
	if policy.MaxAttempts < 1 || policy.MaxAttempts > 5 {
		return fmt.Errorf("max_attempts must be 1..=5, got %d", policy.MaxAttempts)
	}
	if len(policy.Tiers) == 0 {
		return errors.New("escalation policy needs at least one tier")
	}
	for _, tier := range policy.Tiers {
		if strings.TrimSpace(tier.ProfileID) == "" {
			return errors.New("escalation tier has empty profile_id")
		}
	}
	return nil
}

func (policy EscalationPolicy) Next(attempt uint8, failure FailureClass) Escalation {
	if failure == FailureBudget || attempt == 0 || attempt >= policy.MaxAttempts || int(attempt) >= len(policy.Tiers) {
		return Escalation{Kind: Exhausted, Flag: policy.OnExhaust, Reason: reasonForExhaustion(failure, attempt, policy.MaxAttempts)}
	}
	return Escalation{Kind: Retry, Next: policy.Tiers[attempt]}
}

func reasonForExhaustion(failure FailureClass, attempt, max uint8) string {
	if failure == FailureBudget {
		return "budget_exceeded"
	}
	if attempt >= max {
		return "attempts_exhausted"
	}
	return "fail_closed"
}

type breakerState uint8

const (
	breakerClosed breakerState = iota
	breakerOpen
	breakerHalfOpen
)

type CircuitBreaker struct {
	mu               sync.Mutex
	failureThreshold int
	cooldown         time.Duration
	failures         int
	state            breakerState
	openedAt         time.Time
}

func NewCircuitBreaker(failureThreshold int, cooldown time.Duration) *CircuitBreaker {
	if failureThreshold < 1 {
		failureThreshold = 1
	}
	if cooldown < 0 {
		cooldown = 0
	}
	return &CircuitBreaker{failureThreshold: failureThreshold, cooldown: cooldown}
}

func (breaker *CircuitBreaker) Allow(now time.Time) bool {
	if breaker == nil {
		return false
	}
	breaker.mu.Lock()
	defer breaker.mu.Unlock()
	switch breaker.state {
	case breakerOpen:
		if now.Sub(breaker.openedAt) < breaker.cooldown {
			return false
		}
		breaker.state = breakerHalfOpen
		return true
	case breakerHalfOpen:
		return false
	default:
		return true
	}
}

func (breaker *CircuitBreaker) Record(now time.Time, success bool) {
	if breaker == nil {
		return
	}
	breaker.mu.Lock()
	defer breaker.mu.Unlock()
	if success {
		breaker.failures = 0
		breaker.state = breakerClosed
		return
	}
	if breaker.state == breakerHalfOpen || breaker.failures+1 >= breaker.failureThreshold {
		breaker.state = breakerOpen
		breaker.openedAt = now
		breaker.failures = breaker.failureThreshold
		return
	}
	breaker.failures++
}

type InvocationResult struct {
	Success bool
}

func GuardedInvoke(ctx context.Context, profile HostProfile, actualDigest string, verifier SignatureVerifier, breaker *CircuitBreaker, now time.Time, invoke func(context.Context, HostProfile) (InvocationResult, error)) (InvocationResult, error) {
	if err := ValidateSignedCLI(profile.CLI, verifier); err != nil {
		return InvocationResult{}, err
	}
	if actualDigest != profile.CLI.SHA256 {
		return InvocationResult{}, errors.New("CLI binary digest does not match attested digest")
	}
	if breaker == nil || !breaker.Allow(now) {
		return InvocationResult{}, errors.New("host profile circuit breaker is open")
	}
	if invoke == nil {
		breaker.Record(now, false)
		return InvocationResult{}, errors.New("host profile invoker is nil")
	}
	result, err := invoke(ctx, profile)
	breaker.Record(now, err == nil && result.Success)
	return result, err
}

func sanitize(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' || char == ' ' {
			builder.WriteRune(char)
		}
		if builder.Len() >= 80 {
			break
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}
