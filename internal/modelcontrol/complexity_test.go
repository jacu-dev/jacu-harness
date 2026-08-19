package modelcontrol

import "testing"

func TestClassifyUsesDocumentedWeightsAndLanes(t *testing.T) {
	tests := []struct {
		name  string
		input ComplexityInput
		score int
		tier  ComplexityTier
		lane  Lane
	}{
		{name: "small code", input: ComplexityInput{Risk: RiskLow, Kind: KindCode, InputTokens: 1000, Criteria: 1}, score: 5, tier: TierLow, lane: LaneCheap},
		{name: "medium boundary", input: ComplexityInput{Risk: RiskMedium, Kind: KindCode, InputTokens: 4000, Criteria: 3}, score: 60, tier: TierMedium, lane: LaneMedium},
		{name: "high dominates", input: ComplexityInput{Risk: RiskHigh}, score: 70, tier: TierHigh, lane: LanePlanner},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.input)
			if got.Score != tt.score || got.Tier != tt.tier || got.Lane != tt.lane {
				t.Fatalf("classify = %#v; want score=%d tier=%q lane=%q", got, tt.score, tt.tier, tt.lane)
			}
		})
	}
}

func TestClassifyClampsUnknownKindToSafeWeight(t *testing.T) {
	got := Classify(ComplexityInput{Kind: TaskKind("untrusted")})
	if got.Score != 0 || got.Tier != TierLow {
		t.Fatalf("unknown kind classify = %#v; want low zero score", got)
	}
}

func TestCostQualityDialUsesTheDocumentedCurve(t *testing.T) {
	want := []int{8, 7, 7, 6, 5, 5, 4, 3, 2, 2, 1}
	for raw, expected := range want {
		dial := NewCostQualityDial(uint8(raw))
		if got := dial.MaxParallelAgents(); got != expected {
			t.Fatalf("dial %d parallelism = %d; want %d", raw, got, expected)
		}
	}
	if NewCostQualityDial(200).MaxParallelAgents() != 1 || NewCostQualityDial(0).LaneHint() != LanePremium || NewCostQualityDial(10).LaneHint() != LaneCheap {
		t.Fatal("dial clamp or lane polarity is wrong")
	}
}
