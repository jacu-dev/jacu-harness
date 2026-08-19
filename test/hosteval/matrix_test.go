//go:build hosteval

package hosteval

import (
	"github.com/jacu-dev/jacu-harness/internal/testgit"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMatrix drives real host CLIs. It costs money and needs credentials, so it
// never runs by accident: JACU_HOSTEVAL=1 is required. Everything else in this
// package runs unattended and proves the judging logic without a host.
//
//	JACU_HOSTEVAL=1 go test -tags=hosteval ./test/hosteval/ -run TestMatrix -v
//	JACU_HOSTEVAL=1 JACU_HOSTEVAL_HOSTS=codex JACU_HOSTEVAL_CASES=4.1-inspect ...
func TestMatrix(t *testing.T) {
	if os.Getenv("JACU_HOSTEVAL") != "1" {
		t.Skip("set JACU_HOSTEVAL=1 to drive real hosts (costs money, needs credentials)")
	}

	dir, err := StreamDir()
	if err != nil {
		t.Fatalf("stream dir: %v", err)
	}

	workdir, projectID := cobaia(t)
	runner := Runner{StreamDir: dir, Workdir: workdir, ProjectID: projectID}

	hosts := selected(os.Getenv("JACU_HOSTEVAL_HOSTS"), hostNames())
	cases := selected(os.Getenv("JACU_HOSTEVAL_CASES"), caseIDs())

	var results []Result
	for _, hn := range hosts {
		h, ok := HostByName(hn)
		if !ok {
			t.Fatalf("unknown host %q; matrix is %v", hn, hostNames())
		}
		for _, c := range SRCases() {
			if !contains(cases, c.ID) {
				continue
			}
			res := runner.Run(t.Context(), h, c)
			results = append(results, res)
			t.Logf("[%s/%s] %s tools=%v %s", h.Name, c.ID, res.Verdict, res.Tools,
				strings.Join(append(res.Failures, res.Reason), " "))
		}
	}

	t.Logf("\n%s", Report(results))

	// Skipped is reported, never fatal. Fail is fatal: the whole point is that
	// a red routing case blocks, the way the manual matrix never did.
	var failed int
	for _, r := range results {
		if r.Verdict == Fail {
			failed++
		}
	}
	if failed > 0 {
		t.Fatalf("%d case(s) failed; see the table above", failed)
	}
}

// cobaia builds the throwaway Go project the routing cases act on: one package
// with a deliberate defect in Soma and a test that catches it. Reproducing the
// 2026-08-13 manual fixture matters — the recorded evidence is only comparable
// against the same subject.
func cobaia(t *testing.T) (workdir, projectID string) {
	t.Helper()
	dir := t.TempDir()

	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("go.mod", "module cobaia\n\ngo 1.26.0\n")
	write("soma.go", "package cobaia\n\n// Soma has a deliberate defect: it subtracts.\nfunc Soma(a, b int) int {\n\treturn a - b\n}\n")
	write("soma_test.go", "package cobaia\n\nimport \"testing\"\n\nfunc TestSoma(t *testing.T) {\n\tif got := Soma(2, 2); got != 4 {\n\t\tt.Fatalf(\"Soma(2,2) = %d, want 4\", got)\n\t}\n}\n")

	for _, args := range [][]string{{"init"}, {"add", "."}, {"-c", "user.email=hosteval@local", "-c", "user.name=hosteval", "commit", "-m", "cobaia"}} {
		cmd := exec.Command("git", args...) //nolint:gosec // fixed argv against a test-owned temp dir
		cmd.Dir = dir
		cmd.Env = testgit.Env()
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// jacu derives project_id from the repository. This harness must not
	// reimplement that derivation: encoding the hash here would drift the day
	// the algorithm changes, and a filter that silently matches nothing turns
	// every case into a false "no tools called" — which is a passing 4.4.
	//
	// So the id is supplied, never guessed. Set JACU_HOSTEVAL_PROJECT_ID from
	// `jacu_project_inspect` on this directory, or from the newest line of the
	// telemetry stream after one manual call.
	projectID = strings.TrimSpace(os.Getenv("JACU_HOSTEVAL_PROJECT_ID"))
	if projectID == "" {
		t.Skip("set JACU_HOSTEVAL_PROJECT_ID to the throwaway project's prj_ id " +
			"(run jacu_project_inspect in it once and read project_id from the envelope)")
	}
	return dir, projectID
}

func hostNames() []string {
	var out []string
	for _, h := range Hosts() {
		out = append(out, h.Name)
	}
	return out
}

func caseIDs() []string {
	var out []string
	for _, c := range SRCases() {
		out = append(out, c.ID)
	}
	return out
}

func selected(env string, all []string) []string {
	if strings.TrimSpace(env) == "" {
		return all
	}
	var out []string
	for _, p := range strings.Split(env, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
