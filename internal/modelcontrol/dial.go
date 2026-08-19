package modelcontrol

// CostQualityDial has intentionally counter-intuitive polarity: zero favors
// the strongest profile and ten favors economy. It controls a concurrency
// ceiling, not a promise that the mission will spend that amount.
type CostQualityDial uint8

func NewCostQualityDial(raw uint8) CostQualityDial {
	if raw > 10 {
		return 10
	}
	return CostQualityDial(raw)
}

func (dial CostQualityDial) LaneHint() Lane {
	switch {
	case dial <= 2:
		return LanePremium
	case dial <= 6:
		return LanePlanner
	case dial <= 8:
		return LaneMedium
	default:
		return LaneCheap
	}
}

func (dial CostQualityDial) MaxParallelAgents() int {
	value := int(dial)
	if value > 10 {
		value = 10
	}
	return 1 + (((8-1)*(10-value))+5)/10
}
