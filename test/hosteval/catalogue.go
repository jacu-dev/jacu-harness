package hosteval

import "fmt"

// ToolDesc is one advertised or observed MCP tool description.
type ToolDesc struct {
	Name        string
	Description string
}

// CompareToolCatalogue asserts every advertised tool description reaches the
// host non-empty and non-truncated. A shortened description fails naming the
// tool and the observed length.
func CompareToolCatalogue(advertised, observed []ToolDesc) error {
	got := make(map[string]ToolDesc, len(observed))
	for _, tool := range observed {
		got[tool.Name] = tool
	}
	for _, want := range advertised {
		if want.Name == "" {
			return fmt.Errorf("advertised catalogue contains a tool with an empty name")
		}
		if want.Description == "" {
			return fmt.Errorf("advertised tool %s has an empty description", want.Name)
		}
		seen, ok := got[want.Name]
		if !ok {
			return fmt.Errorf("host omitted tool %s", want.Name)
		}
		if seen.Description == "" {
			return fmt.Errorf("tool %s description is empty; observed length 0", want.Name)
		}
		if len(seen.Description) < len(want.Description) {
			return fmt.Errorf("tool %s description truncated: advertised %d bytes, observed %d", want.Name, len(want.Description), len(seen.Description))
		}
		if seen.Description != want.Description {
			return fmt.Errorf("tool %s description changed: advertised %d bytes, observed %d", want.Name, len(want.Description), len(seen.Description))
		}
	}
	return nil
}
