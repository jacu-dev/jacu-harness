package provenance

import (
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func emailAddress() string {
	return "noreply@" + "anthropic" + ".com"
}

func cursorEmailAddress() string {
	return "cursoragent@" + "cursor" + ".com"
}

func coAuthoredBy() string {
	return "Co-" + "Authored-By"
}

func generatedWith() string {
	return "Generated" + " with"
}

func robotEmoji() string {
	return string(rune(0x1F916))
}

func TestScanFilesFindsEveryKindAndNormalizesInput(t *testing.T) {
	files := fstest.MapFS{
		"signals.txt": &fstest.MapFile{Data: []byte(strings.Join([]string{
			"Claude",
			emailAddress(),
			"cO-aUtHoReD-bY: human",
			"Generated" + "\t" + "with",
			robotEmoji() + string(rune(0xFE0F)),
			"supports" + "\u00a0" + "Ｃｌａｕｄｅ",
		}, "\r\n"))},
	}

	report, err := ScanFiles(files, []string{"signals.txt"})
	if err != nil {
		t.Fatalf("ScanFiles() error = %v", err)
	}

	for _, want := range []Kind{
		KindAIAuthor,
		KindAITrailer,
		KindAIEmail,
		KindGeneratedWith,
		KindRobotEmoji,
	} {
		if !hasFinding(report.Findings, want, "") {
			t.Fatalf("ScanFiles() missing kind %q in %#v", want, report.Findings)
		}
	}
	if report.Products != 3 {
		t.Fatalf("ScanFiles() products = %d, want 3", report.Products)
	}
	if report.Traces != 4 {
		t.Fatalf("ScanFiles() traces = %d, want 4", report.Traces)
	}
	if report.Policies != 0 || len(report.Gaps) != 0 {
		t.Fatalf("ScanFiles() policy/gaps = %d/%v, want 0/none", report.Policies, report.Gaps)
	}
}

func TestScanFilesSkipsDirectoriesAndBinaryData(t *testing.T) {
	files := fstest.MapFS{
		"dir":             &fstest.MapFile{Mode: fs.ModeDir},
		"dir/ignored.txt": &fstest.MapFile{Data: []byte(emailAddress())},
		"nul.bin":         &fstest.MapFile{Data: []byte("prefix\x00" + emailAddress())},
		"invalid.bin":     &fstest.MapFile{Data: append([]byte{0xff, 0xfe}, []byte(emailAddress())...)},
		"invalid-nul.bin": &fstest.MapFile{Data: append([]byte{0xff, 0x00}, []byte(emailAddress())...)},
	}

	report, err := ScanFiles(files, []string{"dir", "nul.bin", "invalid.bin", "invalid-nul.bin"})
	if err != nil {
		t.Fatalf("ScanFiles() error = %v", err)
	}
	if len(report.Findings) != 2 || !hasFinding(report.Findings, KindAIEmail, "") {
		t.Fatalf("ScanFiles() findings = %#v, want invalid-UTF-8 email and host findings", report.Findings)
	}
}

func TestScanFilesReportsReadGaps(t *testing.T) {
	report, err := ScanFiles(fstest.MapFS{}, []string{"missing.txt"})
	if err == nil {
		t.Fatalf("ScanFiles() error = nil, want missing-file error")
	}
	if len(report.Gaps) != 1 || report.Gaps[0].Check != "scan_files" {
		t.Fatalf("ScanFiles() gaps = %#v, want one scan_files gap", report.Gaps)
	}
	if report.Clean() {
		t.Fatalf("Report.Clean() = true for a report with a gap")
	}
}

func TestPolicyPathsAreClosedSet(t *testing.T) {
	want := []string{
		"internal/provenance/",
		"docs/adr/ADR-028-open-source-export.md",
		"docs/sdd/016-open-source-export/",
		"docs/plans/one-shot-open-source.md",
		"CONTRIBUTING.md",
		"internal/export/",
		"docs/export/",
		"scripts/export/",
	}
	if !reflect.DeepEqual(policyPaths, want) {
		t.Fatalf("policyPaths = %#v, want closed set %#v", policyPaths, want)
	}
}

func TestPolicyQuotingAndUnquotedMatches(t *testing.T) {
	quotedBacktick := "`" + emailAddress() + "`"
	quotedDouble := `"` + generatedWith() + `"`

	for _, test := range []struct {
		name  string
		line  string
		class Class
	}{
		{name: "backtick", line: quotedBacktick, class: ClassPolicy},
		{name: "double quote", line: quotedDouble, class: ClassPolicy},
		{name: "unquoted", line: emailAddress(), class: ClassTrace},
		{name: "unquoted host", line: "Claude", class: ClassProduct},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, rule := Classify("CONTRIBUTING.md", test.line, kindForLine(test.line))
			if got != test.class {
				t.Fatalf("Classify() class = %q, want %q", got, test.class)
			}
			if rule == "" {
				t.Fatalf("Classify() rule is empty")
			}
		})
	}

	report, err := ScanFiles(fstest.MapFS{
		"CONTRIBUTING.md": &fstest.MapFile{Data: []byte(strings.Join([]string{
			quotedBacktick,
			quotedDouble,
			emailAddress(),
		}, "\n"))},
	}, []string{"CONTRIBUTING.md"})
	if err != nil {
		t.Fatalf("ScanFiles() error = %v", err)
	}
	if report.Policies != 3 || report.Traces != 1 {
		t.Fatalf("ScanFiles() policy/trace counts = %d/%d, want 3/1", report.Policies, report.Traces)
	}
}

func TestScanCommitsFindsMetadataTrailersAndSubjectKinds(t *testing.T) {
	commits := []Commit{
		{
			Hash:        "abc123",
			AuthorName:  "Claude",
			AuthorEmail: emailAddress(),
			Subject:     "melhorar" + " o arquivo para exportação",
			Body:        "body\n" + "cO-aUtHoReD-bY: any person\n" + generatedWith() + " " + robotEmoji(),
		},
		{
			Hash:    "def456",
			Subject: "unconventional subject",
		},
	}

	report := ScanCommits(commits)
	for _, want := range []Kind{
		KindAIAuthor,
		KindAIEmail,
		KindAITrailer,
		KindGeneratedWith,
		KindRobotEmoji,
		KindNonEnglishSubject,
		KindNonConventionalSubject,
	} {
		if !hasAnyKind(report.Findings, want) {
			t.Fatalf("ScanCommits() missing kind %q in %#v", want, report.Findings)
		}
	}
	if report.Traces != len(report.Findings) {
		t.Fatalf("ScanCommits() traces = %d, findings = %d, want all traces", report.Traces, len(report.Findings))
	}
	if report.Products != 0 || len(report.Gaps) != 0 {
		t.Fatalf("ScanCommits() products/gaps = %d/%v, want 0/none", report.Products, report.Gaps)
	}
}

func TestScanCommitsTreatsAnyTrailerAsTrace(t *testing.T) {
	report := ScanCommits([]Commit{{Hash: "h", Body: "co-authored-by: owner <owner@example.test>"}})
	if !hasFinding(report.Findings, KindAITrailer, "h") {
		t.Fatalf("ScanCommits() findings = %#v, want arbitrary trailer finding", report.Findings)
	}
}

func TestScanCommitsKeepsHostReferencesAsProducts(t *testing.T) {
	report := ScanCommits([]Commit{{
		Hash:    "h",
		Subject: "feat: support Claude and Codex",
		Body:    "also Cursor and GPT-5",
	}})
	if report.Traces != 0 || report.Products != 4 {
		t.Fatalf("ScanCommits() traces/products = %d/%d, want 0/4", report.Traces, report.Products)
	}
}

func TestCheckSubject(t *testing.T) {
	tests := []struct {
		name  string
		input string
		kinds []Kind
	}{
		{name: "valid conventional English", input: "feat(parser): add support", kinds: nil},
		{name: "valid bang", input: "fix!: repair crash", kinds: nil},
		{name: "Portuguese conventional", input: "docs: atualizar o arquivo", kinds: []Kind{KindNonEnglishSubject}},
		{name: "non conventional English", input: "Update the README", kinds: []Kind{KindNonConventionalSubject}},
		{name: "both checks", input: "atualizar o arquivo para exportação", kinds: []Kind{KindNonEnglishSubject, KindNonConventionalSubject}},
		{name: "word boundary", input: "feat: comparable parameter", kinds: nil},
		{name: "missing description", input: "feat: ", kinds: []Kind{KindNonConventionalSubject}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := CheckSubject(test.input)
			if len(got) != len(test.kinds) {
				t.Fatalf("CheckSubject() = %#v, want kinds %#v", got, test.kinds)
			}
			for index, want := range test.kinds {
				if got[index].Kind != want || got[index].Class != ClassTrace {
					t.Fatalf("CheckSubject()[%d] = %#v, want trace kind %q", index, got[index], want)
				}
			}
		})
	}
}

func TestReportClean(t *testing.T) {
	if !(Report{}).Clean() {
		t.Fatalf("empty report Clean() = false, want true")
	}
	if (Report{Traces: 1}).Clean() {
		t.Fatalf("trace report Clean() = true, want false")
	}
	if (Report{Gaps: []Gap{{Check: "x", Reason: "y"}}}).Clean() {
		t.Fatalf("gap report Clean() = true, want false")
	}
}

func hasFinding(findings []Finding, kind Kind, commit string) bool {
	for _, finding := range findings {
		if finding.Kind == kind && finding.Commit == commit {
			return true
		}
	}
	return false
}

func hasAnyKind(findings []Finding, kind Kind) bool {
	for _, finding := range findings {
		if finding.Kind == kind {
			return true
		}
	}
	return false
}

func kindForLine(line string) Kind {
	switch {
	case strings.Contains(line, emailAddress()):
		return KindAIEmail
	case strings.Contains(line, generatedWith()):
		return KindGeneratedWith
	default:
		return KindAIAuthor
	}
}
