package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	return runWithInput(t, nil, args...)
}

// Keep every test away from the real ~/.config/kits.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "kits-cli-test-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_CONFIG_HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// runWithInput runs a command whose stdin is a terminal answering with input;
// a nil input means stdin is not a terminal.
func runWithInput(t *testing.T, input *string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	env := Env{Stdout: &out, Stderr: &errOut}
	if input != nil {
		env.Stdin, env.StdinIsTTY = strings.NewReader(*input), true
	}
	code = Run(context.Background(), env, args)
	return code, out.String(), errOut.String()
}

func TestTargetsDefault(t *testing.T) {
	code, out, errOut := run(t, "targets")
	if code != ExitOK {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	for _, want := range []string{"claude", "cursor", "copilot", "codex", "agents", "CLAUDE.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestTargetsJSON(t *testing.T) {
	code, out, errOut := run(t, "targets", "--json")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var got struct {
		Targets []struct {
			Name      string `json:"name"`
			AgentsDir string `json:"agents_dir"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(got.Targets) == 0 || got.Targets[0].Name != "claude" || got.Targets[0].AgentsDir == "" {
		t.Fatalf("targets = %+v", got.Targets)
	}
}

func TestUnknownCommand(t *testing.T) {
	if code, _, _ := run(t, "nope"); code != ExitUsage {
		t.Fatalf("exit %d, want %d", code, ExitUsage)
	}
}

func TestInitFromFixtures(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "CLAUDE.md"), []byte("# Mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	from := filepath.Join("..", "..", "testdata", "kits")

	code, out, errOut := run(t, "init", "core", "ia", "--from", from, "--dir", project, "--dry-run")
	if code != ExitOK {
		t.Fatalf("dry run exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "nothing written") {
		t.Errorf("dry run output:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "hello")); err == nil {
		t.Fatal("dry run wrote files")
	}

	code, out, errOut = run(t, "init", "core", "ia", "--from", from, "--dir", project)
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, want := range []string{"create", ".claude/skills/prompting/SKILL.md", "skip", ".github/skills", "not found (copilot)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestInitNothingDetected(t *testing.T) {
	from := filepath.Join("..", "..", "testdata", "kits")
	code, _, errOut := run(t, "init", "core", "--from", from, "--dir", t.TempDir())
	if code != ExitError || !strings.Contains(errOut, "no integration paths") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
}

func TestInitFetchesFromRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	// A repository whose kits/ directory is a copy of testdata/kits.
	repo := t.TempDir()
	if err := os.CopyFS(filepath.Join(repo, "kits"), os.DirFS(filepath.Join("..", "..", "testdata", "kits"))); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "."},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-qm", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	project := t.TempDir()
	os.MkdirAll(filepath.Join(project, ".claude", "skills"), 0o755)
	url := "file://" + filepath.ToSlash(repo)
	if !strings.HasPrefix(filepath.ToSlash(repo), "/") {
		url = "file:///" + filepath.ToSlash(repo) // Windows: file:///C:/...
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	run(t, "config", "set", "repo", url)
	run(t, "config", "set", "path", "kits")
	code, out, errOut := run(t, "init", "ia", "--dir", project)
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, ".claude/skills/prompting/SKILL.md") {
		t.Errorf("output:\n%s", out)
	}
	lockData, err := os.ReadFile(filepath.Join(project, "kits.lock.json"))
	if err != nil || !strings.Contains(string(lockData), `"commit"`) {
		t.Errorf("lock lacks the source commit: %v\n%s", err, lockData)
	}

	code, _, errOut = run(t, "init", "nope", "--repo", url, "--path", "kits", "--dir", project)
	if code != ExitError || !strings.Contains(errOut, `kit "nope" not found in file://`) {
		t.Errorf("missing kit: exit %d: %s", code, errOut)
	}
}

func TestInitFromExcludesRepo(t *testing.T) {
	code, _, errOut := run(t, "init", "core", "--from", ".", "--repo", "a/b")
	if code != ExitUsage || !strings.Contains(errOut, "--repo") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
}

func TestInitTargetAsksPerPath(t *testing.T) {
	project := t.TempDir()
	from := filepath.Join("..", "..", "testdata", "kits")
	answers := "y\nn\n"
	code, out, errOut := runWithInput(t, &answers, "init", "core", "--from", from, "--dir", project, "--target", "claude")
	if code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, want := range []string{"Create file CLAUDE.md (claude)? [y/N]", "Create directory .claude/skills (claude)? [y/N]"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("prompt %q missing:\n%s", want, errOut)
		}
	}
	if !strings.Contains(out, "declined (claude)") {
		t.Errorf("output:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(project, "CLAUDE.md")); err != nil {
		t.Error("approved CLAUDE.md not created")
	}
	if _, err := os.Stat(filepath.Join(project, ".claude")); err == nil {
		t.Error("declined .claude/skills was created")
	}
	if strings.Contains(out, "cursor") || strings.Contains(out, "copilot") {
		t.Errorf("unselected targets appear in the output:\n%s", out)
	}
}

func TestInitTargetWithoutTerminal(t *testing.T) {
	project := t.TempDir()
	from := filepath.Join("..", "..", "testdata", "kits")
	code, _, errOut := run(t, "init", "core", "--from", from, "--dir", project, "--target", "claude")
	if code != ExitError || !strings.Contains(errOut, "pass --yes") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	entries, _ := os.ReadDir(project)
	if len(entries) != 0 {
		t.Fatalf("wrote %v without confirmation", entries)
	}

	code, out, errOut := run(t, "init", "core", "--from", from, "--dir", project, "--target", "claude,copilot", "--yes")
	if code != ExitOK {
		t.Fatalf("--yes exit %d: %s", code, errOut)
	}
	for _, want := range []string{"mkdir", ".claude/skills", ".github/skills", "create", ".github/copilot-instructions.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestInitTargetDryRunDoesNotAsk(t *testing.T) {
	project := t.TempDir()
	from := filepath.Join("..", "..", "testdata", "kits")
	code, out, errOut := run(t, "init", "core", "--from", from, "--dir", project, "--target", "claude", "--dry-run")
	if code != ExitOK || !strings.Contains(out, "nothing written") {
		t.Fatalf("exit %d: %s\n%s", code, errOut, out)
	}
	if entries, _ := os.ReadDir(project); len(entries) != 0 {
		t.Fatalf("dry run wrote %v", entries)
	}
}

func TestInitUsageErrors(t *testing.T) {
	cases := map[string][]string{
		"unknown target":     {"init", "core", "--from", ".", "--target", "vim"},
		"yes without target": {"init", "core", "--from", ".", "--yes"},
	}
	for name, args := range cases {
		if code, _, _ := run(t, args...); code != ExitUsage {
			t.Errorf("%s: exit %d, want %d", name, code, ExitUsage)
		}
	}
}

func TestConfigCommand(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if code, _, errOut := run(t, "config", "set", "repo", "git@example.com:me/kits.git"); code != ExitOK {
		t.Fatalf("set: %s", errOut)
	}
	if code, _, errOut := run(t, "config", "set", "path", "kits"); code != ExitOK {
		t.Fatalf("set path: %s", errOut)
	}
	if _, out, _ := run(t, "config", "get", "repo"); out != "git@example.com:me/kits.git\n" {
		t.Errorf("get repo = %q", out)
	}
	if _, out, _ := run(t, "config"); !strings.Contains(out, "path = kits") {
		t.Errorf("list:\n%s", out)
	}
	run(t, "config", "unset", "path")
	if _, out, _ := run(t, "config", "get", "path"); out != "\n" {
		t.Errorf("path after unset = %q", out)
	}
	for name, args := range map[string][]string{
		"unknown key": {"config", "set", "branch", "x"},
		"bad path":    {"config", "set", "path", "../x"},
		"bad ref":     {"config", "set", "ref", "-x"},
		"missing arg": {"config", "set", "repo"},
		"bad verb":    {"config", "delete", "repo"},
	} {
		if code, _, _ := run(t, args...); code != ExitUsage {
			t.Errorf("%s: exit %d", name, code)
		}
	}
}

func TestInitWithoutRepoConfigured(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	code, _, errOut := run(t, "init", "core", "--dir", t.TempDir())
	if code != ExitUsage || !strings.Contains(errOut, "kits config set repo") {
		t.Fatalf("exit %d: %s", code, errOut)
	}
}
