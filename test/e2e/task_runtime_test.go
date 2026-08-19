//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTaskRuntimeStartsPollsReturnsAndCancels(t *testing.T) {
	project := newProjectRepo(t)
	if err := os.MkdirAll(filepath.Join(project, ".jacu"), 0o700); err != nil {
		t.Fatalf("mkdir project policy: %v", err)
	}
	writeFile(t, filepath.Join(project, ".jacu", "verify-allowlist.json"), `{"allow":[{"program":"printf"},{"program":"sleep"}]}`)
	git(t, project, "add", ".jacu/verify-allowlist.json")
	git(t, project, "commit", "-m", "authorize verification fixture")

	s := startSession(t, project)
	s.useSessionlessProtocol("jacu-task-runtime-e2e")
	inspect := s.callTool("jacu_project_inspect", map[string]any{})
	projectID, _ := s.data("jacu_project_inspect", inspect)["project_id"].(string)
	if projectID == "" {
		t.Fatal("inspect returned no project_id")
	}

	quickMission := map[string]any{
		"objective":             "Run a quick verification",
		"context":               map[string]any{"project_id": projectID},
		"acceptance_criteria":   []string{"The check completes"},
		"verification_commands": [][]string{{"printf", "done"}},
		"risk_hint":             "safe",
	}
	quickCompile := s.callTool("jacu_mission_compile", quickMission)
	quickMissionID, _ := s.data("jacu_mission_compile", quickCompile)["mission_id"].(string)
	quickOpen := s.callTool("jacu_workspace_open", map[string]any{
		"mission_input": quickMission,
		"mission_id":    quickMissionID,
	})
	quickRunID, _ := s.data("jacu_workspace_open", quickOpen)["run_id"].(string)
	quickStart := s.callTool("jacu_verify", map[string]any{"run_id": quickRunID, "async": true})
	if quickStart.Status != "accepted" {
		t.Fatalf("async start status = %q (%s); want accepted", quickStart.Status, quickStart.Summary)
	}
	quickTaskID := taskIDFromData(t, s.data("jacu_verify", quickStart))
	quickDone := waitForTaskStatus(t, s, quickTaskID, "done")
	quickResult, _ := quickDone["result"].(map[string]any)
	resultData, _ := quickResult["data"].(map[string]any)
	if resultData["verdict"] != "pass" {
		t.Fatalf("async result = %#v; want verify verdict pass", quickDone)
	}

	longMission := map[string]any{
		"objective":             "Run a cancellable verification",
		"context":               map[string]any{"project_id": projectID},
		"acceptance_criteria":   []string{"The check can be cancelled"},
		"verification_commands": [][]string{{"sleep", "5"}},
		"risk_hint":             "safe",
	}
	longCompile := s.callTool("jacu_mission_compile", longMission)
	longMissionID, _ := s.data("jacu_mission_compile", longCompile)["mission_id"].(string)
	longOpen := s.callTool("jacu_workspace_open", map[string]any{
		"mission_input": longMission,
		"mission_id":    longMissionID,
	})
	longRunID, _ := s.data("jacu_workspace_open", longOpen)["run_id"].(string)
	longStart := s.callTool("jacu_verify", map[string]any{"run_id": longRunID, "async": true})
	longTaskID := taskIDFromData(t, s.data("jacu_verify", longStart))
	cancel := s.callTool("jacu_verify", map[string]any{"task_id": longTaskID, "cancel": true})
	if cancel.Status != "accepted" {
		t.Fatalf("cancel status = %q (%s); want accepted", cancel.Status, cancel.Summary)
	}
	cancelled := waitForTaskStatus(t, s, longTaskID, "cancelled")
	if cancelled["result"] == nil {
		t.Fatalf("cancelled task has no bounded result: %#v", cancelled)
	}
}

func TestFlowRuntimeRunsDeterministicIndependentMissionWave(t *testing.T) {
	project := newProjectRepo(t)
	s := startSession(t, project)
	s.useSessionlessProtocol("jacu-flow-e2e")
	start := s.callTool("jacu_flow_run", map[string]any{
		"async": true,
		"flow": map[string]any{
			"nodes": []map[string]any{
				{"id": "docs", "uses": "mission", "allowed_paths": []string{"docs"}, "with": map[string]any{
					"objective": "Update the docs example now", "allowed_paths": []string{"docs"},
					"acceptance_criteria": []string{"The docs example is clear"},
				}},
				{"id": "readme", "uses": "mission", "allowed_paths": []string{"README.md"}, "with": map[string]any{
					"objective": "Update the README example now", "allowed_paths": []string{"README.md"},
					"acceptance_criteria": []string{"The README example is clear"},
				}},
			},
			"edges": []map[string]any{},
		},
	})
	if start.Status != "accepted" {
		t.Fatalf("flow start status = %q (%s); want accepted", start.Status, start.Summary)
	}
	taskID := taskIDFromData(t, s.data("jacu_flow_run", start))
	done := waitForTaskStatus(t, s, taskID, "done")
	result, _ := done["result"].(map[string]any)
	if result["status"] != "ok" {
		t.Fatalf("flow result = %#v; want status ok", result)
	}
	waves, _ := result["waves"].([]any)
	if len(waves) != 1 {
		t.Fatalf("flow waves = %#v; want one independent wave", result["waves"])
	}
	trace, _ := result["trace"].([]any)
	if len(trace) != 2 {
		t.Fatalf("flow trace = %#v; want two nodes", result["trace"])
	}
}

func taskIDFromData(t *testing.T, data map[string]any) string {
	t.Helper()
	task, ok := data["task"].(map[string]any)
	if !ok {
		t.Fatalf("task metadata = %#v; want object", data["task"])
	}
	taskID, _ := task["task_id"].(string)
	if taskID == "" {
		t.Fatalf("task metadata = %#v; want task_id", task)
	}
	return taskID
}

func waitForTaskStatus(t *testing.T, s *session, taskID, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		status := s.callTool("jacu_status", map[string]any{"task_id": taskID})
		if status.Status != "ok" {
			t.Fatalf("status for %s = %q (%s)", taskID, status.Status, status.Summary)
		}
		tasks, _ := s.data("jacu_status", status)["tasks"].([]any)
		if len(tasks) != 1 {
			t.Fatalf("tasks for %s = %#v; want one", taskID, tasks)
		}
		task, _ := tasks[0].(map[string]any)
		if task["status"] == want {
			return task
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("task %s did not reach %s", taskID, want)
	return nil
}
