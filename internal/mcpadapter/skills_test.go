package mcpadapter

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A skill ships in the same PR as the capability it teaches — regra 4 of
// docs/plano/skills-arquitetura.md. Nothing in the build enforces that: the
// skills are markdown the host loads, and the tool surface is Go. So a tool can
// reach every host with no skill saying when to call it, and a skill can keep
// naming a tool that was renamed or never existed. Both failures are silent in
// every test we had, and both are invisible until a host session goes wrong.
// These tests are the missing link between the two halves.

const (
	routerSkill = "using-jacu"
	// The binary's own name reads exactly like a skill reference and is not one.
	binaryName = "jacu"
	// regra 5: over budget means the skill is teaching too much and must split.
	maxSkillLines  = 100
	maxRouterLines = 40
)

// Tools are jacu_snake_case and skills are jacu-kebab-case, which is what makes
// a plain scan of the prose reliable here.
var (
	toolMention  = regexp.MustCompile(`jacu_[a-z_]+`)
	skillMention = regexp.MustCompile(`jacu-[a-z]+`)
)

type skillDoc struct {
	dir         string
	path        string
	body        string
	lines       int
	frontName   string
	description string
}

func TestEverySkillNamesOnlyToolsAndSkillsThatExist(t *testing.T) {
	skills := loadSkills(t)
	tools := serverToolNames(t)

	for _, skill := range sortedSkills(skills) {
		for _, tool := range unique(toolMention.FindAllString(skill.body, -1)) {
			if !tools[tool] {
				t.Errorf("%s names %s, which the server does not expose", skill.path, tool)
			}
		}
		for _, named := range unique(skillMention.FindAllString(skill.body, -1)) {
			if named == binaryName || named == skill.dir {
				continue
			}
			if _, ok := skills[named]; !ok {
				t.Errorf("%s routes to skill %s, which is not shipped", skill.path, named)
			}
		}
	}
}

func TestEveryToolIsTaughtBySomeSkill(t *testing.T) {
	skills := loadSkills(t)

	taught := make(map[string]string)
	for _, skill := range sortedSkills(skills) {
		for _, tool := range unique(toolMention.FindAllString(skill.body, -1)) {
			if _, seen := taught[tool]; !seen {
				taught[tool] = skill.dir
			}
		}
	}

	for tool := range serverToolNames(t) {
		if taught[tool] == "" {
			t.Errorf("no skill teaches %s: a tool no skill mentions is a tool no host calls", tool)
		}
	}
}

func TestRouterRoutesToEveryCapabilitySkill(t *testing.T) {
	skills := loadSkills(t)
	router, ok := skills[routerSkill]
	if !ok {
		t.Fatalf("router skill %s not found", routerSkill)
	}

	routed := make(map[string]bool)
	for _, named := range skillMention.FindAllString(router.body, -1) {
		routed[named] = true
	}

	for _, skill := range sortedSkills(skills) {
		if skill.dir == routerSkill {
			continue
		}
		if !routed[skill.dir] {
			t.Errorf("%s is shipped but the router never names it, so no host reaches it", skill.dir)
		}
	}
}

func TestSkillFrontmatterMatchesItsDirectory(t *testing.T) {
	for _, skill := range sortedSkills(loadSkills(t)) {
		// The host keys the skill by the frontmatter name, not by the folder.
		if skill.frontName != skill.dir {
			t.Errorf("%s declares name %q; the directory is %q", skill.path, skill.frontName, skill.dir)
		}
		if skill.description == "" {
			// The description is the whole trigger: without it the host has no
			// rule for when to load the skill.
			t.Errorf("%s has no description", skill.path)
		}
	}
}

func TestSkillsStayWithinTheirSizeBudget(t *testing.T) {
	for _, skill := range sortedSkills(loadSkills(t)) {
		budget := maxSkillLines
		if skill.dir == routerSkill {
			budget = maxRouterLines
		}
		if skill.lines > budget {
			t.Errorf("%s has %d lines, budget is %d — it is teaching too much", skill.path, skill.lines, budget)
		}
	}
}

func loadSkills(t *testing.T) map[string]skillDoc {
	t.Helper()

	// Rooted at the skills directory rather than joining paths against it: the
	// read cannot leave the tree it is meant to inspect, which is also why gosec
	// has nothing to warn about here.
	skillsDir := os.DirFS(filepath.Join("..", "..", "skills"))
	entries, err := fs.ReadDir(skillsDir, ".")
	if err != nil {
		t.Fatalf("read skills dir: %v", err)
	}

	skills := make(map[string]skillDoc, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := entry.Name() + "/SKILL.md"
		raw, readErr := fs.ReadFile(skillsDir, path)
		if readErr != nil {
			t.Fatalf("read skills/%s: %v", path, readErr)
		}
		body := string(raw)
		name, description := parseFrontmatter(body)
		skills[entry.Name()] = skillDoc{
			dir:         entry.Name(),
			path:        "skills/" + path,
			body:        body,
			lines:       strings.Count(strings.TrimRight(body, "\n"), "\n") + 1,
			frontName:   name,
			description: description,
		}
	}
	if len(skills) == 0 {
		t.Fatal("no skills found")
	}
	return skills
}

func parseFrontmatter(body string) (name, description string) {
	lines := strings.Split(body, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", ""
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `'"`)
		switch strings.TrimSpace(key) {
		case "name":
			name = value
		case "description":
			description = value
		}
	}
	return name, description
}

func serverToolNames(t *testing.T) map[string]bool {
	t.Helper()

	srv := NewServer("test", t.TempDir())
	session, ctx := connectMCPTestServer(t, srv, "skills-test")
	listed, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	names := make(map[string]bool, len(listed.Tools))
	for _, tool := range listed.Tools {
		names[tool.Name] = true
	}
	return names
}

func sortedSkills(skills map[string]skillDoc) []skillDoc {
	out := make([]skillDoc, 0, len(skills))
	for _, skill := range skills {
		out = append(out, skill)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].dir < out[j].dir })
	return out
}

func unique(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
