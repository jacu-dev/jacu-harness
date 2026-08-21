package sdd

import "strings"

type Task struct {
	Number   string
	Task     string
	Files    string
	Verify   string
	Status   string
	Evidence string
}

type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Target   string `json:"target"`
	Message  string `json:"message"`
}

const (
	SeverityBlock = "BLOCK"
	SeverityWarn  = "WARN"
	SeverityInfo  = "INFO"
)

// Tasks extracts the task table from a parsed SDD.
func Tasks(document Document) []Task {
	for _, section := range document.Sections {
		if !strings.EqualFold(section.Name, "Tasks") {
			continue
		}
		var tasks []Task
		for _, line := range section.Lines {
			cells, ok := markdownTableCells(line)
			if !ok || len(cells) < 6 || cells[0] == "#" || strings.HasPrefix(cells[0], "-") {
				continue
			}
			tasks = append(tasks, Task{
				Number:   cells[0],
				Task:     cells[1],
				Files:    cells[2],
				Verify:   cells[3],
				Status:   strings.ToLower(cells[4]),
				Evidence: cells[5],
			})
		}
		return tasks
	}
	return nil
}

func validateTasks(tasks []Task) []Finding {
	findings := make([]Finding, 0)
	doing := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		target := task.Number
		if strings.TrimSpace(task.Verify) == "" {
			findings = append(findings, Finding{Code: "sdd_task_without_verify", Severity: SeverityBlock, Target: target, Message: "task has no Verify command"})
		}
		if strings.EqualFold(strings.TrimSpace(task.Status), "done") && strings.TrimSpace(task.Evidence) == "" {
			findings = append(findings, Finding{Code: "sdd_task_done_without_evidence", Severity: SeverityBlock, Target: target, Message: "done task has no evidence"})
		}
		if strings.EqualFold(strings.TrimSpace(task.Status), "doing") {
			doing = append(doing, task)
		}
		if isImplementationTask(task.Task) && !hasPriorRedTask(tasks, task.Number) {
			findings = append(findings, Finding{Code: "sdd_task_without_red", Severity: SeverityWarn, Target: target, Message: "implementation task has no prior RED task"})
		}
	}
	if len(doing) > 1 {
		findings = append(findings, Finding{Code: "sdd_two_tasks_in_flight", Severity: SeverityBlock, Target: doing[1].Number, Message: "more than one task is doing"})
	}
	return findings
}

func isImplementationTask(task string) bool {
	task = strings.ToLower(task)
	return strings.Contains(task, "green") || strings.Contains(task, "implement")
}

func hasPriorRedTask(tasks []Task, number string) bool {
	for _, task := range tasks {
		if task.Number == number {
			return false
		}
		if strings.Contains(strings.ToLower(task.Task), "red") {
			return true
		}
	}
	return false
}

func markdownTableCells(line string) ([]string, bool) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 2 || trimmed[0] != '|' || trimmed[len(trimmed)-1] != '|' {
		return nil, false
	}
	parts := strings.Split(trimmed[1:len(trimmed)-1], "|")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts, true
}
