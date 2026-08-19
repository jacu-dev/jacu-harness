package memory

import (
	"fmt"
	"strings"
	"testing"
)

func TestDecodeRetrievalCorpusRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "unknown field",
			content: `[{"memory_id":"mem_0000000000000001","unexpected":true}]`,
			want:    "unknown field",
		},
		{
			name:    "trailing JSON",
			content: `[] {}`,
			want:    "trailing JSON",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeRetrievalCorpus([]byte(test.content))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("decodeRetrievalCorpus error = %v; want containing %q", err, test.want)
			}
		})
	}
}

func TestDecodeRetrievalQueriesRequiresExpectedAndRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "missing expected",
			content: `[{"query":"question","k":3}]`,
			want:    "expected is required",
		},
		{
			name:    "unknown field",
			content: `[{"query":"question","expected":[],"k":3,"unexpected":true}]`,
			want:    "unknown field",
		},
		{
			name:    "trailing JSON",
			content: `[{"query":"question","expected":[],"k":3}] {}`,
			want:    "trailing JSON",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeRetrievalQueries([]byte(test.content))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("decodeRetrievalQueries error = %v; want containing %q", err, test.want)
			}
		})
	}

	queries, err := decodeRetrievalQueries([]byte(`[{"query":"negative","expected":[],"k":3}]`))
	if err != nil {
		t.Fatalf("explicit empty expected must remain a valid negative: %v", err)
	}
	if len(queries) != 1 || queries[0].Expected == nil || len(queries[0].Expected) != 0 {
		t.Fatalf("decoded explicit negative = %#v; want one non-nil empty expected", queries)
	}
}

func TestValidateRetrievalFixturesRejectsVacuousOrMalformedEval(t *testing.T) {
	tests := []struct {
		name   string
		mutate func([]Record, []retrievalEvalQuery) ([]Record, []retrievalEvalQuery)
		want   string
	}{
		{
			name: "corpus below 40",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				return records[:39], queries
			},
			want: "40 to 60",
		},
		{
			name: "corpus above 60",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				for len(records) < 61 {
					records = append(records, evalIntegrityRecord(len(records)+1, "decision"))
				}
				return records, queries
			},
			want: "40 to 60",
		},
		{
			name: "fewer than 10 decisions",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				records[0].Kind = "preference"
				return records, queries
			},
			want: "decision records; want at least 10",
		},
		{
			name: "fewer than 10 conventions",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				records[10].Kind = "preference"
				return records, queries
			},
			want: "convention records; want at least 10",
		},
		{
			name: "fewer than 10 gotchas",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				records[20].Kind = "preference"
				return records, queries
			},
			want: "gotcha records; want at least 10",
		},
		{
			name: "fewer than 5 preferences",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				for index := 30; index < 36; index++ {
					records[index].Kind = "decision"
				}
				return records, queries
			},
			want: "preference records; want at least 5",
		},
		{
			name: "fewer than 20 queries",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				return records, queries[:19]
			},
			want: "exactly 20 queries",
		},
		{
			name: "more than 20 queries",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				return records, append(queries, retrievalEvalQuery{Query: "extra negative", Expected: []string{}, K: 3})
			},
			want: "exactly 20 queries",
		},
		{
			name: "blank trimmed query",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				queries[0].Query = " \t\n "
				return records, queries
			},
			want: "query must not be blank",
		},
		{
			name: "k is not 3",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				queries[0].K = 5
				return records, queries
			},
			want: "k=5; want 3",
		},
		{
			name: "positive has more than 3 expected IDs",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				queries[0].Expected = []string{records[0].MemoryID, records[1].MemoryID, records[2].MemoryID, records[3].MemoryID}
				return records, queries
			},
			want: "1 to 3 expected IDs",
		},
		{
			name: "positive has duplicate expected IDs",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				queries[0].Expected = []string{records[0].MemoryID, records[0].MemoryID}
				return records, queries
			},
			want: "duplicate expected memory_id",
		},
		{
			name: "no positive query",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				for index := range queries {
					queries[index].Expected = []string{}
				}
				return records, queries
			},
			want: "at least one positive query",
		},
		{
			name: "unknown expected ID",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				queries[0].Expected = []string{"mem_ffffffffffffffff"}
				return records, queries
			},
			want: "unknown memory_id",
		},
		{
			name: "mixed expected scopes",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				records[30].ProjectID = ""
				queries[0].Expected = []string{records[0].MemoryID, records[30].MemoryID}
				return records, queries
			},
			want: "mixes project scopes",
		},
		{
			name: "expected records do not cover all kinds",
			mutate: func(records []Record, queries []retrievalEvalQuery) ([]Record, []retrievalEvalQuery) {
				for index := 0; index < 17; index++ {
					queries[index].Expected = []string{records[index%10].MemoryID}
				}
				return records, queries
			},
			want: "expected records do not cover kind",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			records, queries := validRetrievalEvalFixtures()
			records, queries = test.mutate(records, queries)
			err := validateRetrievalFixtures(records, queries)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateRetrievalFixtures error = %v; want containing %q", err, test.want)
			}
		})
	}
}

func TestValidateRetrievalFixturesAcceptsAtLeastThreeExplicitNegatives(t *testing.T) {
	records, queries := validRetrievalEvalFixtures()
	queries[16].Expected = []string{}
	if err := validateRetrievalFixtures(records, queries); err != nil {
		t.Fatalf("four explicit negatives must remain valid: %v", err)
	}
}

func validRetrievalEvalFixtures() ([]Record, []retrievalEvalQuery) {
	kinds := []string{"decision", "convention", "gotcha", "preference"}
	records := make([]Record, 0, 40)
	for _, kind := range kinds {
		for count := 0; count < 10; count++ {
			records = append(records, evalIntegrityRecord(len(records)+1, kind))
		}
	}
	queries := make([]retrievalEvalQuery, 20)
	for index := 0; index < 17; index++ {
		recordIndex := (index%4)*10 + index/4
		queries[index] = retrievalEvalQuery{
			Query:    fmt.Sprintf("query %d", index+1),
			Expected: []string{records[recordIndex].MemoryID},
			K:        3,
		}
	}
	for index := 17; index < 20; index++ {
		queries[index] = retrievalEvalQuery{
			Query:    fmt.Sprintf("negative query %d", index+1),
			Expected: []string{},
			K:        3,
		}
	}
	return records, queries
}

func evalIntegrityRecord(sequence int, kind string) Record {
	return Record{
		MemoryID:     fmt.Sprintf("mem_%016x", sequence),
		ProjectID:    "prj_0123456789abcdef",
		Kind:         kind,
		Title:        fmt.Sprintf("Record %d", sequence),
		Body:         "Fixture body",
		Evidence:     []string{"docs/evidence.md"},
		Source:       "human",
		Status:       "active",
		SupersededBy: "",
		CreatedAt:    "2026-07-31T00:00:00Z",
		UpdatedAt:    "2026-07-31T00:00:00Z",
	}
}
