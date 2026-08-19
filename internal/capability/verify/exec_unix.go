//go:build unix

package verify

import (
	"os/exec"
	"syscall"
	"time"
)

// Creating the group and killing it come from the same place, on purpose. In
// the previous product the pgid was derived from the child's pid, which was
// only true because every caller remembered to create the group first — a
// forgotten call site raised no error, killed nothing, and leaked the process.
func configureProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return killGroup(command) }
	command.WaitDelay = time.Second
}

// stopProcessGroup is the second half: after Wait returns, make sure nothing
// survives in the group. SIGKILL on a group is not atomic with its members
// leaving, so the work is only done when the group is gone.
func stopProcessGroup(command *exec.Cmd) {
	if command.Process == nil {
		return
	}
	pgid := command.Process.Pid
	if !groupExists(pgid) {
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	if waitForGroupToLeave(pgid, 250*time.Millisecond) {
		return
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !groupExists(pgid) {
			return
		}
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		time.Sleep(10 * time.Millisecond)
	}
}

func killGroup(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
}

func waitForGroupToLeave(pgid int, window time.Duration) bool {
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		if !groupExists(pgid) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return !groupExists(pgid)
}

func groupExists(pgid int) bool {
	return syscall.Kill(-pgid, 0) == nil
}
