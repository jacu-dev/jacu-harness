package context

import "strings"

func Anchors(spec Spec) []string {
	anchors := make([]string, 0, 1+len(spec.Acceptance)+len(spec.RequiredPaths)+len(spec.Verification))
	if spec.Objective != "" {
		anchors = append(anchors, "anchor:mission/objective")
	}
	for i, criterion := range spec.Acceptance {
		if strings.TrimSpace(criterion) == "" {
			continue
		}
		anchors = append(anchors, "anchor:mission/acceptance/"+itoa(i))
	}
	for _, path := range spec.RequiredPaths {
		if path != "" {
			anchors = append(anchors, "path:"+path)
		}
	}
	return unique(anchors)
}

func CheckAnchors(pack Pack) (lost int) {
	present := make(map[string]bool, len(pack.Items))
	for _, item := range pack.Items {
		present[item.ID] = true
		present[item.Path] = true
	}
	for _, anchor := range pack.Anchors {
		if present[anchor] {
			continue
		}
		lost++
	}
	return lost
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
