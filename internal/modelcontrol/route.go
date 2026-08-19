package modelcontrol

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type SignedCLI struct {
	Path      string
	SHA256    string
	Signer    string
	Signature string
}

type HostProfile struct {
	ID               string
	Lane             Lane
	Rank             int
	Capability       int
	Contracts        []string
	MaxContextTokens uint32
	Available        bool
	CLI              SignedCLI
}

type RouteRequest struct {
	Lane               Lane
	Contract           string
	InputTokens        uint32
	OutputTokens       uint32
	RequiredCapability int
}

type Finding struct {
	ProfileID string
	Rule      string
}

type SignatureVerifier func(SignedCLI) bool

// ValidateSignedCLI validates the shape locally and delegates trust in the
// signature to the host-owned verifier. It intentionally does not implement a
// signing protocol or inspect credentials.
func ValidateSignedCLI(cli SignedCLI, verifier SignatureVerifier) error {
	if !filepath.IsAbs(cli.Path) {
		return errors.New("CLI path must be absolute")
	}
	if !validSHA256(cli.SHA256) {
		return errors.New("CLI SHA-256 digest is invalid")
	}
	if strings.TrimSpace(cli.Signer) == "" || strings.TrimSpace(cli.Signature) == "" {
		return errors.New("CLI signature attestation is incomplete")
	}
	if verifier == nil {
		return errors.New("CLI signature attestation verifier is required")
	}
	if !verifier(cli) {
		return errors.New("CLI signature attestation failed")
	}
	return nil
}

func Route(profiles []HostProfile, request RouteRequest, verifier SignatureVerifier) (HostProfile, []Finding, error) {
	if verifier == nil {
		return HostProfile{}, nil, errors.New("CLI signature attestation verifier is required")
	}
	findings := make([]Finding, 0)
	candidates := make([]HostProfile, 0, len(profiles))
	for _, profile := range profiles {
		if !profile.Available || profile.Lane != request.Lane || profile.Capability < request.RequiredCapability || !contains(profile.Contracts, request.Contract) {
			continue
		}
		if uint64(request.InputTokens)+uint64(request.OutputTokens) > uint64(profile.MaxContextTokens) {
			findings = append(findings, Finding{ProfileID: profile.ID, Rule: "context_overflow"})
			continue
		}
		if err := ValidateSignedCLI(profile.CLI, verifier); err != nil {
			findings = append(findings, Finding{ProfileID: profile.ID, Rule: "attestation"})
			continue
		}
		candidates = append(candidates, profile)
	}
	if len(candidates) == 0 {
		return HostProfile{}, findings, fmt.Errorf("no eligible host profile for lane %q and contract %q", request.Lane, request.Contract)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Rank != candidates[j].Rank {
			return candidates[i].Rank < candidates[j].Rank
		}
		return candidates[i].ID < candidates[j].ID
	})
	return candidates[0], findings, nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func validSHA256(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, char := range value[len("sha256:"):] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
