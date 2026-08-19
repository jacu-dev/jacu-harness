package preflight

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/capability/verify"
	"github.com/jacu-dev/jacu-harness/internal/runstate"
)

func ResolveEnvironment(root string, mission runstate.MissionSnapshot) Environment {
	pathNames := []string{}
	seen := map[string]struct{}{}
	pathDirs := configuredPathDirs(root)
	for _, argv := range mission.VerificationCommands {
		if len(argv) == 0 || strings.Contains(argv[0], ":") {
			continue
		}
		if executableOnPath(argv[0], pathDirs) {
			if _, exists := seen[argv[0]]; !exists {
				seen[argv[0]] = struct{}{}
				pathNames = append(pathNames, argv[0])
			}
		}
	}
	return Environment{Root: root, Path: pathNames, Credentials: map[string]bool{}}
}

func executableOnPath(program string, pathDirs []string) bool {
	for _, directory := range verify.SearchPath(pathDirs) {
		candidate := filepath.Join(directory, program)
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return true
		}
	}
	return false
}

func environmentFindings(mission runstate.MissionSnapshot, env Environment) []Finding {
	findings := []Finding{}
	add := func(class, target, detail string) {
		findings = append(findings, Finding{Class: class, Target: target, Detail: detail})
	}
	if env.RequiredNetwork && !env.NetworkDeclared {
		add(ClassNetworkUndeclared, "network", "network need was not declared")
	}
	for _, path := range mission.AllowedPaths {
		resolved := path
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(env.Root, resolved)
		}
		if _, err := os.Stat(resolved); err == nil && !pathWritable(env, path, resolved) {
			add(ClassPathNotWritable, path, "path is not writable")
		}
	}
	for _, token := range strings.Fields(mission.Objective) {
		if requiredPath, ok := strings.CutPrefix(token, "path:"); ok && requiredPath != "" {
			resolved := requiredPath
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(env.Root, resolved)
			}
			if _, err := os.Stat(resolved); err != nil {
				add(ClassPathMissing, requiredPath, "required path is absent")
			}
		}
		if credential, ok := strings.CutPrefix(token, "credential:"); ok && credential != "" && !env.Credentials[credential] {
			add(ClassCredentialAbsent, "credential", "credential presence is unresolved")
		}
		if doc, ok := strings.CutPrefix(token, "doc:"); ok && doc != "" {
			resolved := doc
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(env.Root, resolved)
			}
			if _, err := os.Stat(resolved); err != nil {
				add(ClassDocMissing, doc, "document is absent")
			}
		}
	}
	return findings
}

func pathWritable(env Environment, declared, resolved string) bool {
	if env.WritablePaths != nil {
		writable, ok := env.WritablePaths[declared]
		if ok {
			return writable
		}
	}
	info, err := os.Stat(resolved)
	return err == nil && info.Mode().Perm()&0o200 != 0
}
