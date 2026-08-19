//go:build unix

package runner

import (
	"os/exec"
	"syscall"
	"time"
)

func configureProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return terminateProcessGroup(command) }
	command.WaitDelay = time.Second
}

func terminateProcessGroup(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	pgid := command.Process.Pid
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	if waitForProcessGroup(pgid, terminationGrace) {
		return nil
	}
	return syscall.Kill(-pgid, syscall.SIGKILL)
}

func stopProcessGroup(command *exec.Cmd) {
	if command.Process == nil {
		return
	}
	_ = terminateProcessGroup(command)
}

func waitForProcessGroup(pgid int, window time.Duration) bool {
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		if syscall.Kill(-pgid, 0) != nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return syscall.Kill(-pgid, 0) != nil
}
