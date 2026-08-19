package userstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestMigrate(t *testing.T) {
	tests := []struct {
		name          string
		currentName   string
		legacyName    string
		setup         func(t *testing.T, home, currentPath, legacyPath string)
		wantMoved     bool
		wantSkipped   string
		wantError     string
		wantCurrent   bool
		wantLegacy    bool
		wantFromAndTo bool
	}{
		{
			name:        "no legacy",
			currentName: "new-state",
			legacyName:  "",
			wantSkipped: "no-legacy",
			wantLegacy:  true,
		},
		{
			name:        "current directory already exists",
			currentName: "new-state",
			legacyName:  "old-state",
			setup: func(t *testing.T, home, currentPath, legacyPath string) {
				t.Helper()
				mustMkdir(t, currentPath)
			},
			wantCurrent: true,
		},
		{
			name:        "both directories fail closed",
			currentName: "new-state",
			legacyName:  "old-state",
			setup: func(t *testing.T, home, currentPath, legacyPath string) {
				t.Helper()
				mustMkdir(t, currentPath)
				mustMkdir(t, legacyPath)
			},
			wantError:   "new-state",
			wantCurrent: true,
			wantLegacy:  true,
		},
		{
			name:        "neither directory exists",
			currentName: "new-state",
			legacyName:  "old-state",
		},
		{
			name:        "legacy directory moves",
			currentName: "new-state",
			legacyName:  "old-state",
			setup: func(t *testing.T, home, currentPath, legacyPath string) {
				t.Helper()
				mustMkdir(t, legacyPath)
				mustWriteFile(t, filepath.Join(legacyPath, "state.json"))
			},
			wantMoved:     true,
			wantCurrent:   true,
			wantFromAndTo: true,
		},
		{
			name:        "current symlink refused",
			currentName: "new-state",
			legacyName:  "old-state",
			setup: func(t *testing.T, home, currentPath, legacyPath string) {
				t.Helper()
				mustSymlink(t, legacyPath, currentPath)
			},
			wantError: "new-state",
		},
		{
			name:        "legacy symlink refused",
			currentName: "new-state",
			legacyName:  "old-state",
			setup: func(t *testing.T, home, currentPath, legacyPath string) {
				t.Helper()
				mustSymlink(t, currentPath, legacyPath)
			},
			wantError:  "old-state",
			wantLegacy: true,
		},
		{
			name:        "current non-directory refused",
			currentName: "new-state",
			legacyName:  "old-state",
			setup: func(t *testing.T, home, currentPath, legacyPath string) {
				t.Helper()
				mustWriteFile(t, currentPath)
			},
			wantError: "new-state",
		},
		{
			name:        "legacy non-directory refused",
			currentName: "new-state",
			legacyName:  "old-state",
			setup: func(t *testing.T, home, currentPath, legacyPath string) {
				t.Helper()
				mustWriteFile(t, legacyPath)
			},
			wantError:  "old-state",
			wantLegacy: true,
		},
		{
			name:        "rename failure does not copy",
			currentName: filepath.Join("missing-parent", "new-state"),
			legacyName:  "old-state",
			setup: func(t *testing.T, home, currentPath, legacyPath string) {
				t.Helper()
				mustMkdir(t, legacyPath)
				mustWriteFile(t, filepath.Join(legacyPath, "state.json"))
			},
			wantError:  "migrate",
			wantLegacy: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			currentPath := filepath.Join(home, test.currentName)
			legacyPath := filepath.Join(home, test.legacyName)
			if test.setup != nil {
				test.setup(t, home, currentPath, legacyPath)
			}

			got, err := Migrate(home, test.currentName, test.legacyName)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("Migrate() error = %v, want substring %q", err, test.wantError)
				}
				if test.name == "both directories fail closed" && (!strings.Contains(err.Error(), currentPath) || !strings.Contains(err.Error(), legacyPath)) {
					t.Fatalf("Migrate() error = %v, want both paths %q and %q", err, currentPath, legacyPath)
				}
			} else if err != nil {
				t.Fatalf("Migrate() error = %v", err)
			}
			if got.Moved != test.wantMoved {
				t.Fatalf("Migrate() Moved = %t, want %t", got.Moved, test.wantMoved)
			}
			if got.Skipped != test.wantSkipped {
				t.Fatalf("Migrate() Skipped = %q, want %q", got.Skipped, test.wantSkipped)
			}
			if test.wantFromAndTo && (got.From != legacyPath || got.To != currentPath) {
				t.Fatalf("Migrate() paths = (%q, %q), want (%q, %q)", got.From, got.To, legacyPath, currentPath)
			}
			if exists, err := pathExistsAsDirectory(currentPath); err != nil {
				t.Fatal(err)
			} else if exists != test.wantCurrent {
				t.Fatalf("current directory exists = %t, want %t", exists, test.wantCurrent)
			}
			if exists, err := pathExists(legacyPath); err != nil {
				t.Fatal(err)
			} else if exists != test.wantLegacy {
				t.Fatalf("legacy path exists = %t, want %t", exists, test.wantLegacy)
			}
		})
	}
}

func TestMigrateRace(t *testing.T) {
	home := t.TempDir()
	currentName, legacyName := "new-state", "old-state"
	currentPath := filepath.Join(home, currentName)
	legacyPath := filepath.Join(home, legacyName)
	mustMkdir(t, legacyPath)

	const callers = 32
	results := make(chan error, callers)
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			_, err := Migrate(home, currentName, legacyName)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("Migrate() race error = %v", err)
		}
	}
	if exists, err := pathExistsAsDirectory(currentPath); err != nil {
		t.Fatal(err)
	} else if !exists {
		t.Fatal("race migration did not create current directory")
	}
	if exists, err := pathExists(legacyPath); err != nil {
		t.Fatal(err)
	} else if exists {
		t.Fatal("race migration left legacy directory")
	}
}

func TestDirDoesNotCacheHome(t *testing.T) {
	t.Setenv(HomeEnv, "")
	first := t.TempDir()
	second := t.TempDir()
	t.Setenv("HOME", first)
	gotFirst, err := Dir()
	if err != nil {
		t.Fatalf("Dir() first home: %v", err)
	}
	t.Setenv("HOME", second)
	gotSecond, err := Dir()
	if err != nil {
		t.Fatalf("Dir() second home: %v", err)
	}
	if gotFirst == gotSecond {
		t.Fatalf("Dir() cached %q across HOME change", gotFirst)
	}
	if gotFirst != DirIn(first) || gotSecond != DirIn(second) {
		t.Fatalf("Dir() = %q then %q, want %q then %q", gotFirst, gotSecond, DirIn(first), DirIn(second))
	}
}

func TestDirOrLocalHonorsHomeEnv(t *testing.T) {
	configured := t.TempDir()
	t.Setenv(HomeEnv, configured)
	if got := DirOrLocal(); got != configured {
		t.Fatalf("DirOrLocal() = %q, want %q", got, configured)
	}
}

func TestPathHelpers(t *testing.T) {
	home := filepath.Join("home", "user")
	if got, want := DirIn(home), filepath.Join(home, Name); got != want {
		t.Fatalf("DirIn() = %q, want %q", got, want)
	}
	if got, want := WorktreesDir(home), filepath.Join(home, Name, "worktrees"); got != want {
		t.Fatalf("WorktreesDir() = %q, want %q", got, want)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func mustWriteFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("state"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func pathExistsAsDirectory(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err == nil {
		return info.IsDir() && info.Mode()&os.ModeSymlink == 0, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
