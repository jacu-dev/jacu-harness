package hosteval

import "testing"

// GeminiMustNotBeRouted is the panel gate: Gemini is not a route target
// until a host-harness case exists and passes.
const GeminiMustNotBeRouted = true

func TestGeminiIsNotARouteTargetUntilHarnessPasses(t *testing.T) {
	t.Parallel()
	if !GeminiMustNotBeRouted {
		t.Fatal("panel must not route to Gemini before host-harness cases exist")
	}
}
