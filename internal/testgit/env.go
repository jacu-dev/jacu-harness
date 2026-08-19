// Package testgit provides hermetic environments for temporary Git fixtures.
package testgit

import (
	"os"
	"strings"
)

func Env() []string {
	base := os.Environ()
	result := make([]string, 0, len(base)+2)
	for _, entry := range base {
		if strings.HasPrefix(entry, "GIT_CONFIG_GLOBAL=") || strings.HasPrefix(entry, "GIT_CONFIG_NOSYSTEM=") {
			continue
		}
		result = append(result, entry)
	}
	return append(result, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
}
