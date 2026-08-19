package provenance

func Classify(path, line string, kind Kind) (Class, string) {
	normalized := normalizeForMatch(line)
	for _, match := range findPatternMatches(normalized) {
		if match.kind != kind {
			continue
		}
		return classifyMatch(path, normalized, match)
	}
	return ClassTrace, "unclassified-match"
}

func classifyMatch(path, line string, match patternMatch) (Class, string) {
	if match.product {
		if isPolicyPath(path) && insideQuoted(line, match.start, match.end) {
			return ClassPolicy, match.rule
		}
		return ClassProduct, match.rule
	}
	if isPolicyPath(path) && insideQuoted(line, match.start, match.end) {
		return ClassPolicy, match.rule
	}
	return ClassTrace, match.rule
}

func insideQuoted(line string, start, end int) bool {
	for _, quote := range []byte{'`', '"'} {
		open := -1
		for index := 0; index < len(line); index++ {
			if line[index] != quote || escaped(line, index) {
				continue
			}
			if open < 0 {
				open = index
				continue
			}
			if start > open && end <= index {
				return true
			}
			open = -1
		}
	}
	return false
}

func escaped(value string, index int) bool {
	backslashes := 0
	for index > 0 && value[index-1] == '\\' {
		index--
		backslashes++
	}
	return backslashes%2 == 1
}
