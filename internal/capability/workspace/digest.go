package workspace

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

func canonicalizeDiff(patch string) string {
	var canonical strings.Builder
	canonical.Grow(len(patch))
	for _, line := range strings.SplitAfter(patch, "\n") {
		if strings.HasPrefix(line, "index ") {
			continue
		}
		canonical.WriteString(line)
	}
	return canonical.String()
}

func diffDigest(patch string) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(canonicalizeDiff(patch))))
}
