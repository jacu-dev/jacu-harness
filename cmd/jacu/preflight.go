package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/capability/preflight"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func runPreflight(root string, args []string, stdout, stderr *os.File) int {
	mission, overrides, jsonOutput, err := parsePreflightArgs(root, args)
	if err != nil {
		if _, printErr := fmt.Fprintln(stderr, "preflight:", err); printErr != nil {
			return 2
		}
		if _, printErr := fmt.Fprintln(stderr, "usage: jacu preflight [--json] [--command <program>] [--command-argv <json-array>] [--path <path>] [--required-path <path>] [--credential <name>] [--credential-present <name>] [--doc <path>] [--network-required] [--network-declared]"); printErr != nil {
			return 2
		}
		return 2
	}
	env := preflight.ResolveEnvironment(root, mission)
	env.Credentials = overrides.Credentials
	env.NetworkDeclared = overrides.NetworkDeclared
	env.RequiredNetwork = env.RequiredNetwork || overrides.RequiredNetwork
	report := preflight.Check(mission, env)
	preflight.EmitTelemetry(root, report)
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			if _, printErr := fmt.Fprintln(stderr, "preflight: encode report:", err); printErr != nil {
				return 2
			}
			return 2
		}
	} else {
		if _, printErr := fmt.Fprintln(stdout, report.Verdict); printErr != nil {
			return 2
		}
		for _, finding := range report.Findings {
			if _, printErr := fmt.Fprintln(stdout, finding.String()); printErr != nil {
				return 2
			}
		}
	}
	if report.Verdict == "pass" {
		return 0
	}
	return 1
}

func parsePreflightArgs(root string, args []string) (runstate.MissionSnapshot, preflight.Environment, bool, error) {
	mission := runstate.MissionSnapshot{AllowedPaths: []string{}, VerificationCommands: [][]string{}}
	env := preflight.Environment{Root: root, Credentials: map[string]bool{}, WritablePaths: map[string]bool{}}
	jsonOutput := false
	objective := []string{}
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			jsonOutput = true
		case "--command":
			value, next, err := preflightValue(args, index)
			if err != nil {
				return mission, env, jsonOutput, err
			}
			mission.VerificationCommands = append(mission.VerificationCommands, []string{value})
			index = next
		case "--command-argv":
			value, next, err := preflightValue(args, index)
			if err != nil {
				return mission, env, jsonOutput, err
			}
			var argv []string
			if err := json.Unmarshal([]byte(value), &argv); err != nil || len(argv) == 0 || argv[0] == "" {
				return mission, env, jsonOutput, fmt.Errorf("--command-argv requires a non-empty JSON argv array")
			}
			mission.VerificationCommands = append(mission.VerificationCommands, argv)
			index = next
		case "--path":
			value, next, err := preflightValue(args, index)
			if err != nil {
				return mission, env, jsonOutput, err
			}
			mission.AllowedPaths = append(mission.AllowedPaths, value)
			index = next
		case "--required-path":
			value, next, err := preflightValue(args, index)
			if err != nil {
				return mission, env, jsonOutput, err
			}
			objective = append(objective, "path:"+value)
			index = next
		case "--credential":
			value, next, err := preflightValue(args, index)
			if err != nil {
				return mission, env, jsonOutput, err
			}
			objective = append(objective, "credential:"+value)
			index = next
		case "--credential-present":
			value, next, err := preflightValue(args, index)
			if err != nil {
				return mission, env, jsonOutput, err
			}
			env.Credentials[value] = true
			index = next
		case "--doc":
			value, next, err := preflightValue(args, index)
			if err != nil {
				return mission, env, jsonOutput, err
			}
			objective = append(objective, "doc:"+value)
			index = next
		case "--network-required":
			env.RequiredNetwork = true
		case "--network-declared":
			env.NetworkDeclared = true
		default:
			return mission, env, jsonOutput, fmt.Errorf("unknown option %q", args[index])
		}
	}
	mission.Objective = strings.Join(objective, " ")
	return mission, env, jsonOutput, nil
}

func preflightValue(args []string, index int) (string, int, error) {
	if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") || args[index+1] == "" {
		return "", index, fmt.Errorf("%s requires a value", args[index])
	}
	return args[index+1], index + 1, nil
}
