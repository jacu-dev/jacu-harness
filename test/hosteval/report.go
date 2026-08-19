//go:build hosteval

package hosteval

import (
	"fmt"
	"strings"
)

// Report renders results as the markdown row set that goes into
// scripts/host-smoke/README.md.
func Report(results []Result) string {
	var b strings.Builder
	b.WriteString("| Caso | Host | Veredito | Observado | Nota |\n|---|---|---|---|---|\n")
	for _, r := range results {
		note := r.Reason
		if len(r.Failures) > 0 {
			note = strings.Join(r.Failures, "; ")
		}
		if r.Truncated {
			note = strings.TrimSpace(note + " [host avisou truncamento de description]")
		}
		if r.SkippedLines > 0 {
			note = strings.TrimSpace(fmt.Sprintf("%s [%d linha(s) ilegível(is) no stream]", note, r.SkippedLines))
		}
		if note == "" {
			note = "—"
		}
		_, _ = fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			r.Case, r.Host, r.Verdict, render(r.Tools), note)
	}
	return b.String()
}
