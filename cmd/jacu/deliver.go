package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/capability/workspace"
	"github.com/jacu-dev/jacu-harness/internal/gitx"
)

var sddDeliverBranch = regexp.MustCompile(`^sdd/[0-9]{3}$`)

const deliverUsage = "usage: jacu deliver [--base main] [--title text] [--json]"

type deliverResult struct {
	Branch string `json:"branch"`
	Base   string `json:"base"`
	URL    string `json:"url"`
	Title  string `json:"title"`
}

var deliverGitNew = gitx.New

var deliverPush = func(ctx context.Context, git *gitx.Git, root, branch string) error {
	return git.Exec(ctx, root, "push", "--set-upstream", "origin", branch)
}

var deliverCreatePR = func(ctx context.Context, argv []string) (string, error) {
	// #nosec G204 -- argv is a fixed gh invocation built by runDeliver; no shell.
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// errDeliverPrecondition marks the refusals that happen before anything leaves
// the machine: wrong branch or a dirty tree. They exit 2, like a usage error,
// and never push.
var errDeliverPrecondition = errors.New("deliver precondition")

// wireAutonomyDeliver gives the workspace package the one function allowed to
// push. Without it a program with `deliver_at_end` merges every mission
// locally and then ends with "deliver is not wired", so main calls it before
// dispatching any subcommand — `serve` included.
func wireAutonomyDeliver() {
	workspace.SetAutonomyDeliver(deliverForAutonomy)
}

func deliverForAutonomy(ctx context.Context, root string) error {
	_, err := deliver(ctx, root, "main", "")
	return err
}

func deliver(ctx context.Context, root, base, title string) (deliverResult, error) {
	git, err := deliverGitNew()
	if err != nil {
		return deliverResult{}, err
	}
	branch, err := git.CurrentBranch(ctx, root)
	if err != nil || !sddDeliverBranch.MatchString(branch) {
		return deliverResult{}, fmt.Errorf("%w: checkout is not an sdd/<NNN> branch", errDeliverPrecondition)
	}
	clean, err := git.IsClean(ctx, root)
	if err != nil {
		return deliverResult{}, err
	}
	if !clean {
		return deliverResult{}, fmt.Errorf("%w: checkout has uncommitted changes", errDeliverPrecondition)
	}
	if title == "" {
		title = branch
	}
	if pushErr := deliverPush(ctx, git, root, branch); pushErr != nil {
		return deliverResult{}, fmt.Errorf("push failed: %w", pushErr)
	}
	argv := []string{"gh", "pr", "create", "--base", base, "--head", branch, "--title", title}
	url, err := deliverCreatePR(ctx, argv)
	if err != nil {
		return deliverResult{}, fmt.Errorf("pull request create failed: %w", err)
	}
	return deliverResult{Branch: branch, Base: base, URL: url, Title: title}, nil
}

func runDeliver(root string, args []string, stdout, stderr io.Writer) int {
	base, title, jsonOutput := "main", "", false
	for index := 0; index < len(args); index++ {
		option := args[index]
		if option == "--json" {
			jsonOutput = true
			continue
		}
		if option != "--base" && option != "--title" {
			_, _ = fmt.Fprintf(stderr, "deliver: unknown option %s\n", option)
			_, _ = fmt.Fprintln(stderr, deliverUsage)
			return 2
		}
		// Slicing past the switched-on element keeps the bound obvious to the
		// reader and to gosec, which reads `args[index+1]` as unproven.
		remaining := args[index+1:]
		if len(remaining) == 0 {
			_, _ = fmt.Fprintf(stderr, "deliver: %s requires a value\n", option)
			_, _ = fmt.Fprintln(stderr, deliverUsage)
			return 2
		}
		if option == "--base" {
			base = remaining[0]
		} else {
			title = remaining[0]
		}
		index++
	}
	result, err := deliver(context.Background(), root, base, title)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "deliver:", err)
		if errors.Is(err, errDeliverPrecondition) {
			return 2
		}
		return 1
	}
	if jsonOutput {
		if encodeErr := json.NewEncoder(stdout).Encode(result); encodeErr != nil {
			return 1
		}
		return 0
	}
	_, _ = fmt.Fprintln(stdout, result.URL)
	return 0
}
