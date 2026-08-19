//go:build darwin || linux

package runstate

import (
	"errors"
	"os"
	"syscall"
)

func lockRunstateFile(file *os.File) error {
	for {
		// #nosec G115 -- syscall.Flock accepts the valid descriptor returned by
		// os.File.Fd; conversion is required by the syscall API.
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

func unlockRunstateFile(file *os.File) error {
	// #nosec G115 -- see the matching LOCK_EX conversion above.
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
