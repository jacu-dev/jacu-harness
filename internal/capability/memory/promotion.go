package memory

import (
	"errors"
	"fmt"
	"time"
)

// PromoteDerivedToConvention uses only the caller's already-computed eval
// outcome. It does not inspect eval content or invent a clock value.
func PromoteDerivedToConvention(store Store, id string, evalPassed bool, now time.Time) (Record, error) {
	if store == nil {
		return Record{}, errors.New("memory store is required")
	}
	if !evalPassed {
		return Record{}, errors.New("promotion blocked: eval did not pass")
	}
	if now.IsZero() {
		return Record{}, errors.New("promotion timestamp is required")
	}
	source, ok := store.Get(id)
	if !ok {
		return Record{}, fmt.Errorf("promotion source %q not found", id)
	}
	if source.Source != "derived" {
		return Record{}, errors.New("promotion source must be derived")
	}
	if source.Status != "active" {
		return Record{}, errors.New("promotion source must be active")
	}
	if source.Kind == "convention" {
		return Record{}, errors.New("promotion source is already a convention")
	}
	if source.ProjectID == "" {
		return Record{}, errors.New("global preference cannot become a convention")
	}

	successor := source
	successor.MemoryID = memoryID(Input{ProjectID: source.ProjectID, Kind: "convention", Title: source.Title})
	if existing, exists := store.Get(successor.MemoryID); exists && existing.MemoryID != source.MemoryID {
		return Record{}, fmt.Errorf("promotion successor %q already exists", successor.MemoryID)
	}
	successor.Kind = "convention"
	successor.Status = "active"
	successor.SupersededBy = ""
	successor.CreatedAt = now.UTC().Format(time.RFC3339Nano)
	successor.UpdatedAt = successor.CreatedAt
	if err := store.Save(successor, source.MemoryID); err != nil {
		return Record{}, fmt.Errorf("save promoted convention: %w", err)
	}
	return successor, nil
}
