package testgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestEnvReplacesAmbientGitConfig(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", "/hostile/global")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "0")
	values := Env()
	global, system := 0, 0
	for _, value := range values {
		if strings.HasPrefix(value, "GIT_CONFIG_GLOBAL=") {
			global++
			if value != "GIT_CONFIG_GLOBAL="+os.DevNull {
				t.Fatalf("global = %q", value)
			}
		}
		if strings.HasPrefix(value, "GIT_CONFIG_NOSYSTEM=") {
			system++
			if value != "GIT_CONFIG_NOSYSTEM=1" {
				t.Fatalf("system = %q", value)
			}
		}
	}
	if global != 1 || system != 1 {
		t.Fatalf("counts global=%d system=%d", global, system)
	}
}

func TestEnvRunsFixtureWithoutHostileGlobalConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell hook and signer fixture is Unix-specific")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git unavailable: %v", err)
	}

	root := t.TempDir()
	hostileConfig := filepath.Join(root, "hostile.gitconfig")
	hookDir := filepath.Join(root, "hostile-hooks")
	templateDir := filepath.Join(root, "hostile-template")
	hookSentinel := filepath.Join(root, "hook-ran")
	signerSentinel := filepath.Join(root, "signer-ran")
	repo := filepath.Join(root, "repo")

	if err := os.MkdirAll(hookDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(templateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(hookDir, "pre-commit"), hookSentinel)
	signer := filepath.Join(root, "hostile-signer")
	writeExecutable(t, signer, signerSentinel)
	if err := os.WriteFile(filepath.Join(templateDir, "hostile-template-marker"), []byte("copied\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	configureFile(t, hostileConfig, "commit.gpgsign", "true")
	configureFile(t, hostileConfig, "user.signingkey", "hostile-signing-key")
	configureFile(t, hostileConfig, "gpg.program", signer)
	configureFile(t, hostileConfig, "core.hooksPath", hookDir)
	configureFile(t, hostileConfig, "init.templateDir", templateDir)
	t.Setenv("GIT_CONFIG_GLOBAL", hostileConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "0")

	env := Env()
	assertConfigEnvOnce(t, env, "GIT_CONFIG_GLOBAL="+os.DevNull)
	assertConfigEnvOnce(t, env, "GIT_CONFIG_NOSYSTEM=1")

	runGit(t, env, root, "init", repo)
	if _, err := os.Stat(filepath.Join(repo, ".git", "hostile-template-marker")); !os.IsNotExist(err) {
		t.Fatalf("hostile init template was applied: %v", err)
	}

	runGit(t, env, repo, "config", "--local", "user.name", "Fixture User")
	runGit(t, env, repo, "config", "--local", "user.email", "fixture@example.invalid")
	if got := strings.TrimSpace(runGit(t, env, repo, "config", "--local", "--get", "user.name")); got != "Fixture User" {
		t.Fatalf("local user.name = %q", got)
	}
	if got := strings.TrimSpace(runGit(t, env, repo, "config", "--local", "--get", "user.email")); got != "fixture@example.invalid" {
		t.Fatalf("local user.email = %q", got)
	}

	if err := os.WriteFile(filepath.Join(repo, "fixture.txt"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, env, repo, "add", "fixture.txt")
	runGit(t, env, repo, "commit", "-m", "fixture commit")
	if got := strings.TrimSpace(runGit(t, env, repo, "log", "-1", "--format=%an <%ae>")); got != "Fixture User <fixture@example.invalid>" {
		t.Fatalf("commit identity = %q", got)
	}
	for _, sentinel := range []string{hookSentinel, signerSentinel} {
		if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
			t.Fatalf("hostile executable ran (%s): %v", sentinel, err)
		}
	}
}

func configureFile(t *testing.T, path, key, value string) {
	t.Helper()
	command := exec.Command("git", "config", "--file", path, key, value) // #nosec G204 -- fixed git binary with TempDir-owned fixture path and arguments.
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git config --file %s %s: %v\n%s", path, key, err, output)
	}
}

func writeExecutable(t *testing.T, path, sentinel string) {
	t.Helper()
	contents := "#!/bin/sh\nprintf ran > " + strconv.Quote(sentinel) + "\nexit 1\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o700); err != nil { // #nosec G302 -- test fixture must be executable to prove hostile hooks are not invoked.
		t.Fatal(err)
	}
}

func assertConfigEnvOnce(t *testing.T, env []string, want string) {
	t.Helper()
	count := 0
	key := strings.SplitN(want, "=", 2)[0] + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, key) {
			count++
			if entry != want {
				t.Fatalf("%s = %q, want %q", key, entry, want)
			}
		}
	}
	if count != 1 {
		t.Fatalf("%s count = %d, want 1", key, count)
	}
}

func runGit(t *testing.T, env []string, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...) // #nosec G204 -- fixed git binary with test-controlled arguments.
	command.Dir = dir
	command.Env = env
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
