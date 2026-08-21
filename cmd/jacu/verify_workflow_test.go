package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyWorkflowIsSelfContainedAndLintsProvenance(t *testing.T) {
	root := filepath.Join("..", "..")
	ci, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml")) // #nosec G304 -- fixed path to the vendored workflow in this repository
	if err != nil {
		t.Fatal(err)
	}
	verify, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "verify.yml")) // #nosec G304 -- fixed path to the vendored workflow in this repository
	if err != nil {
		t.Fatal(err)
	}
	ciText, verifyText := string(ci), string(verify)
	if !strings.Contains(ciText, "uses: ./.github/workflows/verify.yml") {
		t.Fatal("ci.yml must call the vendored verify workflow")
	}
	if !strings.Contains(verifyText, "workflow_call:") {
		t.Fatal("verify.yml must be reusable via workflow_call")
	}
	if strings.Contains(verifyText, "jacu-dev/jacu-dev-ci") {
		t.Fatal("verify.yml must not call the private reusable workflow")
	}
	if !strings.Contains(verifyText, "provenance-lint files") || !strings.Contains(verifyText, "commit-convention") {
		t.Fatal("verify.yml must run provenance-lint files and the commit-convention history step")
	}
	if !strings.Contains(verifyText, "GOLANGCI_VERSION: v2.11.4") {
		t.Fatal("verify.yml must pin golangci-lint v2.11.4")
	}
}
