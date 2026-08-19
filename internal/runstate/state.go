package runstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Status string

const CurrentSchemaVersion = "1"

const (
	StatusOpen      Status = "open"
	StatusReviewed  Status = "reviewed"
	StatusApplied   Status = "applied"
	StatusDiscarded Status = "discarded"
	StatusCorrupted Status = "corrupted"
)

type Run struct {
	SchemaVersion     string                `json:"schema_version"`
	RunID             string                `json:"run_id"`
	MissionID         string                `json:"mission_id"`
	MissionInput      MissionInput          `json:"mission_input"`
	Mission           MissionSnapshot       `json:"mission"`
	Status            Status                `json:"status"`
	CreatedAt         time.Time             `json:"created_at"`
	BaseSHA           string                `json:"base_sha"`
	Branch            string                `json:"branch"`
	Worktree          string                `json:"worktree"`
	ReviewedDigest    string                `json:"reviewed_digest,omitempty"`
	ReviewedAt        time.Time             `json:"reviewed_at,omitempty"`
	AppliedCommit     string                `json:"applied_commit,omitempty"`
	ArchivePatch      string                `json:"archive_patch,omitempty"`
	ArchiveDigest     string                `json:"archive_digest,omitempty"`
	ProgramID         string                `json:"program_id,omitempty"`
	ProgramMissionIDs []string              `json:"program_mission_ids,omitempty"`
	ProgramCursor     int                   `json:"program_cursor,omitempty"`
	ProgramEscalated  []int                 `json:"program_escalated,omitempty"`
	ProgramMissions   []ProgramMissionState `json:"program_missions,omitempty"`
	Audit             *AuditPackage         `json:"audit,omitempty"`
}

type ProgramMissionState struct {
	Index      int           `json:"index"`
	Status     string        `json:"status"`
	Iterations int           `json:"iterations"`
	Audit      *AuditPackage `json:"audit,omitempty"`
	Warnings   []string      `json:"warnings,omitempty"`
}

type AuditPackage struct {
	Objective      string               `json:"objective"`
	DiffDigest     string               `json:"diff_digest"`
	Verdict        string               `json:"verdict"`
	EvidenceDigest string               `json:"evidence_digest"`
	ReceiptRef     string               `json:"receipt_ref,omitempty"`
	Iterations     int                  `json:"iterations"`
	Warnings       []string             `json:"warnings"`
	CheckEvidence  []CheckEvidenceAudit `json:"check_evidence,omitempty"`
	Remediations   []RemediationAudit   `json:"remediations,omitempty"`
}

type CheckEvidenceAudit struct {
	Check          string   `json:"check"`
	Status         string   `json:"status"`
	Repository     string   `json:"repository,omitempty"`
	RunID          string   `json:"run_id,omitempty"`
	JobID          string   `json:"job_id,omitempty"`
	LogDigest      string   `json:"log_digest,omitempty"`
	LogTruncated   bool     `json:"log_truncated,omitempty"`
	AnnotationPath []string `json:"annotation_paths,omitempty"`
	EvidenceDigest string   `json:"evidence_digest"`
}

type RemediationAudit struct {
	Action           string   `json:"action"`
	Check            string   `json:"check"`
	Class            string   `json:"class"`
	Objective        string   `json:"objective,omitempty"`
	AllowedPaths     []string `json:"allowed_paths,omitempty"`
	BudgetIterations int      `json:"budget_iterations"`
	Reason           string   `json:"reason"`
}

func (r *Run) Transition(next Status) error {
	if r.Status == next {
		return nil
	}
	if next == StatusCorrupted {
		r.Status = next
		return nil
	}
	valid := (r.Status == StatusOpen && (next == StatusReviewed || next == StatusDiscarded)) ||
		(r.Status == StatusReviewed && (next == StatusApplied || next == StatusDiscarded)) ||
		(r.Status == StatusCorrupted && next == StatusDiscarded)
	if !valid {
		return fmt.Errorf("invalid run transition %q to %q", r.Status, next)
	}
	r.Status = next
	return nil
}

func Save(repo string, run Run) error {
	return WithLock(repo, func() error { return SaveLocked(repo, run) })
}

// SaveLocked persists one run while the caller holds WithLock. It is exposed
// within the internal module so workspace operations can hold one lock across
// their Git and runstate critical section without recursive file locking.
func SaveLocked(repo string, run Run) error {
	if err := normalizeSchemaVersion(&run); err != nil {
		return err
	}
	run.CreatedAt = run.CreatedAt.UTC()
	existing, err := Load(repo, run.RunID)
	if err == nil {
		if !existing.CreatedAt.Equal(run.CreatedAt) {
			return fmt.Errorf("created_at is immutable for run %s", run.RunID)
		}
		current := Run{Status: existing.Status}
		if transitionErr := current.Transition(run.Status); transitionErr != nil {
			return transitionErr
		}
	} else if !os.IsNotExist(unwrapPathError(err)) {
		return err
	}
	return saveWithRename(repo, run, os.Rename)
}

func Load(repo, runID string) (Run, error) {
	if !ValidRunID(runID) {
		return Run{}, fmt.Errorf("invalid run_id %q", runID)
	}
	content, err := os.ReadFile(runPath(repo, runID))
	if err != nil {
		return Run{}, err
	}
	var run Run
	if err := json.Unmarshal(content, &run); err != nil {
		return Run{}, fmt.Errorf("decode run %s: %w", runID, err)
	}
	if run.RunID != runID {
		return Run{}, fmt.Errorf("loaded run_id %q does not match requested run_id %q", run.RunID, runID)
	}
	if err := normalizeSchemaVersion(&run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func normalizeSchemaVersion(run *Run) error {
	if run.SchemaVersion == "" {
		run.SchemaVersion = CurrentSchemaVersion
	}
	if run.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("unsupported run schema version %q", run.SchemaVersion)
	}
	return nil
}

func ValidRunID(runID string) bool {
	if len(runID) != len("run_")+16 || !strings.HasPrefix(runID, "run_") {
		return false
	}
	for _, char := range strings.TrimPrefix(runID, "run_") {
		if (char >= 'a' && char <= 'f') || (char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}

func List(repo string) ([]Run, error) {
	entries, err := os.ReadDir(runsDir(repo))
	if os.IsNotExist(err) {
		return []Run{}, nil
	}
	if err != nil {
		return nil, err
	}
	runs := make([]Run, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		runID := strings.TrimSuffix(entry.Name(), ".json")
		run, err := Load(repo, runID)
		if err != nil {
			runs = append(runs, Run{RunID: runID, Status: StatusCorrupted})
			continue
		}
		if run.RunID != runID {
			runs = append(runs, Run{RunID: runID, Status: StatusCorrupted})
			continue
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].CreatedAt.IsZero() != runs[j].CreatedAt.IsZero() {
			return !runs[i].CreatedAt.IsZero()
		}
		if runs[i].CreatedAt.Equal(runs[j].CreatedAt) {
			return runs[i].RunID < runs[j].RunID
		}
		return runs[i].CreatedAt.Before(runs[j].CreatedAt)
	})
	return runs, nil
}

func Delete(repo, runID string) error {
	if !ValidRunID(runID) {
		return fmt.Errorf("invalid run_id %q", runID)
	}
	return os.Remove(runPath(repo, runID))
}

func saveWithRename(repo string, run Run, rename func(string, string) error) error {
	dir := runsDir(repo)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	content, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	temp, err := os.CreateTemp(dir, "."+run.RunID+"-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := temp.Write(content); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := rename(tempPath, runPath(repo, run.RunID)); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func runsDir(repo string) string {
	return filepath.Join(repo, ".git", "jacu", "runs")
}

func runPath(repo, runID string) string {
	return filepath.Join(runsDir(repo), runID+".json")
}

func unwrapPathError(err error) error {
	if pathErr, ok := err.(*os.PathError); ok {
		return pathErr.Err
	}
	return err
}
