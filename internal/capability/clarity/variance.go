package clarity

type Variance struct {
	Disagree bool
	Field    string
	Runs     int
}

func CompareRuns(readbacks []Readback) Variance {
	normalized := make([]Readback, 0, len(readbacks))
	for _, readback := range readbacks {
		normalized = append(normalized, Normalize(readback))
	}
	result := Variance{Runs: len(normalized)}
	if len(normalized) < 2 {
		return result
	}
	fields := []struct {
		name string
		pick func(Readback) []string
	}{
		{FieldWriteScope, func(readback Readback) []string { return readback.WriteScope }},
		{FieldForbidden, func(readback Readback) []string { return readback.ForbiddenPaths }},
		{FieldRequirements, func(readback Readback) []string { return readback.Requirements }},
		{FieldOutOfScope, func(readback Readback) []string { return readback.OutOfScope }},
		{FieldTasks, func(readback Readback) []string { return readback.Tasks }},
	}
	for _, field := range fields {
		first := field.pick(normalized[0])
		for _, readback := range normalized[1:] {
			if equalLists(first, field.pick(readback)) {
				continue
			}
			result.Disagree = true
			result.Field = field.name
			return result
		}
	}
	return result
}
