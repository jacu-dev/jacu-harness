//go:build !darwin && !linux

package memory

import (
	"errors"
	"os"
)

func lockStoreFile(*os.File) error {
	return errors.New("memory store file locking is unsupported on this platform")
}

func unlockStoreFile(*os.File) error {
	return nil
}
