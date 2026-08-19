package modelcontrol

import (
	"testing"
)

func TestRouteRequiresExternalSignedCLIAttestation(t *testing.T) {
	profiles := []HostProfile{{
		ID: "claude-cheap", Lane: LaneCheap, Rank: 1, Capability: 40,
		Contracts: []string{"code.patch.v1"}, MaxContextTokens: 8000,
		Available: true, CLI: SignedCLI{Path: "/usr/local/bin/claude", SHA256: testDigest, Signer: "host"},
	}}
	if _, findings, err := Route(profiles, RouteRequest{Lane: LaneCheap, Contract: "code.patch.v1", InputTokens: 100}, func(SignedCLI) bool { return false }); err == nil || len(findings) != 1 || findings[0].Rule != "attestation" {
		t.Fatalf("route error = %v findings=%#v; want attestation block", err, findings)
	}
	if _, _, err := Route(profiles, RouteRequest{Lane: LaneCheap, Contract: "code.patch.v1", InputTokens: 100}, nil); err == nil {
		t.Fatal("route accepted missing verifier")
	}
}

func TestRouteSelectsDeterministicVerifiedProfile(t *testing.T) {
	profiles := []HostProfile{
		{ID: "zeta", Lane: LaneCheap, Rank: 1, Capability: 40, Contracts: []string{"code.patch.v1"}, MaxContextTokens: 8000, Available: true, CLI: SignedCLI{Path: "/bin/zeta", SHA256: testDigest, Signer: "host", Signature: "sig"}},
		{ID: "alpha", Lane: LaneCheap, Rank: 1, Capability: 40, Contracts: []string{"code.patch.v1"}, MaxContextTokens: 8000, Available: true, CLI: SignedCLI{Path: "/bin/alpha", SHA256: testDigest, Signer: "host", Signature: "sig"}},
	}
	got, findings, err := Route(profiles, RouteRequest{Lane: LaneCheap, Contract: "code.patch.v1", InputTokens: 100}, func(SignedCLI) bool { return true })
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if got.ID != "alpha" || len(findings) != 0 {
		t.Fatalf("route = %#v findings=%#v; want alpha and no findings", got, findings)
	}
}

func TestRouteRejectsUnsafePathAndContextOverflow(t *testing.T) {
	profiles := []HostProfile{
		{ID: "relative", Lane: LaneCheap, Contracts: []string{"code.patch.v1"}, MaxContextTokens: 1000, Available: true, CLI: SignedCLI{Path: "claude", SHA256: testDigest, Signer: "host", Signature: "sig"}},
		{ID: "large", Lane: LaneCheap, Contracts: []string{"code.patch.v1"}, MaxContextTokens: 1000, Available: true, CLI: SignedCLI{Path: "/bin/claude", SHA256: testDigest, Signer: "host", Signature: "sig"}},
	}
	_, findings, err := Route(profiles, RouteRequest{Lane: LaneCheap, Contract: "code.patch.v1", InputTokens: 1001}, func(SignedCLI) bool { return true })
	if err == nil || len(findings) == 0 {
		t.Fatalf("route = err %v findings %#v; want fail-closed findings", err, findings)
	}
}

const testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
