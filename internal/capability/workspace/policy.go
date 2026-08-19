package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// AutonomyPolicy is the immutable, project-root policy for automatic apply.
// It intentionally contains no executor/session identity: the receipt is an
// audit artifact, while this policy only decides whether the runtime may act.
type AutonomyPolicy struct {
	Require       []string `json:"require"`
	RiskMax       string   `json:"risk_max"`
	MaxIterations int      `json:"max_iterations"`
	OnViolation   string   `json:"on_violation"`
}

type PolicyDecision struct {
	Allowed  bool
	Escalate bool
	Reason   string
}

type autonomyPolicyDocument struct {
	Policy *struct {
		AutoApply *AutonomyPolicy `json:"auto_apply"`
	} `json:"policy"`
}

var validPolicyRequirements = map[string]bool{
	"verify_pass":  true,
	"cross_review": true,
	"risk_tier":    true,
}

var riskRank = map[string]int{
	"safe":        0,
	"write":       1,
	"destructive": 2,
}

func LoadAutonomyPolicy(root string) (AutonomyPolicy, bool, error) {
	for _, name := range []string{"autonomy-policy.json", "autonomy-policy.yaml", "autonomy-policy.yml"} {
		path := filepath.Join(root, ".jacu", name)
		// #nosec G304 -- path is constrained to the repository root's .jacu policy names.
		content, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return AutonomyPolicy{}, false, fmt.Errorf("read autonomy policy: %w", err)
		}
		var policy AutonomyPolicy
		if strings.HasSuffix(name, ".json") {
			policy, err = decodeAutonomyPolicyJSON(content)
		} else {
			policy, err = decodeAutonomyPolicyYAML(content)
		}
		if err != nil {
			return AutonomyPolicy{}, false, err
		}
		return policy, true, nil
	}
	return AutonomyPolicy{}, false, nil
}

func decodeAutonomyPolicyJSON(content []byte) (AutonomyPolicy, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var document autonomyPolicyDocument
	if err := decoder.Decode(&document); err != nil {
		return AutonomyPolicy{}, fmt.Errorf("decode autonomy policy: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return AutonomyPolicy{}, errors.New("decode autonomy policy: trailing JSON")
		}
		return AutonomyPolicy{}, fmt.Errorf("decode autonomy policy: %w", err)
	}
	if document.Policy == nil || document.Policy.AutoApply == nil {
		return AutonomyPolicy{}, errors.New("autonomy policy: policy.auto_apply is required")
	}
	return validateAutonomyPolicy(*document.Policy.AutoApply)
}

// decodeAutonomyPolicyYAML deliberately supports only the small declarative
// policy shape documented for this phase. Keeping the parser narrow avoids a
// new YAML dependency and fails closed on every unknown key.
func decodeAutonomyPolicyYAML(content []byte) (AutonomyPolicy, error) {
	var policy AutonomyPolicy
	seenPolicy, seenAuto := false, false
	seen := map[string]bool{}
	for lineNo, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(strings.SplitN(raw, "#", 2)[0])
		if line == "" {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if indent == 0 && line == "policy:" {
			seenPolicy = true
			continue
		}
		if indent == 2 && line == "auto_apply:" {
			seenAuto = true
			continue
		}
		if indent != 4 || !seenPolicy || !seenAuto {
			return AutonomyPolicy{}, fmt.Errorf("decode autonomy policy YAML line %d: unexpected structure", lineNo+1)
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return AutonomyPolicy{}, fmt.Errorf("decode autonomy policy YAML line %d: expected key", lineNo+1)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if seen[key] {
			return AutonomyPolicy{}, fmt.Errorf("decode autonomy policy YAML line %d: duplicate key %q", lineNo+1, key)
		}
		seen[key] = true
		switch key {
		case "require":
			if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
				return AutonomyPolicy{}, fmt.Errorf("decode autonomy policy YAML line %d: require must be an inline list", lineNo+1)
			}
			for _, item := range strings.Split(strings.TrimSpace(value[1:len(value)-1]), ",") {
				item = strings.Trim(strings.TrimSpace(item), "\"'")
				if item != "" {
					policy.Require = append(policy.Require, item)
				}
			}
		case "risk_max":
			policy.RiskMax = strings.Trim(value, "\"'")
		case "max_iterations":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return AutonomyPolicy{}, fmt.Errorf("decode autonomy policy YAML line %d: max_iterations: %w", lineNo+1, err)
			}
			policy.MaxIterations = parsed
		case "on_violation":
			policy.OnViolation = strings.Trim(value, "\"'")
		default:
			return AutonomyPolicy{}, fmt.Errorf("decode autonomy policy YAML line %d: unknown key %q", lineNo+1, key)
		}
	}
	if !seenPolicy || !seenAuto {
		return AutonomyPolicy{}, errors.New("autonomy policy YAML: policy.auto_apply is required")
	}
	return validateAutonomyPolicy(policy)
}

func validateAutonomyPolicy(policy AutonomyPolicy) (AutonomyPolicy, error) {
	if len(policy.Require) == 0 {
		return AutonomyPolicy{}, errors.New("autonomy policy: require must not be empty")
	}
	seen := make(map[string]bool, len(policy.Require))
	for _, requirement := range policy.Require {
		if !validPolicyRequirements[requirement] {
			return AutonomyPolicy{}, fmt.Errorf("autonomy policy: unknown requirement %q", requirement)
		}
		if seen[requirement] {
			return AutonomyPolicy{}, fmt.Errorf("autonomy policy: duplicate requirement %q", requirement)
		}
		seen[requirement] = true
	}
	if !seen["verify_pass"] || !seen["cross_review"] {
		return AutonomyPolicy{}, errors.New("autonomy policy: require must include verify_pass and cross_review")
	}
	if riskRank[policy.RiskMax] == 0 && policy.RiskMax != "safe" {
		return AutonomyPolicy{}, fmt.Errorf("autonomy policy: invalid risk_max %q", policy.RiskMax)
	}
	if policy.MaxIterations <= 0 {
		return AutonomyPolicy{}, errors.New("autonomy policy: max_iterations must be positive")
	}
	if policy.OnViolation != "block" && policy.OnViolation != "escalate" {
		return AutonomyPolicy{}, fmt.Errorf("autonomy policy: invalid on_violation %q", policy.OnViolation)
	}
	policy.Require = append([]string(nil), policy.Require...)
	return policy, nil
}

func EvaluateAutoApplyPolicy(policy AutonomyPolicy, verdict, risk string, receiptValid bool, iteration int) PolicyDecision {
	violate := func(reason string) PolicyDecision {
		return PolicyDecision{Allowed: false, Escalate: policy.OnViolation == "escalate", Reason: reason}
	}
	if !validPolicyRequirements["verify_pass"] || verdict != "pass" {
		return violate("verify_pass requires verdict pass")
	}
	if !receiptValid {
		return violate("cross_review receipt is invalid or absent")
	}
	if riskRank[risk] > riskRank[policy.RiskMax] {
		return violate("risk tier exceeds policy risk_max")
	}
	if riskRank[risk] == 0 && risk != "safe" || riskRank[policy.RiskMax] == 0 && policy.RiskMax != "safe" {
		return violate("unknown risk tier")
	}
	if iteration <= 0 || iteration > policy.MaxIterations {
		return violate("mission iteration budget exhausted")
	}
	return PolicyDecision{Allowed: true, Reason: "policy requirements satisfied"}
}
