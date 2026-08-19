package orchestration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jacu-dev/jacu-harness/internal/gitx"
)

func TestFanInStagesNewFilesAndAppliesPatchesInDeterministicOrder(t *testing.T) {
	runner := &fanInRunner{patches: map[string]string{
		"agent-a": "patch-a",
		"agent-b": "patch-b",
	}}
	git := gitx.NewWithRunner("git", runner)
	applied, err := FanIn(context.Background(), git, "target", "base", []string{"agent-b", "agent-a"})
	if err != nil || !applied {
		t.Fatalf("fan-in applied=%v err=%v; want applied", applied, err)
	}
	if !strings.Contains(strings.Join(runner.calls, "|"), "agent-a:add|-A|agent-a:diff") {
		t.Fatalf("calls = %v; want stage and diff for agent-a", runner.calls)
	}
	if runner.applyCount != 2 || runner.resetCount != 0 {
		t.Fatalf("apply=%d reset=%d; want two applies and no reset", runner.applyCount, runner.resetCount)
	}
}

func TestFanInConflictResetsTargetAndReturnsEscalationValue(t *testing.T) {
	runner := &fanInRunner{patches: map[string]string{"agent": "patch"}, conflict: true}
	git := gitx.NewWithRunner("git", runner)
	applied, err := FanIn(context.Background(), git, "target", "base", []string{"agent"})
	if err != nil {
		t.Fatalf("fan-in conflict error = %v; want nil escalation", err)
	}
	if applied {
		t.Fatal("fan-in conflict applied; want false")
	}
	if runner.resetCount != 1 {
		t.Fatalf("reset count = %d; want one atomic reset", runner.resetCount)
	}
}

type fanInRunner struct {
	patches    map[string]string
	calls      []string
	applyCount int
	resetCount int
	conflict   bool
}

func (r *fanInRunner) Run(_ context.Context, _ string, repo string, _ []string, input string, args ...string) (string, string, error) {
	if len(args) == 0 {
		return "", "", errors.New("missing git args")
	}
	r.calls = append(r.calls, repo+":"+args[0]+"|"+strings.Join(args[1:], "|"))
	switch args[0] {
	case "add":
		return "", "", nil
	case "diff":
		return r.patches[repo], "", nil
	case "apply":
		r.applyCount++
		if r.conflict {
			return "", "Applied patch to 'file' with conflicts.", errors.New("git apply failed")
		}
		if input == "" {
			return "", "", errors.New("empty patch")
		}
		return "", "", nil
	case "reset":
		r.resetCount++
		return "", "", nil
	default:
		return "", "", nil
	}
}
