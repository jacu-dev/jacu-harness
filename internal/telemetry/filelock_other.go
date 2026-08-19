//go:build !darwin && !linux

package telemetry

import (
	"errors"
	"os"
)

func lockStoreFile(*os.File) error {
	return errors.New("telemetry file locking is unsupported on this platform")
}

func unlockStoreFile(*os.File) error { return nil }
