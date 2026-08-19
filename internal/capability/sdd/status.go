package sdd

import (
	"encoding/json"
	"fmt"
)

// DerivedStatus is intentionally assembled from generated lock data and live
// repository state. It never trusts the hand-written task checkbox as status.
type DerivedStatus struct {
	TasksTotal   int
	TasksDone    int
	TasksDoing   int
	Blocks       int
	ChangedPaths int
}

// DeriveStatusWithLock derives status from bytes read through an already-opened
// root. It keeps CLI status free of absolute-path SDD reads after confinement.
func DeriveStatusWithLock(directory string, document, content []byte) (DerivedStatus, error) {
	var lock Lock
	if decodeErr := json.Unmarshal(content, &lock); decodeErr != nil {
		return DerivedStatus{}, fmt.Errorf("decode lock: %w", decodeErr)
	}
	status := DerivedStatus{TasksTotal: len(lock.Tasks)}
	for _, task := range lock.Tasks {
		switch task.Status {
		case "done":
			status.TasksDone++
		case "doing":
			status.TasksDoing++
		}
	}
	for _, finding := range LintSDDContentWithLock(directory, document, content) {
		if finding.Severity == SeverityBlock {
			status.Blocks++
		}
	}
	changedPaths, changedErr := changedPathsFor(directory)
	if changedErr != nil {
		return DerivedStatus{}, fmt.Errorf("read git state: %w", changedErr)
	}
	status.ChangedPaths = len(changedPaths)
	return status, nil
}
