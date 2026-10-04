package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitTargetLayoutsAndUpdates(t *testing.T) {
	agent := "---\nname: reviewer\ndescription: Review changes\ntools: Read, Grep, Glob, Bash\n---\n\nUse the review skill.\n"
	skill := "---\nname: review\ndescription: Review changes\n---\n\nCheck behaviour.\n"
	for _, tc := range []struct {
		name, root, skills, agent string
	}{
		{"claude", "CLAUDE.md", ".claude/skills", ".claude/agents/reviewer.md"},
		{"copilot", ".github/copilot-instructions.md", ".github/skills", ".github/agents/reviewer.agent.md"},
		{"codex", "AGENTS.md", ".agents/skills", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			from, project := t.TempDir(), t.TempDir()
			put := func(root, rel, data string) {
				t.Helper()
				p := filepath.Join(root, filepath.FromSlash(rel))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			put(from, "core/instructions.md", "Project conventions.\n")
			put(from, "core/skills/review/SKILL.md", skill)
			put(from, "core/skills/review/assets/policy.json", "{}\n")
			put(from, "core/agents/reviewer.md", agent)
			put(project, tc.root, "# User instructions\n")
			args := []string{"init", "core", "--from", from, "--dir", project, "--target", tc.name, "--yes"}
			if code, _, stderr := run(t, args...); code != ExitOK {
				t.Fatalf("init: %d %s", code, stderr)
			}
			want := map[string]string{
				tc.root:                                  "# User instructions\n\n<!-- kits:begin core -->\nProject conventions.\n<!-- kits:end core -->\n",
				tc.skills + "/review/SKILL.md":           skill,
				tc.skills + "/review/assets/policy.json": "{}\n",
			}
			if tc.agent != "" {
				want[tc.agent] = agent
			}
			checkFiles := func() {
				t.Helper()
				if err := filepath.WalkDir(project, func(path string, d os.DirEntry, err error) error {
					if err != nil || d.IsDir() {
						return err
					}
					rel, err := filepath.Rel(project, path)
					if err != nil {
						return err
					}
					rel = filepath.ToSlash(rel)
					if _, ok := want[rel]; !ok && rel != "kits.lock.json" {
						t.Errorf("unexpected file: %s", rel)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				for rel, expected := range want {
					data, err := os.ReadFile(filepath.Join(project, filepath.FromSlash(rel)))
					if err != nil || string(data) != expected {
						t.Errorf("%s: got %q, error %v, want %q", rel, data, err, expected)
					}
				}
			}
			checkFiles()
			if code, out, stderr := run(t, args...); code != ExitOK || !strings.Contains(out, "0 created, 0 updated, 0 deleted") {
				t.Fatalf("repeat: %d %s %s", code, out, stderr)
			}
			put(from, "core/skills/review/assets/policy.json", "{\"version\":2}\n")
			if code, _, stderr := run(t, args...); code != ExitOK {
				t.Fatalf("update: %d %s", code, stderr)
			}
			want[tc.skills+"/review/assets/policy.json"] = "{\"version\":2}\n"
			checkFiles()
			local := tc.skills + "/review/SKILL.md"
			want[local] = skill + "\nLocal edit.\n"
			put(project, local, want[local])
			if code, _, stderr := run(t, args...); code != ExitError || !strings.Contains(stderr, "modified since") {
				t.Fatalf("conflict: %d %s", code, stderr)
			}
			checkFiles()
		})
	}
}
