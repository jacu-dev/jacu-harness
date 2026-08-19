package provenance

import (
	"fmt"
	"io"
	"io/fs"
	"strings"
)

type Commit struct {
	Hash           string
	AuthorName     string
	AuthorEmail    string
	CommitterName  string
	CommitterEmail string
	Subject        string
	Body           string
}

func ScanFiles(fsys fs.FS, paths []string) (Report, error) {
	var report Report
	var firstErr error
	for _, path := range paths {
		if !ExportablePath(path) {
			continue
		}
		file, err := fsys.Open(path)
		if err != nil {
			report.Gaps = append(report.Gaps, Gap{
				Check:  "scan_files",
				Reason: fmt.Sprintf("%s: %v", path, err),
			})
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			report.Gaps = append(report.Gaps, Gap{
				Check:  "scan_files",
				Reason: fmt.Sprintf("%s: stat: %v", path, err),
			})
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if info.IsDir() {
			_ = file.Close()
			continue
		}

		data, err := io.ReadAll(file)
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			report.Gaps = append(report.Gaps, Gap{
				Check:  "scan_files",
				Reason: fmt.Sprintf("%s: read: %v", path, err),
			})
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if containsNUL(data) {
			continue
		}
		scanFileData(&report, path, data)
	}
	return report, firstErr
}

func scanFileData(report *Report, path string, data []byte) {
	valid := strings.ToValidUTF8(string(data), "\uFFFD")
	rawLines := strings.Split(valid, "\n")
	normalizedLines := strings.Split(normalizeForMatch(valid), "\n")
	for index, normalizedLine := range normalizedLines {
		rawLine := ""
		if index < len(rawLines) {
			rawLine = strings.TrimSuffix(rawLines[index], "\r")
		}
		normalizedLine = strings.TrimSuffix(normalizedLine, "\r")
		for _, match := range findPatternMatches(normalizedLine) {
			class, rule := classifyMatch(path, normalizedLine, match)
			report.addFinding(Finding{
				Kind:    match.kind,
				Class:   class,
				Rule:    rule,
				Path:    path,
				Line:    index + 1,
				Excerpt: rawLine,
			})
		}
	}
}

func ScanCommits(commits []Commit) Report {
	var report Report
	for _, commit := range commits {
		scanCommitIdentity(&report, commit)
		scanCommitSubject(&report, commit)
		scanCommitBody(&report, commit)
	}
	return report
}

func scanCommitIdentity(report *Report, commit Commit) {
	for _, name := range []string{commit.AuthorName, commit.CommitterName} {
		if normalizeForMatch(strings.TrimSpace(name)) == "claude" {
			report.addFinding(Finding{
				Kind:    KindAIAuthor,
				Class:   ClassTrace,
				Rule:    "author-name-claude",
				Commit:  commit.Hash,
				Excerpt: name,
			})
		}
	}
	for _, email := range []string{commit.AuthorEmail, commit.CommitterEmail} {
		normalized := normalizeForMatch(email)
		for _, match := range findPatternMatches(normalized) {
			if match.kind != KindAIEmail {
				continue
			}
			report.addFinding(Finding{
				Kind:    match.kind,
				Class:   ClassTrace,
				Rule:    match.rule,
				Commit:  commit.Hash,
				Excerpt: email,
			})
		}
	}
}

func scanCommitSubject(report *Report, commit Commit) {
	for _, match := range findPatternMatches(normalizeForMatch(commit.Subject)) {
		if match.kind == KindAITrailer {
			continue
		}
		class, rule := classifyMatch("", normalizeForMatch(commit.Subject), match)
		report.addFinding(Finding{
			Kind:    match.kind,
			Class:   class,
			Rule:    rule,
			Commit:  commit.Hash,
			Line:    1,
			Excerpt: commit.Subject,
		})
	}
	for _, finding := range CheckSubject(commit.Subject) {
		finding.Commit = commit.Hash
		finding.Line = 1
		report.addFinding(finding)
	}
}

func scanCommitBody(report *Report, commit Commit) {
	valid := strings.ToValidUTF8(commit.Body, "\uFFFD")
	for index, rawLine := range strings.Split(valid, "\n") {
		normalizedLine := normalizeForMatch(strings.TrimSuffix(rawLine, "\r"))
		if strings.HasPrefix(normalizedLine, "co-authored-by:") {
			report.addFinding(Finding{
				Kind:    KindAITrailer,
				Class:   ClassTrace,
				Rule:    "co-authored-by",
				Commit:  commit.Hash,
				Line:    index + 1,
				Excerpt: strings.TrimSuffix(rawLine, "\r"),
			})
		}
		for _, match := range findPatternMatches(normalizedLine) {
			if match.kind == KindAITrailer {
				continue
			}
			class, rule := classifyMatch("", normalizedLine, match)
			report.addFinding(Finding{
				Kind:    match.kind,
				Class:   class,
				Rule:    rule,
				Commit:  commit.Hash,
				Line:    index + 1,
				Excerpt: strings.TrimSuffix(rawLine, "\r"),
			})
		}
	}
}

func containsNUL(data []byte) bool {
	if len(data) > 8192 {
		data = data[:8192]
	}
	for _, value := range data {
		if value == 0 {
			return true
		}
	}
	return false
}
