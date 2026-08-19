package memory

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type retrievalEvalQuery struct {
	Query    string   `json:"query"`
	Expected []string `json:"expected"`
	K        int      `json:"k"`
}

type retrievalEvalQueryWire struct {
	Query    string    `json:"query"`
	Expected *[]string `json:"expected"`
	K        int       `json:"k"`
}

type retrievalRecall struct {
	Relevant int
	HitsAt3  int
	HitsAt5  int
}

func (r retrievalRecall) at3() float64 {
	return recallRatio(r.HitsAt3, r.Relevant)
}

func (r retrievalRecall) at5() float64 {
	return recallRatio(r.HitsAt5, r.Relevant)
}

type retrievalEvalReport struct {
	RecallAt3        float64
	RecallAt5        float64
	PerKind          map[string]retrievalRecall
	NegativeTotal    int
	NegativeFailures int
}

func TestNaiveRetrievalRecall(t *testing.T) {
	report := runRetrievalEval(t)
	t.Logf("naive aggregate recall@3=%.4f recall@5=%.4f", report.RecallAt3, report.RecallAt5)
	kinds := make([]string, 0, len(report.PerKind))
	for kind := range report.PerKind {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		metrics := report.PerKind[kind]
		t.Logf("naive kind=%s relevant=%d recall@3=%.4f recall@5=%.4f", kind, metrics.Relevant, metrics.at3(), metrics.at5())
	}
	t.Logf("naive negatives passed=%d total=%d", report.NegativeTotal-report.NegativeFailures, report.NegativeTotal)

	if report.RecallAt3 < 0.70 {
		t.Fatalf("aggregate recall@3 = %.4f; want >= 0.70", report.RecallAt3)
	}
	if report.NegativeFailures != 0 {
		t.Fatalf("negative queries returning results = %d; want 0", report.NegativeFailures)
	}
}

func runRetrievalEval(t *testing.T) retrievalEvalReport {
	t.Helper()

	corpus, err := decodeRetrievalCorpus(readRetrievalFixture(t, "corpus.json"))
	if err != nil {
		t.Fatalf("decode eval fixture corpus.json: %v", err)
	}
	queries, err := decodeRetrievalQueries(readRetrievalFixture(t, "queries.json"))
	if err != nil {
		t.Fatalf("decode eval fixture queries.json: %v", err)
	}
	if err := validateRetrievalFixtures(corpus, queries); err != nil {
		t.Fatalf("validate eval fixtures: %v", err)
	}

	store := NewFileStore(t.TempDir())
	recordsByID := make(map[string]Record, len(corpus))
	defaultProjectID := ""
	for _, rec := range corpus {
		if err := store.Save(rec, ""); err != nil {
			t.Fatalf("save corpus record %s: %v", rec.MemoryID, err)
		}
		if _, exists := recordsByID[rec.MemoryID]; exists {
			t.Fatalf("duplicate corpus memory_id %s", rec.MemoryID)
		}
		recordsByID[rec.MemoryID] = rec
		if rec.ProjectID != "" {
			if defaultProjectID != "" && rec.ProjectID != defaultProjectID {
				t.Fatalf("corpus contains multiple non-global project_ids: %s and %s", defaultProjectID, rec.ProjectID)
			}
			defaultProjectID = rec.ProjectID
		}
	}
	if defaultProjectID == "" {
		t.Fatal("corpus has no project-scoped record")
	}

	report := retrievalEvalReport{PerKind: make(map[string]retrievalRecall)}
	aggregate := retrievalRecall{}
	for index, query := range queries {
		if query.K != 3 {
			t.Fatalf("query %d has k=%d; want 3", index, query.K)
		}
		projectID := defaultProjectID
		if len(query.Expected) > 0 {
			projectID = projectForExpected(t, query, recordsByID)
		}
		results := store.Search(SearchQuery{ProjectID: projectID, Query: query.Query, K: 5})
		if len(query.Expected) == 0 {
			report.NegativeTotal++
			if len(results) != 0 {
				report.NegativeFailures++
				t.Logf("negative query %q returned %v", query.Query, resultIDs(results))
			}
			continue
		}

		for _, expectedID := range query.Expected {
			rec := recordsByID[expectedID]
			hitAt3 := containsResult(results, expectedID, 3)
			hitAt5 := containsResult(results, expectedID, 5)
			aggregate.Relevant++
			kindMetrics := report.PerKind[rec.Kind]
			kindMetrics.Relevant++
			if hitAt3 {
				aggregate.HitsAt3++
				kindMetrics.HitsAt3++
			}
			if hitAt5 {
				aggregate.HitsAt5++
				kindMetrics.HitsAt5++
			}
			report.PerKind[rec.Kind] = kindMetrics
		}
	}
	report.RecallAt3 = aggregate.at3()
	report.RecallAt5 = aggregate.at5()
	return report
}

func readRetrievalFixture(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("evaldata", name))
	if err != nil {
		t.Fatalf("read eval fixture %s: %v", name, err)
	}
	return content
}

func decodeRetrievalCorpus(content []byte) ([]Record, error) {
	var corpus []Record
	if err := decodeSingleRetrievalJSON(content, &corpus); err != nil {
		return nil, err
	}
	return corpus, nil
}

func decodeRetrievalQueries(content []byte) ([]retrievalEvalQuery, error) {
	var wire []retrievalEvalQueryWire
	if err := decodeSingleRetrievalJSON(content, &wire); err != nil {
		return nil, err
	}
	queries := make([]retrievalEvalQuery, len(wire))
	for index, query := range wire {
		if query.Expected == nil {
			return nil, fmt.Errorf("query %d: expected is required and must be an array", index)
		}
		expected := make([]string, len(*query.Expected))
		copy(expected, *query.Expected)
		queries[index] = retrievalEvalQuery{Query: query.Query, Expected: expected, K: query.K}
	}
	return queries, nil
}

func decodeSingleRetrievalJSON(content []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON: %w", err)
	}
	return nil
}

func validateRetrievalFixtures(corpus []Record, queries []retrievalEvalQuery) error {
	if len(corpus) < 40 || len(corpus) > 60 {
		return fmt.Errorf("corpus has %d records; want 40 to 60", len(corpus))
	}
	kindMinimums := map[string]int{
		"decision":   10,
		"convention": 10,
		"gotcha":     10,
		"preference": 5,
	}
	kindCounts := make(map[string]int, len(kindMinimums))
	recordsByID := make(map[string]Record, len(corpus))
	for _, rec := range corpus {
		kindCounts[rec.Kind]++
		recordsByID[rec.MemoryID] = rec
	}
	for _, kind := range []string{"decision", "convention", "gotcha", "preference"} {
		if kindCounts[kind] < kindMinimums[kind] {
			return fmt.Errorf("corpus has %d %s records; want at least %d", kindCounts[kind], kind, kindMinimums[kind])
		}
	}
	if len(queries) != 20 {
		return fmt.Errorf("eval has %d queries; want exactly 20 queries", len(queries))
	}

	negativeCount := 0
	relevant := 0
	expectedKinds := make(map[string]struct{}, 4)
	for index, query := range queries {
		if strings.TrimSpace(query.Query) == "" {
			return fmt.Errorf("query %d: query must not be blank", index)
		}
		if query.K != 3 {
			return fmt.Errorf("query %d has k=%d; want 3", index, query.K)
		}
		if query.Expected == nil {
			return fmt.Errorf("query %d: expected is required and must be an array", index)
		}
		if len(query.Expected) == 0 {
			negativeCount++
			continue
		}
		if len(query.Expected) > 3 {
			return fmt.Errorf("query %d has %d expected IDs; want 1 to 3 expected IDs", index, len(query.Expected))
		}
		relevant += len(query.Expected)
		seen := make(map[string]struct{}, len(query.Expected))
		expectedProjectID := ""
		hasExpectedProject := false
		for _, expectedID := range query.Expected {
			if _, duplicate := seen[expectedID]; duplicate {
				return fmt.Errorf("query %d has duplicate expected memory_id %s", index, expectedID)
			}
			seen[expectedID] = struct{}{}
			rec, ok := recordsByID[expectedID]
			if !ok {
				return fmt.Errorf("query %d references unknown memory_id %s", index, expectedID)
			}
			if hasExpectedProject && rec.ProjectID != expectedProjectID {
				return fmt.Errorf("query %d mixes project scopes %q and %q", index, expectedProjectID, rec.ProjectID)
			}
			expectedProjectID = rec.ProjectID
			hasExpectedProject = true
			expectedKinds[rec.Kind] = struct{}{}
		}
	}
	if negativeCount < 3 {
		return fmt.Errorf("eval has %d negative queries; want at least 3", negativeCount)
	}
	if relevant == 0 {
		return errors.New("eval requires at least one positive query and Relevant > 0")
	}
	for _, kind := range []string{"decision", "convention", "gotcha", "preference"} {
		if _, ok := expectedKinds[kind]; !ok {
			return fmt.Errorf("expected records do not cover kind %s", kind)
		}
	}
	return nil
}

func projectForExpected(t *testing.T, query retrievalEvalQuery, recordsByID map[string]Record) string {
	t.Helper()
	projectID := ""
	initialized := false
	for _, expectedID := range query.Expected {
		rec, ok := recordsByID[expectedID]
		if !ok {
			t.Fatalf("query %q references unknown memory_id %s", query.Query, expectedID)
		}
		if !initialized {
			projectID = rec.ProjectID
			initialized = true
			continue
		}
		if rec.ProjectID != projectID {
			t.Fatalf("query %q mixes project scopes %q and %q", query.Query, projectID, rec.ProjectID)
		}
	}
	return projectID
}

func containsResult(results []Scored, memoryID string, limit int) bool {
	if limit > len(results) {
		limit = len(results)
	}
	for _, result := range results[:limit] {
		if result.Record.MemoryID == memoryID {
			return true
		}
	}
	return false
}

func resultIDs(results []Scored) []string {
	ids := make([]string, 0, len(results))
	for _, result := range results {
		ids = append(ids, fmt.Sprintf("%s(%d)", result.Record.MemoryID, result.Score))
	}
	return ids
}

func recallRatio(hits, relevant int) float64 {
	if relevant == 0 {
		return 1
	}
	return float64(hits) / float64(relevant)
}
