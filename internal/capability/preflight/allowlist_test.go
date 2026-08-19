package preflight

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAllowlistCheckMatchesVerifyPolicy(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".jacu"), 0o700); err != nil {
		t.Fatal(err)
	}
	policy := []byte(`{"allow":[{"program":"tool","required_arg_prefix":["check"]}]}`)
	if err := os.WriteFile(filepath.Join(root, ".jacu", "verify-allowlist.json"), policy, 0o600); err != nil {
		t.Fatal(err)
	}
	if !allowlistCheck(root, []string{"tool", "check", "input"}) {
		t.Fatal("preflight rejected a command accepted by verify policy")
	}
	if allowlistCheck(root, []string{"tool", "apply"}) {
		t.Fatal("preflight accepted a command rejected by verify policy")
	}
}

func TestRepositoryPolicyAuthorizesPreflightSubcommand(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	if !allowlistCheck(root, []string{"jacu", "preflight"}) {
		t.Fatal("repository policy does not authorize the preflight subcommand")
	}
}
