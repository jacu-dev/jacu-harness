package sdd

import "testing"

func TestValidateTasksReportsRequiredTaskFindings(t *testing.T) {
	tasks := []Task{
		{Number: "T1", Verify: "", Status: "todo"},
		{Number: "T2", Verify: "go test ./...", Status: "doing"},
		{Number: "T3", Verify: "go test ./...", Status: "doing"},
		{Number: "T4", Verify: "go test ./...", Status: "done", Evidence: ""},
		{Number: "T5", Verify: "go test ./...", Status: "todo", Task: "GREEN: implement parser"},
	}
	findings := validateTasks(tasks)
	for _, code := range []string{"sdd_task_without_verify", "sdd_two_tasks_in_flight", "sdd_task_done_without_evidence", "sdd_task_without_red"} {
		if !hasFindingCode(findings, code) {
			t.Errorf("missing finding %q in %#v", code, findings)
		}
	}
}

func hasFindingCode(findings []Finding, code string) bool {
	for _, finding := range findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
