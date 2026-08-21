package clarity

import (
	"strings"

	"github.com/jacu-dev/jacu-harness/internal/capability/sdd"
)

func Expected(document sdd.Document) Readback {
	allowed, forbidden := sdd.WriteScope(document)
	requirements := make([]string, 0, len(document.Requirements))
	for _, requirement := range document.Requirements {
		requirements = append(requirements, requirement.Name)
	}
	tasks := sdd.Tasks(document)
	numbers := make([]string, 0, len(tasks))
	for _, task := range tasks {
		numbers = append(numbers, task.Number)
	}
	return Normalize(Readback{
		WriteScope:     allowed,
		ForbiddenPaths: forbidden,
		Requirements:   requirements,
		OutOfScope:     sectionItems(document, "Out of scope"),
		Tasks:          numbers,
	})
}

func sectionItems(document sdd.Document, name string) []string {
	items := make([]string, 0)
	for _, section := range document.Sections {
		if !strings.EqualFold(section.Name, name) {
			continue
		}
		for _, line := range section.Lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "```") {
				continue
			}
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
			if trimmed == "" {
				continue
			}
			items = append(items, trimmed)
		}
	}
	return items
}
