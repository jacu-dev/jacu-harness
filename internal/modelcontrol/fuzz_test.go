package modelcontrol

import (
	"testing"
)

func FuzzProfileRouteNeverPanics(f *testing.F) {
	f.Add("profile", "/bin/cli", "code.patch.v1", uint32(100), uint32(8000))
	f.Add("", "relative", "", uint32(0), uint32(0))
	f.Fuzz(func(t *testing.T, id, path, contract string, input, maxContext uint32) {
		profiles := []HostProfile{{ID: id, Lane: LaneCheap, Contracts: []string{contract}, MaxContextTokens: maxContext, Available: true, CLI: SignedCLI{Path: path, SHA256: testDigest, Signer: "host", Signature: "sig"}}}
		_, _, _ = Route(profiles, RouteRequest{Lane: LaneCheap, Contract: contract, InputTokens: input}, func(SignedCLI) bool { return true })
	})
}
