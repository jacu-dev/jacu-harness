//go:build !unix

package verify

import "os/exec"

// The executor is unix only. Setpgid and Kill(-pgid) have no direct equivalent
// elsewhere, and a timeout that cannot kill the whole group is a timeout that
// leaks processes. The design records Windows as out of scope; the tool answers
// blocked there rather than pretending.
func configureProcessGroup(command *exec.Cmd) {}

func stopProcessGroup(command *exec.Cmd) {}
