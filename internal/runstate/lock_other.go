//go:build !darwin && !linux

package runstate

import (
	"errors"
	"os"
)

func lockRunstateFile(*os.File) error {
	return errors.New("runstate file locking is unsupported on this platform")
}

func unlockRunstateFile(*os.File) error { return nil }
