package install

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/alexvinola/kits/internal/kit"
	"github.com/alexvinola/kits/internal/lock"
	"github.com/alexvinola/kits/internal/targets"
)

func testConfig(t *testing.T) *targets.Config {
	t.Helper()
	cfg, err := targets.Parse([]byte(`{"version":1,"targets":[
		{"name":"alpha","root_file":"ALPHA.md","skills_dir":".alpha/skills/{skill}"},
		{"name":"beta","root_file":".beta/rules.md","skills_dir":".beta/skills/{skill}"},
		{"name":"gamma","root_file":"ALPHA.md"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func loadKit(t *testing.T, fsys fstest.MapFS, name string) *kit.Kit {
	t.Helper()
	k, err := kit.Load(fsys, name)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

var coreV1 = fstest.MapFS{
	"core/instructions.md":         {Data: []byte("core v1\n")},
	"core/skills/hello/SKILL.md":   {Data: []byte("hello v1\n")},
	"core/skills/bye/SKILL.md":     {Data: []byte("bye v1\n")},
	"core/skills/bye/scripts/x.sh": {Data: []byte("#!/bin/sh\n"), Mode: 0o755},
}

func write(t *testing.T, dir, rel, data string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func exists(dir, rel string) bool {
	_, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(rel)))
	return err == nil
}

func run(t *testing.T, dir string, force bool, kits ...*kit.Kit) *Plan {
	t.Helper()
	p, err := PlanInit(Options{ProjectDir: dir, Config: testConfig(t), Kits: kits, Force: force})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Conflicts) == 0 {
		if err := p.Apply(); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestDetectionOnlyTouchesExistingPaths(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "ALPHA.md", "# Mine\n")
	if err := os.MkdirAll(filepath.Join(dir, ".alpha/skills"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := run(t, dir, false, loadKit(t, coreV1, "core"))
	if len(p.Conflicts) > 0 {
		t.Fatalf("conflicts: %v", p.Conflicts)
	}

	if got := read(t, dir, "ALPHA.md"); !strings.HasPrefix(got, "# Mine\n\n<!-- kits:begin core -->\ncore v1\n") {
		t.Errorf("ALPHA.md = %q", got)
	}
	if read(t, dir, ".alpha/skills/hello/SKILL.md") != "hello v1\n" {
		t.Error("hello skill not installed")
	}
	// Windows has no exec bit to check.
	if info, err := os.Stat(filepath.Join(dir, ".alpha/skills/bye/scripts/x.sh")); err != nil {
		t.Error(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
		t.Error("x.sh not executable")
	}
	if exists(dir, ".beta") {
		t.Error(".beta was created in detection mode")
	}

	var missing []string
	for _, m := range p.Detection.Missing {
		missing = append(missing, m.Path)
	}
	if got := strings.Join(missing, ","); got != ".beta/rules.md,.beta/skills" {
		t.Errorf("missing = %s", got)
	}
	// alpha and gamma share ALPHA.md: one op, one block.
	if n := strings.Count(read(t, dir, "ALPHA.md"), "kits:begin core"); n != 1 {
		t.Errorf("%d core blocks in ALPHA.md", n)
	}

	l, err := lock.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Kits["core"].Files) != 3 {
		t.Errorf("lock files = %v", l.Kits["core"].Files)
	}
}

func TestRerunIsNoop(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "ALPHA.md", "")
	os.MkdirAll(filepath.Join(dir, ".alpha/skills"), 0o755)
	run(t, dir, false, loadKit(t, coreV1, "core"))

	p := run(t, dir, false, loadKit(t, coreV1, "core"))
	for _, op := range p.Ops {
		if op.Kind != Unchanged {
			t.Errorf("second run: %s %s", op.Kind, op.Path)
		}
	}
	if p.lock != nil {
		t.Error("second run rewrote the lock")
	}
}

func TestNothingDetected(t *testing.T) {
	p := run(t, t.TempDir(), false, loadKit(t, coreV1, "core"))
	if !p.Detection.Empty() || len(p.Ops) != 0 {
		t.Fatalf("detected %+v, ops %v", p.Detection, p.Ops)
	}
}

func TestUserFileConflicts(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".alpha/skills/hello/SKILL.md", "user's own\n")

	p := run(t, dir, false, loadKit(t, coreV1, "core"))
	if len(p.Conflicts) != 1 || !strings.Contains(p.Conflicts[0], "not installed by kits") {
		t.Fatalf("conflicts = %v", p.Conflicts)
	}
	if exists(dir, ".alpha/skills/bye") || exists(dir, lock.FileName) {
		t.Fatal("a plan with conflicts wrote something")
	}

	run(t, dir, true, loadKit(t, coreV1, "core"))
	if read(t, dir, ".alpha/skills/hello/SKILL.md") != "hello v1\n" {
		t.Fatal("--force did not overwrite")
	}
}

func TestModifiedFileConflicts(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".alpha/skills"), 0o755)
	run(t, dir, false, loadKit(t, coreV1, "core"))
	write(t, dir, ".alpha/skills/hello/SKILL.md", "edited\n")

	v2 := fstest.MapFS{
		"core/skills/hello/SKILL.md": {Data: []byte("hello v2\n")},
	}
	p := run(t, dir, false, loadKit(t, v2, "core"))
	if len(p.Conflicts) != 1 || !strings.Contains(p.Conflicts[0], "modified since") {
		t.Fatalf("conflicts = %v", p.Conflicts)
	}
}

func TestUpdateDeletesDroppedFilesAndPrunes(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "ALPHA.md", "# Mine\n")
	os.MkdirAll(filepath.Join(dir, ".alpha/skills"), 0o755)
	run(t, dir, false, loadKit(t, coreV1, "core"))

	// v2 drops the bye skill and the instructions.
	v2 := fstest.MapFS{"core/skills/hello/SKILL.md": {Data: []byte("hello v2\n")}}
	p := run(t, dir, false, loadKit(t, v2, "core"))
	if len(p.Conflicts) > 0 {
		t.Fatal(p.Conflicts)
	}
	if read(t, dir, ".alpha/skills/hello/SKILL.md") != "hello v2\n" {
		t.Error("hello not updated")
	}
	if exists(dir, ".alpha/skills/bye") {
		t.Error("bye skill dir not pruned")
	}
	if !exists(dir, ".alpha/skills") {
		t.Error("skills root pruned")
	}
	if got := read(t, dir, "ALPHA.md"); got != "# Mine\n" {
		t.Errorf("block not removed: %q", got)
	}
}

func TestSymlinkedRootFileIsWrittenThrough(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "real.md", "# Real\n")
	if err := os.Symlink("real.md", filepath.Join(dir, "ALPHA.md")); err != nil {
		t.Skip(err)
	}
	run(t, dir, false, loadKit(t, coreV1, "core"))
	info, err := os.Lstat(filepath.Join(dir, "ALPHA.md"))
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced")
	}
	if !strings.Contains(read(t, dir, "real.md"), "kits:begin core") {
		t.Error("block not written to the link target")
	}
}

func TestCrossKitCollision(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".alpha/skills"), 0o755)
	ia := fstest.MapFS{"ia/skills/hello/SKILL.md": {Data: []byte("ia hello\n")}}
	p := run(t, dir, false, loadKit(t, coreV1, "core"), loadKit(t, ia, "ia"))
	if len(p.Conflicts) != 1 || !strings.Contains(p.Conflicts[0], `"core" and "ia"`) {
		t.Fatalf("conflicts = %v", p.Conflicts)
	}
}

func TestConfirmCreatesApprovedPaths(t *testing.T) {
	dir := t.TempDir()
	var asked []string
	confirm := func(m Missing) (bool, error) {
		asked = append(asked, m.Path)
		return m.Path != ".alpha/skills", nil // yes to ALPHA.md, no to skills
	}
	p, err := PlanInit(Options{ProjectDir: dir, Config: testConfig(t), Kits: []*kit.Kit{loadKit(t, coreV1, "core")},
		Only: []string{"alpha"}, Confirm: confirm})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(asked, ","); got != "ALPHA.md,.alpha/skills" {
		t.Fatalf("asked about %s", got)
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, dir, "ALPHA.md"), "core v1") {
		t.Error("approved root file not created with the block")
	}
	if exists(dir, ".alpha") || exists(dir, ".beta") {
		t.Error("declined or unselected path was created")
	}
	if len(p.Detection.Missing) != 1 || !p.Detection.Missing[0].Declined {
		t.Errorf("missing = %+v", p.Detection.Missing)
	}
}

func TestConfirmCreatesSkillsRootInNestedDir(t *testing.T) {
	dir := t.TempDir()
	yes := func(Missing) (bool, error) { return true, nil }
	p, err := PlanInit(Options{ProjectDir: dir, Config: testConfig(t), Kits: []*kit.Kit{loadKit(t, coreV1, "core")},
		Only: []string{"beta"}, Confirm: yes})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	if read(t, dir, ".beta/skills/hello/SKILL.md") != "hello v1\n" || !strings.Contains(read(t, dir, ".beta/rules.md"), "core v1") {
		t.Error("beta paths not created")
	}
}

func TestConfirmErrorAborts(t *testing.T) {
	dir := t.TempDir()
	no := func(Missing) (bool, error) { return false, os.ErrClosed }
	_, err := PlanInit(Options{ProjectDir: dir, Config: testConfig(t), Kits: []*kit.Kit{loadKit(t, coreV1, "core")},
		Only: []string{"alpha"}, Confirm: no})
	if err == nil {
		t.Fatal("want error")
	}
}

func TestOnlyKeepsOtherTargetsFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".alpha/skills"), 0o755)
	os.MkdirAll(filepath.Join(dir, ".beta/skills"), 0o755)
	run(t, dir, false, loadKit(t, coreV1, "core"))

	p, err := PlanInit(Options{ProjectDir: dir, Config: testConfig(t), Kits: []*kit.Kit{loadKit(t, coreV1, "core")},
		Only: []string{"alpha"}, Confirm: func(Missing) (bool, error) { return false, nil }})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range p.Ops {
		if op.Kind == Delete {
			t.Errorf("deletes %s from an unselected target", op.Path)
		}
	}
	if err := p.Apply(); err != nil {
		t.Fatal(err)
	}
	l, _ := lock.Load(dir)
	if _, ok := l.Kits["core"].Files[".beta/skills/hello/SKILL.md"]; !ok {
		t.Error("lock forgot beta's files")
	}
}

func TestUnknownOnlyTarget(t *testing.T) {
	_, err := PlanInit(Options{ProjectDir: t.TempDir(), Config: testConfig(t), Only: []string{"zeta"}})
	if err == nil || !strings.Contains(err.Error(), "known: alpha, beta, gamma") {
		t.Fatalf("err = %v", err)
	}
}
