package provenance

import (
	"strings"
	"unicode"
)

var policyPaths = []string{
	"internal/provenance/",
	"docs/adr/ADR-028-open-source-export.md",
	"docs/sdd/016-open-source-export/",
	"docs/plans/one-shot-open-source.md",
	"CONTRIBUTING.md",
	"internal/export/",
	"docs/export/",
	"scripts/export/",
}

type patternMatch struct {
	kind    Kind
	rule    string
	start   int
	end     int
	product bool
}

type textPattern struct {
	kind Kind
	rule string
	text string
}

var attributionPatterns = []textPattern{
	{kind: KindAIEmail, rule: "email-anthropic", text: "noreply@anthropic.com"},
	{kind: KindAIEmail, rule: "email-cursor", text: "cursoragent@cursor.com"},
	{kind: KindAITrailer, rule: "co-authored-by", text: "co-authored-by"},
	{kind: KindGeneratedWith, rule: "generated-with", text: "generated with"},
	{kind: KindRobotEmoji, rule: "robot-emoji", text: string(rune(0x1F916))},
}

var hostPatterns = []struct {
	rule string
	text string
}{
	{rule: "host-claude", text: "claude"},
	{rule: "host-codex", text: "codex"},
	{rule: "host-cursor", text: "cursor"},
	{rule: "host-copilot", text: "copilot"},
	{rule: "host-anthropic", text: "anthropic"},
	{rule: "host-chatgpt", text: "chatgpt"},
	{rule: "host-gpt-4", text: "gpt-4"},
	{rule: "host-gpt-5", text: "gpt-5"},
}

func normalizeForMatch(value string) string {
	value = strings.ToValidUTF8(value, "\uFFFD")
	value = normalizeCompatibility(value)
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return r
		}
		if r == '\uFE0F' {
			return -1
		}
		if unicode.IsSpace(r) {
			return ' '
		}
		return unicode.ToLower(r)
	}, value)
}

// normalizeCompatibility covers the compatibility forms that can otherwise
// hide an ASCII attribution marker, including full-width text and common
// presentation ligatures. Canonical composition is immaterial after case
// folding because the patterns contain no composed accents.
func normalizeCompatibility(value string) string {
	var normalized strings.Builder
	normalized.Grow(len(value))
	for _, r := range value {
		if r >= 0xFF01 && r <= 0xFF5E {
			normalized.WriteRune(r - 0xFEE0)
			continue
		}
		if r == 0x3000 {
			normalized.WriteByte(' ')
			continue
		}
		compatibility := ""
		switch r {
		case '\u00AA':
			compatibility = "a"
		case '\u00BA':
			compatibility = "o"
		case '\u017F':
			compatibility = "s"
		case '\u01C0':
			compatibility = "|"
		case '\u01C3':
			compatibility = "!"
		case '\u2071':
			compatibility = "i"
		case '\u207F':
			compatibility = "n"
		case '\u2080':
			compatibility = "0"
		case '\u2081':
			compatibility = "1"
		case '\u2082':
			compatibility = "2"
		case '\u2083':
			compatibility = "3"
		case '\u2084':
			compatibility = "4"
		case '\u2085':
			compatibility = "5"
		case '\u2086':
			compatibility = "6"
		case '\u2087':
			compatibility = "7"
		case '\u2088':
			compatibility = "8"
		case '\u2089':
			compatibility = "9"
		case '\u00B9':
			compatibility = "1"
		case '\u00B2':
			compatibility = "2"
		case '\u00B3':
			compatibility = "3"
		}
		if compatibility == "" {
			switch r {
			case '\uFB00':
				compatibility = "ff"
			case '\uFB01':
				compatibility = "fi"
			case '\uFB02':
				compatibility = "fl"
			case '\uFB03':
				compatibility = "ffi"
			case '\uFB04':
				compatibility = "ffl"
			case '\uFB05':
				compatibility = "st"
			case '\uFB06':
				compatibility = "st"
			}
		}
		if compatibility != "" {
			normalized.WriteString(compatibility)
		} else {
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}

func findPatternMatches(line string) []patternMatch {
	var matches []patternMatch
	for _, pattern := range attributionPatterns {
		for start := 0; start < len(line); {
			offset := strings.Index(line[start:], pattern.text)
			if offset < 0 {
				break
			}
			offset += start
			matches = append(matches, patternMatch{
				kind:  pattern.kind,
				rule:  pattern.rule,
				start: offset,
				end:   offset + len(pattern.text),
			})
			start = offset + len(pattern.text)
		}
	}
	for _, pattern := range hostPatterns {
		for start := 0; start < len(line); {
			offset := strings.Index(line[start:], pattern.text)
			if offset < 0 {
				break
			}
			offset += start
			end := offset + len(pattern.text)
			if wholeWordAt(line, offset, end) {
				matches = append(matches, patternMatch{
					kind:    KindAIAuthor,
					rule:    pattern.rule,
					start:   offset,
					end:     end,
					product: true,
				})
			}
			start = offset + len(pattern.text)
		}
	}
	return matches
}

func wholeWordAt(value string, start, end int) bool {
	if start > 0 {
		before, _ := decodeLastRune(value[:start])
		if isWordRune(before) {
			return false
		}
	}
	if end < len(value) {
		after, _ := decodeFirstRune(value[end:])
		if isWordRune(after) {
			return false
		}
	}
	return true
}

func decodeFirstRune(value string) (rune, int) {
	for _, r := range value {
		return r, len(string(r))
	}
	return 0, 0
}

func decodeLastRune(value string) (rune, int) {
	var last rune
	size := 0
	for _, r := range value {
		last = r
		size = len(string(r))
	}
	return last, size
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

var ignoredFilePrefixes = []string{
	"docs/heranca/",
	"docs/relatorios/",
	"docs/sdd/archive/",
	"docs/evals/",
}

func ExportablePath(path string) bool {
	path = strings.ReplaceAll(path, "\\", "/")
	for _, prefix := range ignoredFilePrefixes {
		if path == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(path, prefix) {
			return false
		}
	}
	return true
}

func isPolicyPath(path string) bool {
	for _, prefix := range policyPaths {
		if strings.HasSuffix(prefix, "/") {
			if strings.HasPrefix(path, prefix) {
				return true
			}
			continue
		}
		if path == prefix {
			return true
		}
	}
	return false
}
