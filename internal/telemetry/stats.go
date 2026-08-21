package telemetry

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jacu-dev/jacu-harness/internal/gitx"
)

const (
	MetricFirstPassVerify      = "verify_first_pass_pct"
	MetricRemediation          = "remediation_iterations_per_mission"
	MetricEscalation           = "escalation_pct"
	MetricAutoApply            = "auto_apply_without_intervention_pct"
	MetricMissionApplyP50      = "mission_to_apply_p50_ms"
	MetricMissionApplyP95      = "mission_to_apply_p95_ms"
	MetricMissionsPerDay       = "missions_per_day"
	MetricRevertedApplyPct     = "apply_reverted_pct_heuristic"
	MetricMissionBytesIn       = "mission_bytes_in"
	MetricMissionBytesOut      = "mission_bytes_out"
	MetricMissionInterruptions = "mission_interruptions"
	MetricCleanExitResult      = "clean_exit_result"
)

type GitHistory struct{ Repo string }

func ReportStats(root string) Stats {
	events, err := NewStore().ReadSince(time.Unix(0, 0).UTC())
	if err != nil || len(events) == 0 {
		return noDataStats()
	}
	events = FilterProject(events, ProjectID(root))
	if len(events) == 0 {
		return noDataStats()
	}
	until := events[len(events)-1].Timestamp
	since := until.Add(-30 * 24 * time.Hour)
	var history *GitHistory
	if _, statErr := os.Stat(root + string(os.PathSeparator) + ".git"); statErr == nil {
		history = &GitHistory{Repo: root}
	}
	stats, err := ComputeStats(events, since, until, history)
	if err != nil {
		return noDataStats()
	}
	return stats
}

func noDataStats() Stats {
	available := map[string]bool{}
	for _, name := range metricNames() {
		available[name] = false
	}
	return Stats{ToolP95Ms: map[string]int64{}, Available: available, Modules: emptyModuleStats()}
}

type Stats struct {
	Since                 time.Time
	Until                 time.Time
	FirstPassVerifyPct    float64
	RemediationIterations float64
	EscalationPct         float64
	AutoApplyPct          float64
	MissionApplyP50Ms     int64
	MissionApplyP95Ms     int64
	MissionsPerDay        float64
	ToolP95Ms             map[string]int64
	RevertedApplyPct      float64
	RevertHeuristic       bool
	Available             map[string]bool
	Modules               map[string]ModuleStats
	MissionBytesIn        int64
	MissionBytesOut       int64
}

type ModuleStats struct {
	Events      int
	InputBytes  int64
	OutputBytes int64
	Measurement string
}

// FilterProject keeps the shared local store from mixing independent
// repositories in one project's diagnosis.
func FilterProject(events []Event, projectID string) []Event {
	filtered := make([]Event, 0, len(events))
	for _, event := range events {
		if event.ProjectID == projectID {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func ComputeStats(events []Event, since, until time.Time, history *GitHistory) (Stats, error) {
	stats := Stats{
		Since: since.UTC(), Until: until.UTC(), ToolP95Ms: map[string]int64{},
		Available: map[string]bool{}, Modules: emptyModuleStats(),
	}
	filtered := make([]Event, 0, len(events))
	for _, event := range events {
		if event.Timestamp.Before(since) || event.Timestamp.After(until) {
			continue
		}
		filtered = append(filtered, event)
	}
	if len(filtered) == 0 {
		for _, name := range metricNames() {
			stats.Available[name] = false
		}
		return stats, nil
	}

	verifyTotal, verifyPass := 0, 0
	remediationByMission := map[string]int{}
	escalatedMissions := map[string]struct{}{}
	missions := map[string]struct{}{}
	totalApplies, autoApplies := 0, 0
	starts := map[string]time.Time{}
	applyDurations := []int64{}
	toolDurations := map[string][]int64{}
	missionDays := map[string]struct{}{}
	for _, event := range filtered {
		stats.MissionBytesIn += event.InputBytes
		stats.MissionBytesOut += event.OutputBytes
		module := event.Module
		if module == "" {
			module = NoData
		}
		moduleStats := stats.Modules[module]
		moduleStats.Events++
		moduleStats.InputBytes += event.InputBytes
		moduleStats.OutputBytes += event.OutputBytes
		if event.Measurement != "" && event.Measurement != NoData {
			moduleStats.Measurement = event.Measurement
		}
		stats.Modules[module] = moduleStats
		missionKey := event.MissionID
		if missionKey == "" {
			missionKey = event.RunID
		}
		if missionKey != "" {
			missions[missionKey] = struct{}{}
			missionDays[event.Timestamp.UTC().Format("2006-01-02")] = struct{}{}
		}
		switch event.Event {
		case EventToolCall:
			if event.Tool != "" {
				toolDurations[event.Tool] = append(toolDurations[event.Tool], event.DurationMs)
			}
			if event.Tool == "jacu_mission_compile" && missionKey != "" {
				if old, ok := starts[missionKey]; !ok || event.Timestamp.Before(old) {
					starts[missionKey] = event.Timestamp
				}
			}
		case EventVerify:
			if event.Iteration <= 1 {
				verifyTotal++
				if event.Verdict == "pass" {
					verifyPass++
				}
			}
		case EventRemediation:
			if missionKey != "" {
				if event.Iteration > remediationByMission[missionKey] {
					remediationByMission[missionKey] = event.Iteration
				}
			}
		case EventEscalation:
			if missionKey != "" {
				escalatedMissions[missionKey] = struct{}{}
			}
		case EventApply:
			totalApplies++
			if event.Tool == "autonomy" {
				autoApplies++
			}
			if start, ok := starts[missionKey]; ok && !event.Timestamp.Before(start) {
				applyDurations = append(applyDurations, event.Timestamp.Sub(start).Milliseconds())
			}
		}
	}
	if verifyTotal > 0 {
		stats.FirstPassVerifyPct = percentage(verifyPass, verifyTotal)
		stats.Available[MetricFirstPassVerify] = true
	}
	if len(remediationByMission) > 0 {
		total, count := 0, 0
		for _, iteration := range remediationByMission {
			total += iteration
			count++
		}
		stats.RemediationIterations = float64(total) / float64(count)
		stats.Available[MetricRemediation] = true
	}
	if len(missions) > 0 {
		stats.EscalationPct = percentage(len(escalatedMissions), len(missions))
		stats.Available[MetricEscalation] = true
		if len(missionDays) > 0 {
			stats.MissionsPerDay = float64(len(missions)) / float64(len(missionDays))
			stats.Available[MetricMissionsPerDay] = true
		}
	}
	if totalApplies > 0 {
		stats.AutoApplyPct = percentage(autoApplies, totalApplies)
		stats.Available[MetricAutoApply] = true
	}
	if len(applyDurations) > 0 {
		stats.MissionApplyP50Ms = percentile(applyDurations, 0.50)
		stats.MissionApplyP95Ms = percentile(applyDurations, 0.95)
		stats.Available[MetricMissionApplyP50] = true
		stats.Available[MetricMissionApplyP95] = true
	}
	for tool, durations := range toolDurations {
		stats.ToolP95Ms[tool] = percentile(durations, 0.95)
	}
	if history != nil && history.Repo != "" {
		reverted, err := history.revertedApplies(since, until)
		if err != nil {
			return stats, nil
		}
		if totalApplies > 0 {
			stats.RevertedApplyPct = percentage(reverted, totalApplies)
			stats.Available[MetricRevertedApplyPct] = true
			stats.RevertHeuristic = true
		}
	}
	if stats.MissionBytesIn > 0 {
		stats.Available[MetricMissionBytesIn] = true
	}
	if stats.MissionBytesOut > 0 {
		stats.Available[MetricMissionBytesOut] = true
	}
	return stats, nil
}

func metricNames() []string {
	return []string{MetricFirstPassVerify, MetricRemediation, MetricEscalation, MetricAutoApply, MetricMissionApplyP50, MetricMissionApplyP95, MetricMissionsPerDay, MetricRevertedApplyPct, MetricMissionBytesIn, MetricMissionBytesOut, MetricMissionInterruptions, MetricCleanExitResult}
}

func emptyModuleStats() map[string]ModuleStats {
	modules := make(map[string]ModuleStats, len(allowedModules)+1)
	for module := range allowedModules {
		if module != NoData {
			modules[module] = ModuleStats{}
		}
	}
	modules["guardrails"] = ModuleStats{}
	return modules
}

func percentage(numerator, denominator int) float64 {
	return math.Round(float64(numerator)*1000/float64(denominator)) / 10
}

func percentile(values []int64, fraction float64) int64 {
	sorted := append([]int64{}, values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := int(math.Ceil(float64(len(sorted))*fraction)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func (history GitHistory) revertedApplies(since, until time.Time) (int, error) {
	git, err := gitx.New()
	if err != nil {
		return 0, fmt.Errorf("read git revert history: %w", err)
	}
	output, err := git.OutputRaw(context.Background(), history.Repo, "log",
		"--since="+since.UTC().Format(time.RFC3339),
		"--until="+until.UTC().Format(time.RFC3339),
		"--format=%aI%x00%B%x00")
	if err != nil {
		return 0, fmt.Errorf("read git revert history: %w", err)
	}
	count := 0
	for _, record := range bytes.Split([]byte(output), []byte{0}) {
		message := string(record)
		if strings.Contains(strings.ToLower(message), "revert") && strings.Contains(message, "Jacu-Run:") {
			count++
		}
	}
	return count, nil
}

func FormatStats(stats Stats) string {
	var out strings.Builder
	fmt.Fprintf(&out, "telemetry_window: %s..%s\n", stats.Since.Format(time.RFC3339), stats.Until.Format(time.RFC3339))
	writeMetric := func(name string, value any) {
		if !stats.Available[name] {
			fmt.Fprintf(&out, "%s: no-data\n", name)
			return
		}
		fmt.Fprintf(&out, "%s: %v\n", name, value)
	}
	writeMetric(MetricFirstPassVerify, fmt.Sprintf("%.1f%%", stats.FirstPassVerifyPct))
	writeMetric(MetricRemediation, fmt.Sprintf("%.1f", stats.RemediationIterations))
	writeMetric(MetricEscalation, fmt.Sprintf("%.1f%%", stats.EscalationPct))
	writeMetric(MetricAutoApply, fmt.Sprintf("%.1f%%", stats.AutoApplyPct))
	writeMetric(MetricMissionApplyP50, stats.MissionApplyP50Ms)
	writeMetric(MetricMissionApplyP95, stats.MissionApplyP95Ms)
	writeMetric(MetricMissionsPerDay, fmt.Sprintf("%.1f", stats.MissionsPerDay))
	writeMetric(MetricRevertedApplyPct, fmt.Sprintf("%.1f%% (heuristic)", stats.RevertedApplyPct))
	tools := make([]string, 0, len(stats.ToolP95Ms))
	for tool := range stats.ToolP95Ms {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	for _, tool := range tools {
		fmt.Fprintf(&out, "tool_p95_ms.%s: %d\n", tool, stats.ToolP95Ms[tool])
	}
	return out.String()
}

func FormatFullStats(stats Stats) string {
	var out strings.Builder
	fmt.Fprintf(&out, "telemetry_window: %s..%s\n", stats.Since.Format(time.RFC3339), stats.Until.Format(time.RFC3339))
	modules := make([]string, 0, len(stats.Modules))
	for module := range stats.Modules {
		modules = append(modules, module)
	}
	sort.Strings(modules)
	for _, module := range modules {
		section := stats.Modules[module]
		fmt.Fprintf(&out, "\n%s\n", strings.ToUpper(module))
		if section.Events == 0 {
			fmt.Fprintln(&out, "  no-data")
			continue
		}
		fmt.Fprintf(&out, "  events: %d\n", section.Events)
		if section.InputBytes == 0 && section.OutputBytes == 0 {
			fmt.Fprintln(&out, "  cost: no-data")
			continue
		}
		fmt.Fprintf(&out, "  bytes: input=%d output=%d measurement=%s\n", section.InputBytes, section.OutputBytes, section.Measurement)
	}
	return out.String()
}
