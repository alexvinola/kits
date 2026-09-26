package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/alexvinola/kits/internal/kit"
	"github.com/alexvinola/kits/internal/targets"
)

func agentsConfig(t *testing.T) *targets.Config {
	t.Helper()
	cfg, err := targets.Parse([]byte(`{"version":1,"targets":[
		{"name":"alpha","root_file":"ALPHA.md","skills_dir":".alpha/skills/{skill}","agents_dir":".alpha/agents/{agent}.md"},
		{"name":"beta","root_file":"BETA.md"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

var reviewerV1 = fstest.MapFS{
	"tools/agents/reviewer.md": {Data: []byte("---\nname: reviewer\ndescription: v1\n---\n")},
}

func plan(t *testing.T, dir string, confirm func(Missing) (bool, error), kits ...*kit.Kit) *Plan {
	t.Helper()
	p, err := PlanInit(Options{ProjectDir: dir, Config: agentsConfig(t), Kits: kits, Confirm: confirm})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Conflicts) > 0 {
		t.Fatalf("conflicts: %v", p.Conflicts)
	}
	return p
}

func TestAgentsIntoExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".alpha/agents"), 0o755)

	if err := plan(t, dir, nil, loadKit(t, reviewerV1, "tools")).Apply(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, dir, ".alpha/agents/reviewer.md"), "description: v1") {
		t.Error("agent not installed")
	}

	for _, op := range plan(t, dir, nil, loadKit(t, reviewerV1, "tools")).Ops {
		if op.Kind != Unchanged {
			t.Errorf("second run: %s %s", op.Kind, op.Path)
		}
	}
}

func TestDroppedAgentIsRemoved(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".alpha/agents"), 0o755)
	if err := plan(t, dir, nil, loadKit(t, reviewerV1, "tools")).Apply(); err != nil {
		t.Fatal(err)
	}

	v2 := fstest.MapFS{"tools/instructions.md": {Data: []byte("only instructions now\n")}}
	if err := plan(t, dir, nil, loadKit(t, v2, "tools")).Apply(); err != nil {
		t.Fatal(err)
	}
	if exists(dir, ".alpha/agents/reviewer.md") {
		t.Error("dropped agent not deleted")
	}
	if !exists(dir, ".alpha/agents") {
		t.Error("agents root pruned")
	}
}

func TestOnlyAsksAboutPathsTheKitsUse(t *testing.T) {
	dir := t.TempDir()
	var asked []string
	yes := func(m Missing) (bool, error) {
		asked = append(asked, string(m.Kind)+" "+m.Path)
		return true, nil
	}
	if err := plan(t, dir, yes, loadKit(t, reviewerV1, "tools")).Apply(); err != nil {
		t.Fatal(err)
	}
	// The kit has neither instructions nor skills.
	if got := strings.Join(asked, ", "); got != "agents directory .alpha/agents" {
		t.Errorf("asked about: %s", got)
	}
	if !exists(dir, ".alpha/agents/reviewer.md") {
		t.Error("agent not installed into the created directory")
	}
}
