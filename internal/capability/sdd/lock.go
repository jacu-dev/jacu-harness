package sdd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Lock struct {
	SDDID         string              `json:"sdd_id"`
	ContentSHA256 string              `json:"content_sha256"`
	Requirements  []LockedRequirement `json:"requirements"`
	Tasks         []LockedTask        `json:"tasks"`
}

type LockedRequirement struct {
	Name  string `json:"name"`
	Delta string `json:"delta"`
}

type LockedTask struct {
	Number string `json:"number"`
	Verify string `json:"verify"`
	Status string `json:"status"`
}

func GenerateLock(document Document) Lock {
	lock := Lock{
		SDDID:         sddID(document),
		ContentSHA256: normalizedContentSHA(document),
		Requirements:  make([]LockedRequirement, 0, len(document.Requirements)),
		Tasks:         make([]LockedTask, 0),
	}
	for _, requirement := range normalizeDocument(document).Requirements {
		lock.Requirements = append(lock.Requirements, LockedRequirement{Name: requirement.Name, Delta: requirement.Delta})
	}
	for _, task := range Tasks(document) {
		lock.Tasks = append(lock.Tasks, LockedTask{Number: task.Number, Verify: task.Verify, Status: task.Status})
	}
	return lock
}

// WriteLockRoot replaces a lock through an already-opened filesystem root.
// documentDirectory is relative to root and never converted to an absolute
// path after the security boundary is established.
func WriteLockRoot(root *os.Root, documentDirectory string, document Document) error {
	if root == nil {
		return fmt.Errorf("lock root is nil")
	}
	encoded, err := json.MarshalIndent(GenerateLock(document), "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	temporary := filepath.Join(documentDirectory, fmt.Sprintf(".sdd.lock-%d", os.Getpid()))
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(temporary) }()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(encoded); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return root.Rename(temporary, filepath.Join(documentDirectory, "sdd.lock.json"))
}
