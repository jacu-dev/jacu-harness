package gitx

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type fakeGitCommandRunner struct {
	stdout string
	stderr string
	err    error
	bin    string
	repo   string
	env    []string
	args   []string
}

func (runner *fakeGitCommandRunner) Run(_ context.Context, bin, repo string, env []string, _ string, args ...string) (string, string, error) {
	runner.bin, runner.repo, runner.env, runner.args = bin, repo, append([]string{}, env...), append([]string{}, args...)
	return runner.stdout, runner.stderr, runner.err
}

func TestGitCommandBoundaryIsInjectable(t *testing.T) {
	fake := &fakeGitCommandRunner{stdout: "deadbeef\n", stderr: "simulated stderr", err: errors.New("simulated git failure")}
	git := NewWithRunner("/usr/bin/git", fake)
	_, err := git.RevParseHead(context.Background(), "/repo")
	if err == nil || !strings.Contains(err.Error(), "simulated git failure") {
		t.Fatalf("RevParseHead error = %v; want injected failure", err)
	}
	if fake.repo != "/repo" || !reflect.DeepEqual(fake.args, []string{"rev-parse", "HEAD"}) {
		t.Fatalf("injected command = repo %q argv %#v", fake.repo, fake.args)
	}
	for _, entry := range fake.env {
		if strings.HasPrefix(entry, "GIT_") {
			t.Fatalf("injected command inherited Git selector %q", entry)
		}
	}
}
