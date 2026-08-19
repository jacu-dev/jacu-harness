package memory

import (
	"os"
	"sort"
	"strings"
	"unicode"
)

const minScore = 3

// Search returns no results when K is zero or negative so callers cannot
// accidentally request an unbounded listing.
func (s *fileStore) Search(q SearchQuery) []Scored {
	if q.K <= 0 || !validProjectID(q.ProjectID) {
		return []Scored{}
	}
	memoryRoot, err := s.openMemoryForRead()
	if err != nil {
		return []Scored{}
	}
	defer memoryRoot.Close()
	scope := scopeDirectory(q.ProjectID)
	scopeRoot, _, err := openRootChild(memoryRoot, scope, false, false, nil, scopeOpenHook(s.hooks.beforeScopeOpen, scope))
	if err != nil {
		return []Scored{}
	}
	defer scopeRoot.Close()
	entries, err := readRootDir(scopeRoot)
	if err != nil {
		return []Scored{}
	}

	kinds := makeSet(q.Kinds)
	queryTokens := tokenSet(q.Query)
	listing := len(queryTokens) == 0
	results := make([]Scored, 0)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		memoryID := strings.TrimSuffix(entry.Name(), ".json")
		if !memoryIDPattern.MatchString(memoryID) {
			continue
		}
		rec, ok := readRecordRoot(scopeRoot, entry.Name(), memoryID, q.ProjectID, s.hooks.beforeRecordOpen)
		if !ok || !visibleStatus(rec.Status, q.IncludeSuperseded) {
			continue
		}
		if len(kinds) > 0 {
			if _, ok := kinds[rec.Kind]; !ok {
				continue
			}
		}
		score := 0
		if !listing {
			score = scoreRecord(rec, queryTokens)
			if score < minScore {
				continue
			}
		}
		results = append(results, Scored{Record: rec, Score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Record.MemoryID < results[j].Record.MemoryID
	})
	if len(results) > q.K {
		results = results[:q.K]
	}
	return results
}

func scoreRecord(rec Record, queryTokens map[string]struct{}) int {
	title := tokenSet(rec.Title)
	body := tokenSet(rec.Body)
	evidence := tokenSet(strings.Join(rec.Evidence, " "))
	kind := tokenSet(rec.Kind)
	score := 0
	for token := range queryTokens {
		if _, ok := title[token]; ok {
			score += 3
		}
		if _, ok := body[token]; ok {
			score++
		}
		if _, ok := evidence[token]; ok {
			score++
		}
		if _, ok := kind[token]; ok {
			score += 2
		}
	}
	return score
}

func tokenSet(value string) map[string]struct{} {
	tokens := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	return makeSet(tokens)
}

func makeSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func visibleStatus(status string, includeSuperseded bool) bool {
	return status == "active" || includeSuperseded && status == "superseded"
}
