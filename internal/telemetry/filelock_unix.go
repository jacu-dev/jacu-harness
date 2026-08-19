//go:build darwin || linux

package telemetry

import (
	"errors"
	"os"
	"syscall"
)

func lockStoreFile(file *os.File) error {
	for {
		// #nosec G115 -- file.Fd is the valid descriptor returned by os.File and syscall requires int.
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

func unlockStoreFile(file *os.File) error {
	// #nosec G115 -- see lockStoreFile.
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
