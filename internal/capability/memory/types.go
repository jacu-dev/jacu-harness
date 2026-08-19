package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

type Input struct {
	ProjectID  string   `json:"project_id"`
	Kind       string   `json:"kind"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Evidence   []string `json:"evidence,omitempty"`
	Source     string   `json:"source"`
	Supersedes string   `json:"supersedes,omitempty"`
}

type Record struct {
	MemoryID     string   `json:"memory_id"`
	ProjectID    string   `json:"project_id"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Body         string   `json:"body"`
	Evidence     []string `json:"evidence"`
	Source       string   `json:"source"`
	Status       string   `json:"status"`
	SupersededBy string   `json:"superseded_by"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

type Lint struct {
	Level   string `json:"level"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func normalize(in Input) Input {
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.Kind = strings.TrimSpace(in.Kind)
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	in.Evidence = normalizeEvidence(in.Evidence)
	in.Source = strings.TrimSpace(in.Source)
	in.Supersedes = strings.TrimSpace(in.Supersedes)
	return in
}

func memoryID(in Input) string {
	in = normalize(in)
	title := strings.ToLower(strings.Join(strings.Fields(in.Title), " "))
	payload := in.ProjectID + "\x00" + in.Kind + "\x00" + title
	digest := sha256.Sum256([]byte(payload))
	return "mem_" + hex.EncodeToString(digest[:8])
}

func normalizeEvidence(evidence []string) []string {
	if evidence == nil {
		return nil
	}
	unique := make(map[string]struct{}, len(evidence))
	for _, item := range evidence {
		unique[strings.TrimSpace(item)] = struct{}{}
	}
	normalized := make([]string, 0, len(unique))
	for item := range unique {
		normalized = append(normalized, item)
	}
	sort.Strings(normalized)
	return normalized
}
