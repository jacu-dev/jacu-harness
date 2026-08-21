package orchestration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/modelcontrol"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
)

func TestValidateRejectsNamedCLIAndUnknownLane(t *testing.T) {
	named := Validate(Flow{Nodes: []Node{{ID: "a", Uses: UseVerify, Lane: "cheap", With: map[string]any{"cli": "/usr/bin/claude"}}}})
	if !named.Blocked() {
		t.Fatal("named CLI was accepted")
	}
	unknown := Validate(Flow{Nodes: []Node{{ID: "a", Uses: UseVerify, Lane: "gemini"}}})
	if !unknown.Blocked() {
		t.Fatal("unknown lane was accepted")
	}
}

func TestExecuteRoutesLaneThroughModelControl(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "cli")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("#!/bin/sh\nexit 0\n"))
	digest := "sha256:" + hex.EncodeToString(sum[:])
	t.Setenv("JACU_HOME", t.TempDir())
	SetPanel(Panel{
		Profiles: []modelcontrol.HostProfile{{
			ID: "cheap", Lane: modelcontrol.LaneCheap, Rank: 1, Capability: 40,
			Contracts: []string{"code.patch.v1"}, MaxContextTokens: 8000, Available: true,
			CLI: modelcontrol.SignedCLI{Path: binary, SHA256: digest, Signer: "host", Signature: "sig"},
		}},
		Verifier: func(modelcontrol.SignedCLI) bool { return true },
		Breaker:  modelcontrol.NewCircuitBreaker(3, 0),
	})
	t.Cleanup(func() { SetPanel(Panel{}) })
	called := 0
	result, err := Execute(context.Background(), Flow{Nodes: []Node{{ID: "a", Uses: UseVerify, Lane: "cheap"}}}, nil, func(context.Context, Node, map[string]NodeResult) (NodeResult, error) {
		called++
		return NodeResult{Status: "ok", Verdict: "pass"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "ok" || called != 1 {
		t.Fatalf("result=%#v called=%d", result, called)
	}
	events, err := telemetry.NewStore().ReadSince(time.Unix(0, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.Event == telemetry.EventCostTrace {
			found = true
			if event.Tool != "cheap" {
				t.Fatalf("cost tool = %q", event.Tool)
			}
		}
	}
	if !found {
		t.Fatalf("cost.trace missing: %+v", events)
	}
}

func TestDemotionKeepsACandidate(t *testing.T) {
	kept, _ := modelcontrol.ApplyDemotionBias(
		[]modelcontrol.HostProfile{{ID: "down"}, {ID: "up"}},
		[]modelcontrol.ModelMetric{{ProfileID: "down", TaskKind: "code", TotalRuns: 12, AcceptedRuns: 0}},
		"code",
	)
	if len(kept) == 0 {
		t.Fatal("demotion left zero candidates")
	}
}
