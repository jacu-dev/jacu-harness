package modelcontrol

// Risk and task kind are closed inputs to the deterministic classifier. The
// model never supplies either value.
type Risk string

const (
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
	RiskHigh   Risk = "high"
)

type TaskKind string

const (
	KindPlan        TaskKind = "plan"
	KindReview      TaskKind = "review"
	KindCode        TaskKind = "code"
	KindVerify      TaskKind = "verify"
	KindInstall     TaskKind = "install"
	KindPackage     TaskKind = "package"
	KindAssetImage  TaskKind = "asset_image"
	KindAssetSprite TaskKind = "asset_sprite_strip"
	KindVision      TaskKind = "vision_reference"
)

type ComplexityTier string

const (
	TierLow    ComplexityTier = "low"
	TierMedium ComplexityTier = "medium"
	TierHigh   ComplexityTier = "high"
)

type Lane string

const (
	LaneCheap   Lane = "cheap"
	LaneMedium  Lane = "medium"
	LanePlanner Lane = "planner"
	LanePremium Lane = "premium"
)

type ComplexityInput struct {
	Risk        Risk
	InputTokens uint32
	Criteria    uint32
	Kind        TaskKind
}

type ComplexityResult struct {
	Score int
	Tier  ComplexityTier
	Lane  Lane
}

func Classify(input ComplexityInput) ComplexityResult {
	score := 0
	switch input.Risk {
	case RiskMedium:
		score += 30
	case RiskHigh:
		score += 70
	}
	switch {
	case input.InputTokens > 8000:
		score += 30
	case input.InputTokens > 2000:
		score += 15
	}
	switch {
	case input.Criteria > 5:
		score += 20
	case input.Criteria >= 3:
		score += 10
	}
	switch input.Kind {
	case KindPlan, KindReview:
		score += 20
	case KindAssetImage, KindAssetSprite, KindVision:
		score += 10
	case KindCode, KindVerify, KindInstall, KindPackage:
		score += 5
	}
	result := ComplexityResult{Score: score, Tier: TierLow, Lane: LaneCheap}
	if score > 60 {
		result.Tier, result.Lane = TierHigh, LanePlanner
	} else if score > 30 {
		result.Tier, result.Lane = TierMedium, LaneMedium
	}
	return result
}
