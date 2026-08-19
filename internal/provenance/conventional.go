package provenance

import (
	"regexp"
	"strings"
)

var conventionalSubjectPattern = regexp.MustCompile(`^(feat|fix|docs|style|refactor|test|chore|ci|build|perf|revert)(\([^)]+\))?(!)?: \S`)

var portugueseSubjectWords = []string{
	"para",
	"com",
	"sem",
	"não",
	"nao",
	"uma",
	"pelo",
	"pela",
	"dos",
	"das",
	"que",
	"como",
	"este",
	"esta",
	"arquivo",
	"repositório",
	"repositorio",
}

func CheckSubject(subject string) []Finding {
	normalized := normalizeForMatch(subject)
	findings := make([]Finding, 0, 2)
	for _, word := range portugueseSubjectWords {
		if containsWholeWord(normalized, word) {
			findings = append(findings, Finding{
				Kind:    KindNonEnglishSubject,
				Class:   ClassTrace,
				Rule:    "subject-non-english",
				Excerpt: subject,
			})
			break
		}
	}
	if !conventionalSubjectPattern.MatchString(normalized) {
		findings = append(findings, Finding{
			Kind:    KindNonConventionalSubject,
			Class:   ClassTrace,
			Rule:    "subject-non-conventional",
			Excerpt: subject,
		})
	}
	return findings
}

func containsWholeWord(value, word string) bool {
	for start := 0; start < len(value); {
		offset := strings.Index(value[start:], word)
		if offset < 0 {
			return false
		}
		offset += start
		end := offset + len(word)
		if wholeWordAt(value, offset, end) {
			return true
		}
		start = end
	}
	return false
}
