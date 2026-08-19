//go:build unix

package verify

import (
	"os/exec"
	"strings"
	"syscall"
)

// processExists asks the OS, not the executor, whether a pid is still running —
// the timeout test has to be able to disagree with the code under test.
//
// kill(pid, 0) alone is not the question. A killed process stays in the table
// as a zombie until its parent reaps it, and when the parent died first that
// reaping is init's job, whenever it gets to it. kill(pid, 0) succeeds for a
// zombie, so the naive check reports a process that is very much dead as alive.
// The state is what settles it.
func processExists(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	output, err := exec.Command("ps", "-o", "stat=", "-p", itoa(pid)).Output()
	if err != nil {
		// No ps to ask: fall back to the weaker signal rather than inventing one.
		return true
	}
	state := strings.TrimSpace(string(output))
	return state != "" && !strings.HasPrefix(state, "Z")
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := []byte{}
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}
