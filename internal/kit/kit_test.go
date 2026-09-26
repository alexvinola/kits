package kit

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoad(t *testing.T) {
	fsys := fstest.MapFS{
		"core/instructions.md":             {Data: []byte("hi\n")},
		"core/README.md":                   {Data: []byte("ignored")},
		"core/skills/b/SKILL.md":           {Data: []byte("b")},
		"core/skills/a/SKILL.md":           {Data: []byte("a")},
		"core/skills/a/scripts/run.sh":     {Data: []byte("#!/bin/sh"), Mode: 0o755},
		"core/skills/a/reference/notes.md": {Data: []byte("n")},
	}
	k, err := Load(fsys, "core")
	if err != nil {
		t.Fatal(err)
	}
	if string(k.Instructions) != "hi\n" {
		t.Errorf("instructions = %q", k.Instructions)
	}
	if len(k.Skills) != 2 || k.Skills[0].Name != "a" || k.Skills[1].Name != "b" {
		t.Fatalf("skills = %+v", k.Skills)
	}
	var paths []string
	for _, f := range k.Skills[0].Files {
		paths = append(paths, f.Path)
		if f.Path == "scripts/run.sh" && !f.Exec {
			t.Error("run.sh lost its exec bit")
		}
	}
	if got := strings.Join(paths, ","); got != "SKILL.md,reference/notes.md,scripts/run.sh" {
		t.Errorf("files = %s", got)
	}
}

func TestLoadRejects(t *testing.T) {
	cases := []struct {
		name string
		fsys fstest.MapFS
		kit  string
		want string
	}{
		{"missing", fstest.MapFS{"x/instructions.md": {}}, "core", "not found"},
		{"bad name", fstest.MapFS{}, "Core", "invalid kit name"},
		{"empty kit", fstest.MapFS{"core/README.md": {}}, "core", "has none of"},
		{"agent not md", fstest.MapFS{"core/agents/rev.txt": {}}, "core", "want <name>.md"},
		{"agent subdir", fstest.MapFS{"core/agents/x/rev.md": {}}, "core", "only regular .md files"},
		{"empty skill", fstest.MapFS{"core/skills/a": {Mode: fs.ModeDir}}, "core", "empty skill"},
		{"bad skill name", fstest.MapFS{"core/skills/A_b/SKILL.md": {}}, "core", "invalid skill name"},
		{"file in skills", fstest.MapFS{"core/skills/x.md": {}}, "core", "expected a skill directory"},
		{"symlink", fstest.MapFS{"core/skills/a/SKILL.md": {Mode: fs.ModeSymlink}}, "core", "regular files"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(tc.fsys, tc.kit)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want mention of %q", err, tc.want)
			}
		})
	}
}

func TestLoadAgents(t *testing.T) {
	fsys := fstest.MapFS{
		"core/agents/reviewer.md": {Data: []byte("---\nname: reviewer\n---\n")},
		"core/mcp.json":           {Data: []byte(`{"ignored": true}`)},
	}
	k, err := Load(fsys, "core")
	if err != nil {
		t.Fatal(err)
	}
	if len(k.Agents) != 1 || k.Agents[0].Name != "reviewer" {
		t.Fatalf("agents = %+v", k.Agents)
	}
}
