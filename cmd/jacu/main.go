package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/capability/missioncompile"
	storagecap "github.com/jacu-dev/jacu-harness/internal/capability/storage"
	"github.com/jacu-dev/jacu-harness/internal/gitx"
	"github.com/jacu-dev/jacu-harness/internal/mcpadapter"
	"github.com/jacu-dev/jacu-harness/internal/project"
	headlessreport "github.com/jacu-dev/jacu-harness/internal/report"
	"github.com/jacu-dev/jacu-harness/internal/runner"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
	"github.com/jacu-dev/jacu-harness/internal/telemetry"
	"github.com/jacu-dev/jacu-harness/internal/userstate"
)

// Version is set via -ldflags at release; dev builds report "dev".
var Version = "dev"

// usage goes to stderr because stdout belongs to the protocol: a host that
// starts the binary with the wrong argv must not find prose on the wire.
func usage(args []string) {
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "jacu: unknown command %q\n", args[0])
	}
	fmt.Fprintln(os.Stderr, "usage: jacu <command>")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "  serve      speak MCP over stdio; this is what an MCP host runs")
	fmt.Fprintln(os.Stderr, "  inspect    inspect the open project without opening a workspace")
	fmt.Fprintln(os.Stderr, "  compile    compile a mission from JSON input")
	fmt.Fprintln(os.Stderr, "  workspace  open, status, diff, apply, or discard a run")
	fmt.Fprintln(os.Stderr, "  memory     save or recall project memory")
	fmt.Fprintln(os.Stderr, "  verify     run the mission's verification commands")
	fmt.Fprintln(os.Stderr, "  flow       execute a compiled orchestration graph")
	fmt.Fprintln(os.Stderr, "  doctor     report versions, or emit a host pack with --emit <host> [--repo PATH]")
	fmt.Fprintln(os.Stderr, "  init       install skills and emit/apply a host pack into named paths")
	fmt.Fprintln(os.Stderr, "  report     project structured workspace state as deterministic Markdown")
	fmt.Fprintln(os.Stderr, "  stats      print local telemetry metrics")
	fmt.Fprintln(os.Stderr, "  status     list every run still holding a worktree, across all projects")
	fmt.Fprintln(os.Stderr, "  statusline print one honest active-run status line")
	fmt.Fprintln(os.Stderr, "  run        execute one persisted run through a headless provider")
	fmt.Fprintln(os.Stderr, "  preflight  check compiled mission environment before dispatch")
	fmt.Fprintln(os.Stderr, "  provenance scan files and history for authorship traces")
	fmt.Fprintln(os.Stderr, "  storage    inspect or dry-run cleanup of JACU-owned local storage")
	fmt.Fprintln(os.Stderr, "  version    print the build version")
	fmt.Fprintln(os.Stderr, "  sdd        author, lint, and inspect native SDD documents")
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "help":
			usage(nil)
			return
		case "version":
			fmt.Println("jacu", Version)
			return
		case "sdd":
			root, err := projectRoot()
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve project root failed:", err)
				os.Exit(1)
			}
			os.Exit(runSDD(root, os.Args[2:], os.Stdout, os.Stderr))
		case "serve":
			logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
			root, err := projectRoot()
			if err != nil {
				logger.Error("resolve project root failed", "error", err)
				os.Exit(1)
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := mcpadapter.RunStdio(ctx, Version, root); err != nil && ctx.Err() == nil {
				logger.Error("serve failed", "error", err)
				os.Exit(1)
			}
			return
		case "inspect":
			root, err := projectRoot()
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve project root failed:", err)
				os.Exit(1)
			}
			os.Exit(runInspect(root, os.Args[2:], os.Stdout, os.Stderr))
		case "compile":
			root, err := projectRoot()
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve project root failed:", err)
				os.Exit(1)
			}
			os.Exit(runCompile(root, os.Args[2:], os.Stdout, os.Stderr))
		case "workspace":
			root, err := projectRoot()
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve project root failed:", err)
				os.Exit(1)
			}
			os.Exit(runWorkspace(root, os.Args[2:], os.Stdout, os.Stderr))
		case "memory":
			root, err := projectRoot()
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve project root failed:", err)
				os.Exit(1)
			}
			os.Exit(runMemory(root, os.Args[2:], os.Stdout, os.Stderr))
		case "verify":
			root, err := projectRoot()
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve project root failed:", err)
				os.Exit(1)
			}
			os.Exit(runVerify(root, os.Args[2:], os.Stdout, os.Stderr))
		case "flow":
			root, err := projectRoot()
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve project root failed:", err)
				os.Exit(1)
			}
			os.Exit(runFlow(root, os.Args[2:], os.Stdout, os.Stderr))
		case "report":
			root, err := projectRoot()
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve project root failed:", err)
				os.Exit(1)
			}
			os.Exit(runReport(root, os.Args[2:], os.Stdout, os.Stderr))
		case "status":
			// No projectRoot(): this command is deliberately global. Requiring
			// a repository would reproduce the blindness it exists to fix.
			os.Exit(runStatus(os.Args[2:], os.Stdout, os.Stderr, storagecap.StatusOptions{Now: time.Now().UTC()}))
		case "statusline":
			root, err := projectRoot()
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve project root failed:", err)
				os.Exit(1)
			}
			line, err := headlessreport.Statusline(root)
			if err != nil {
				fmt.Fprintln(os.Stderr, "build statusline failed:", err)
				os.Exit(1)
			}
			fmt.Println(line)
			return
		case "stats":
			root, err := projectRoot()
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve project root failed:", err)
				os.Exit(1)
			}
			duration, full, err := parseStatsArgs(os.Args[2:])
			if err != nil {
				fmt.Fprintln(os.Stderr, "stats:", err)
				os.Exit(2)
			}
			now := time.Now().UTC()
			events, err := telemetry.NewStore().ReadSince(now.Add(-duration))
			if err != nil {
				fmt.Fprintln(os.Stderr, "read telemetry failed:", err)
				os.Exit(1)
			}
			events = telemetry.FilterProject(events, telemetry.ProjectID(root))
			stats, err := telemetry.ComputeStats(events, now.Add(-duration), now, &telemetry.GitHistory{Repo: root})
			if err != nil {
				fmt.Fprintln(os.Stderr, "compute telemetry failed:", err)
				os.Exit(1)
			}
			if full {
				fmt.Print(telemetry.FormatFullStats(stats))
			} else {
				fmt.Print(telemetry.FormatStats(stats))
			}
			return
		case "run":
			root, err := projectRoot()
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve project root failed:", err)
				os.Exit(1)
			}
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if err := runHeadless(ctx, root, os.Args[2:], os.Stdout); err != nil {
				fmt.Fprintln(os.Stderr, "headless run failed:", err)
				os.Exit(1)
			}
			return
		case "preflight":
			root, err := projectRoot()
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve project root failed:", err)
				os.Exit(1)
			}
			os.Exit(runPreflight(root, os.Args[2:], os.Stdout, os.Stderr))
		case "provenance":
			root, err := projectRoot()
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve project root failed:", err)
				os.Exit(1)
			}
			os.Exit(runProvenance(root, os.Args[2:], os.Stdout, os.Stderr))
		case "storage":
			root, err := projectRoot()
			if err != nil {
				fmt.Fprintln(os.Stderr, "resolve project root failed:", err)
				os.Exit(1)
			}
			os.Exit(runStorage(root, os.Args[2:], os.Stdout, os.Stderr))
		case "init":
			os.Exit(runInit(os.Args[2:], os.Stdout, os.Stderr))
		case "doctor":
			if len(os.Args) > 2 {
				host, repo, err := parseDoctorEmit(os.Args[2:])
				if err != nil {
					fmt.Fprintln(os.Stderr, "doctor:", err)
					fmt.Fprintln(os.Stderr, "doctor: usage is doctor [--emit <host> [--repo PATH]]")
					os.Exit(1)
				}
				pack, err := renderHostPack(host, repo)
				if err != nil {
					fmt.Fprintln(os.Stderr, "doctor:", err)
					os.Exit(1)
				}
				fmt.Print(pack)
				return
			}
			fmt.Print(doctorReport())
			return
		}
	}
	usage(os.Args[1:])
	os.Exit(1)
}

func parseDoctorEmit(args []string) (host, repo string, err error) {
	if len(args) < 2 || args[0] != "--emit" {
		return "", "", fmt.Errorf("usage is doctor [--emit <host> [--repo PATH]]")
	}
	host = args[1]
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--repo":
			i++
			if i >= len(args) {
				return "", "", fmt.Errorf("--repo requires a path")
			}
			repo = args[i]
		default:
			return "", "", fmt.Errorf("unknown flag %s", args[i])
		}
	}
	return host, repo, nil
}

func parseSinceArgs(args []string) (time.Duration, error) {
	duration, _, err := parseStatsArgs(args)
	return duration, err
}

func parseStatsArgs(args []string) (time.Duration, bool, error) {
	duration := 30 * 24 * time.Hour
	full := false
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--full":
			full = true
		case "--since":
			index++
			durationValue, ok := statsArg(args, index)
			if !ok {
				return 0, false, fmt.Errorf("usage is stats [--full] [--since 30d]")
			}
			var err error
			duration, err = parseStatsDuration(durationValue)
			if err != nil {
				return 0, false, err
			}
		default:
			return 0, false, fmt.Errorf("usage is stats [--full] [--since 30d]")
		}
	}
	return duration, full, nil
}

func statsArg(args []string, index int) (string, bool) {
	if index < 0 || index >= len(args) {
		return "", false
	}
	return args[index], true
}

func parseStatsDuration(value string) (time.Duration, error) {
	if strings.HasSuffix(value, "d") {
		var days int
		if _, err := fmt.Sscanf(strings.TrimSuffix(value, "d"), "%d", &days); err != nil || days <= 0 {
			return 0, fmt.Errorf("invalid --since duration %q", value)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("invalid --since duration %q", value)
	}
	return duration, nil
}

func runHeadless(ctx context.Context, root string, args []string, output interface{ Write([]byte) (int, error) }) error {
	runID, providerName, model, err := parseRunArgs(args)
	if err != nil {
		return err
	}
	provider := runner.Provider(providerName)
	if provider != runner.ProviderClaude && provider != runner.ProviderCodex {
		return fmt.Errorf("provider %q is not allowlisted", providerName)
	}
	run, err := runstate.Load(root, runID)
	if err != nil {
		return fmt.Errorf("run is unavailable")
	}
	if identityErr := validateHeadlessRunIdentity(root, run); identityErr != nil {
		return fmt.Errorf("run is not valid")
	}
	if run.Status != runstate.StatusOpen && run.Status != runstate.StatusReviewed {
		return fmt.Errorf("run is not open for headless execution")
	}
	if !missioncompile.PlanReady(run.Mission.Program) {
		return fmt.Errorf("plan has open decisions")
	}
	result := runner.Run(ctx, runner.Request{
		ProjectID: telemetry.ProjectID(root),
		Provider:  provider,
		Worktree:  run.Worktree,
		Objective: run.Mission.Objective,
		Model:     model,
	})
	encoded, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode headless result")
	}
	if _, err := output.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("write headless result")
	}
	if result.Status != runner.StatusCompleted {
		return fmt.Errorf("provider did not complete")
	}
	return nil
}

func parseRunArgs(args []string) (runID, provider, model string, err error) {
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--run-id":
			if index+1 >= len(args) {
				return "", "", "", fmt.Errorf("--run-id requires a value")
			}
			runID = args[index+1]
			index++
		case "--provider":
			if index+1 >= len(args) {
				return "", "", "", fmt.Errorf("--provider requires a value")
			}
			provider = args[index+1]
			index++
		case "--model":
			if index+1 >= len(args) {
				return "", "", "", fmt.Errorf("--model requires a value")
			}
			model = args[index+1]
			index++
		default:
			return "", "", "", fmt.Errorf("unknown run option")
		}
	}
	if !runstate.ValidRunID(runID) {
		return "", "", "", fmt.Errorf("--run-id is required")
	}
	if provider == "" {
		return "", "", "", fmt.Errorf("--provider is required")
	}
	return runID, provider, model, nil
}

func validateHeadlessRunIdentity(root string, run runstate.Run) error {
	if !runstate.ValidRunID(run.RunID) || run.Branch != "jacu/run-"+strings.TrimPrefix(run.RunID, "run_") || run.BaseSHA == "" {
		return fmt.Errorf("invalid run identity")
	}
	projectID, err := project.ID(root)
	if err != nil {
		return err
	}
	stateDir, err := userstate.Dir()
	if err != nil {
		return err
	}
	expected := filepath.Join(stateDir, "worktrees", projectID, run.RunID)
	actual, err := filepath.Abs(run.Worktree)
	if err != nil || filepath.Clean(actual) != filepath.Clean(expected) {
		return fmt.Errorf("invalid run worktree")
	}
	info, err := os.Stat(actual)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("run worktree is unavailable")
	}
	return nil
}

func projectRoot() (string, error) {
	root, err := filepath.Abs(".")
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	return gitx.OwningRepository(root), nil
}
