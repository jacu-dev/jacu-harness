//go:build !unix

package runner

import "os/exec"

func configureProcessGroup(*exec.Cmd) {}

func stopProcessGroup(*exec.Cmd) {}
