package targets

import (
	"strings"
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	cfg, err := Default()
	if err != nil {
		t.Fatalf("embedded default: %v", err)
	}
	for _, name := range []string{"claude", "cursor", "copilot", "agents"} {
		if _, ok := cfg.Get(name); !ok {
			t.Errorf("default config lacks target %q", name)
		}
	}
}

func TestParseRejects(t *testing.T) {
	cases := []struct {
		name, json, want string
	}{
		{"wrong version", `{"version":2,"targets":[{"name":"a","root_file":"A.md"}]}`, "version"},
		{"no targets", `{"version":1,"targets":[]}`, "at least one target"},
		{"unknown field", `{"version":1,"targets":[{"name":"a","root_file":"A.md","skill_dir":"x/{skill}"}]}`, "unknown field"},
		{"trailing data", `{"version":1,"targets":[{"name":"a","root_file":"A.md"}]} {}`, "trailing data"},
		{"bad name", `{"version":1,"targets":[{"name":"Claude","root_file":"A.md"}]}`, "name must match"},
		{"duplicate", `{"version":1,"targets":[{"name":"a","root_file":"A.md"},{"name":"a","root_file":"B.md"}]}`, "duplicate"},
		{"missing root", `{"version":1,"targets":[{"name":"a"}]}`, "root_file is required"},
		{"absolute root", `{"version":1,"targets":[{"name":"a","root_file":"/etc/A.md"}]}`, "relative"},
		{"windows root", `{"version":1,"targets":[{"name":"a","root_file":"C:/A.md"}]}`, "relative"},
		{"backslash", `{"version":1,"targets":[{"name":"a","root_file":"docs\\A.md"}]}`, "forward slashes"},
		{"escape", `{"version":1,"targets":[{"name":"a","root_file":"../A.md"}]}`, "leave the project"},
		{"unclean", `{"version":1,"targets":[{"name":"a","root_file":"./A.md"}]}`, "clean path"},
		{"placeholder in root", `{"version":1,"targets":[{"name":"a","root_file":"{kit}.md"}]}`, "placeholders are not allowed"},
		{"no skill placeholder", `{"version":1,"targets":[{"name":"a","root_file":"A.md","skills_dir":".a/skills"}]}`, "must contain {skill}"},
		{"no agent placeholder", `{"version":1,"targets":[{"name":"a","root_file":"A.md","agents_dir":".a/agents/x.md"}]}`, "must contain {agent}"},
		{"skill placeholder in agents", `{"version":1,"targets":[{"name":"a","root_file":"A.md","agents_dir":".a/{skill}/{agent}.md"}]}`, "unknown placeholder"},
		{"unknown placeholder", `{"version":1,"targets":[{"name":"a","root_file":"A.md","skills_dir":".a/{name}/{skill}"}]}`, "unknown placeholder"},
		{"unbalanced", `{"version":1,"targets":[{"name":"a","root_file":"A.md","skills_dir":".a/{skill}}"}]}`, "unbalanced"},
		{"no fixed prefix", `{"version":1,"targets":[{"name":"a","root_file":"A.md","skills_dir":"{skill}"}]}`, "fixed directory"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.json))
			if err == nil {
				t.Fatal("want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestValidateReportsAllErrors(t *testing.T) {
	_, err := Parse([]byte(`{"version":1,"targets":[{"name":"a"},{"name":"b","root_file":"../B.md"}]}`))
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{`target "a"`, `target "b"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestSkillPaths(t *testing.T) {
	cases := []struct {
		dir, root, path string
	}{
		{".claude/skills/{skill}", ".claude/skills", ".claude/skills/review"},
		{".x/{kit}/skills/{skill}", ".x", ".x/core/skills/review"},
		{".x/skills/{kit}-{skill}", ".x/skills", ".x/skills/core-review"},
		{"", "", ""},
	}
	for _, tc := range cases {
		tg := Target{SkillsDir: tc.dir}
		if got := tg.SkillsRoot(); got != tc.root {
			t.Errorf("SkillsRoot(%q) = %q, want %q", tc.dir, got, tc.root)
		}
		if got := tg.SkillPath("core", "review"); got != tc.path {
			t.Errorf("SkillPath(%q) = %q, want %q", tc.dir, got, tc.path)
		}
	}
}

func TestAgentPaths(t *testing.T) {
	tg := Target{AgentsDir: ".claude/agents/{agent}.md"}
	if got := tg.AgentsRoot(); got != ".claude/agents" {
		t.Errorf("AgentsRoot = %q", got)
	}
	if got := tg.AgentPath("core", "reviewer"); got != ".claude/agents/reviewer.md" {
		t.Errorf("AgentPath = %q", got)
	}
	if got := (Target{}).AgentPath("core", "reviewer"); got != "" {
		t.Errorf("AgentPath without agents_dir = %q", got)
	}
}
