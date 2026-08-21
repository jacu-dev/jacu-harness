package orchestration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/modelcontrol"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

// Panel is the injected host-profile catalogue. Empty panel skips routing
// unless a node declares lane.
type Panel struct {
	Profiles []modelcontrol.HostProfile
	Verifier modelcontrol.SignatureVerifier
	Breaker  *modelcontrol.CircuitBreaker
	Metrics  []modelcontrol.ModelMetric
	Now      time.Time
}

var activePanel Panel

func SetPanel(panel Panel) { activePanel = panel }

func RouteNode(ctx context.Context, panel Panel, node Node) (modelcontrol.HostProfile, error) {
	if namedCLI(node) {
		return modelcontrol.HostProfile{}, errors.New("flow node names a CLI")
	}
	classified := modelcontrol.Classify(modelcontrol.ComplexityInput{Risk: modelcontrol.RiskLow, Kind: modelcontrol.KindCode})
	lane := modelcontrol.Lane(node.Lane)
	if lane == "" {
		lane = classified.Lane
	}
	request := modelcontrol.RouteRequest{Lane: lane, Contract: "code.patch.v1", RequiredCapability: 0, InputTokens: 0, OutputTokens: 0}
	routed, _, err := modelcontrol.Route(panel.Profiles, request, panel.Verifier)
	if err != nil {
		return modelcontrol.HostProfile{}, err
	}
	candidates := []modelcontrol.HostProfile{routed}
	for _, profile := range panel.Profiles {
		if profile.ID != routed.ID {
			candidates = append(candidates, profile)
		}
	}
	biased, _ := modelcontrol.ApplyDemotionBias(candidates, panel.Metrics, "code")
	if len(biased) == 0 {
		biased = candidates
	}
	chosen := biased[0]
	now := panel.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	actual, err := fileDigest(chosen.CLI.Path)
	if err != nil {
		return modelcontrol.HostProfile{}, err
	}
	breaker := panel.Breaker
	if breaker == nil {
		breaker = &modelcontrol.CircuitBreaker{}
	}
	_, invokeErr := modelcontrol.GuardedInvoke(ctx, chosen, actual, panel.Verifier, breaker, now, func(context.Context, modelcontrol.HostProfile) (modelcontrol.InvocationResult, error) {
		return modelcontrol.InvocationResult{Success: true}, nil
	})
	emitCost(chosen, invokeErr)
	if invokeErr != nil {
		return modelcontrol.HostProfile{}, invokeErr
	}
	return chosen, nil
}

func fileDigest(path string) (string, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- path is the routed HostProfile CLI absolute path
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func emitCost(profile modelcontrol.HostProfile, invokeErr error) {
	status := "ok"
	if invokeErr != nil {
		status = "failed"
	}
	id := strings.TrimSpace(profile.ID)
	if id == "" {
		id = "profile"
	}
	event, err := telemetry.NewEvent(telemetry.EventInput{
		Timestamp: time.Now().UTC(), ProjectID: "prj_unknown", TraceID: telemetry.NewTraceID(),
		Module: "modelcontrol", Stage: "cost", Event: telemetry.EventCostTrace,
		Tool: id, Status: status, Measurement: telemetry.NoData, Duration: time.Millisecond,
	})
	if err != nil {
		return
	}
	telemetry.EmitBestEffort(event)
}
