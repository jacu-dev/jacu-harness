package userstate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	Name    = ".jacu-harness"
	Legacy  = ".jacu-mcp"
	HomeEnv = "JACU_HOME"
)

type Outcome struct {
	Moved             bool
	From, To, Skipped string
}

func Dir() (string, error) {
	if configured := os.Getenv(HomeEnv); configured != "" {
		return configured, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if _, err := EnsureMigrated(home); err != nil {
		return "", err
	}
	return DirIn(home), nil
}

func DirOrLocal() string {
	if configured := os.Getenv(HomeEnv); configured != "" {
		return configured
	}
	if dir, err := Dir(); err == nil && dir != "" {
		return dir
	}
	return filepath.Join(".", Name)
}

func DirIn(home string) string {
	return filepath.Join(home, Name)
}

func WorktreesDir(home string) string {
	return filepath.Join(DirIn(home), "worktrees")
}

func EnsureMigrated(home string) (Outcome, error) {
	return Migrate(home, Name, Legacy)
}

func Migrate(home, currentName, legacyName string) (Outcome, error) {
	if legacyName == "" {
		return Outcome{Skipped: "no-legacy"}, nil
	}

	currentPath := filepath.Join(home, currentName)
	legacyPath := filepath.Join(home, legacyName)
	current, err := inspectDirectory(currentPath)
	if err != nil {
		return Outcome{}, err
	}
	legacy, err := inspectDirectory(legacyPath)
	if err != nil {
		return Outcome{}, err
	}
	if current && legacy {
		return Outcome{}, fmt.Errorf("userstate: both current and legacy directories exist: %q and %q", currentPath, legacyPath)
	}
	if current || !legacy {
		return Outcome{}, nil
	}

	if err := os.Rename(legacyPath, currentPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			current, currentErr := inspectDirectory(currentPath)
			if currentErr != nil {
				return Outcome{}, fmt.Errorf("userstate: migrate %q to %q: %w", legacyPath, currentPath, currentErr)
			}
			if current {
				return Outcome{To: currentPath}, nil
			}
		}
		return Outcome{}, fmt.Errorf("userstate: migrate %q to %q: %w", legacyPath, currentPath, err)
	}

	fmt.Fprintf(os.Stderr, "userstate: migrated %s -> %s\n", legacyPath, currentPath)
	return Outcome{Moved: true, From: legacyPath, To: currentPath}, nil
}

func inspectDirectory(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("userstate: inspect %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, fmt.Errorf("userstate: refuse %q: expected a non-symlink directory", path)
	}
	return true, nil
}
