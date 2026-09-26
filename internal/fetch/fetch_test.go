package fetch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo creates a local repository holding two kits under kits/ and
// returns its file:// URL.
func newRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	// Keep the user's git config (signing, hooks, templates) out of the test.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	dir := t.TempDir()
	files := map[string]string{
		"README.md":                         "readme\n",
		"kits/core/instructions.md":         "core\n",
		"kits/core/skills/hello/SKILL.md":   "hello\n",
		"kits/ia/instructions.md":           "ia\n",
		"kits/ia/skills/prompting/SKILL.md": "prompting\n",
	}
	for rel, data := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "uploadpack.allowFilter", "true"},
		{"add", "."},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-qm", "init"},
		{"tag", "v1"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return fileURL(dir)
}

// fileURL builds a file:// URL git accepts on every OS, including
// file:///C:/... on Windows.
func fileURL(dir string) string {
	p := filepath.ToSlash(dir)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}

func TestFetchOnlyRequestedKits(t *testing.T) {
	repo := newRepo(t)
	for _, ref := range []string{"", "main", "v1"} {
		co, err := Fetch(context.Background(), Source{Repo: repo, Ref: ref, Path: "kits"}, []string{"core"})
		if err != nil {
			t.Fatalf("ref %q: %v", ref, err)
		}
		if _, err := os.Stat(filepath.Join(co.Dir, "core", "skills", "hello", "SKILL.md")); err != nil {
			t.Errorf("ref %q: core not checked out: %v", ref, err)
		}
		if _, err := os.Stat(filepath.Join(co.Dir, "ia")); err == nil {
			t.Errorf("ref %q: ia was checked out", ref)
		}
		if len(co.Commit) != 40 {
			t.Errorf("ref %q: commit = %q", ref, co.Commit)
		}
		root := co.root
		co.Close()
		if _, err := os.Stat(root); err == nil {
			t.Error("Close left the checkout behind")
		}
	}
}

func TestFetchErrors(t *testing.T) {
	repo := newRepo(t)
	cases := []struct {
		src  Source
		kits []string
		want string
	}{
		{Source{Repo: repo, Ref: "nope", Path: "kits"}, []string{"core"}, "git clone"},
		{Source{Repo: repo, Path: "../x"}, []string{"core"}, "kits path"},
		{Source{Repo: repo, Ref: "--upload-pack=x"}, []string{"core"}, "invalid ref"},
		{Source{Repo: repo, Path: "kits"}, []string{"Bad"}, "invalid kit name"},
	}
	for _, tc := range cases {
		_, err := Fetch(context.Background(), tc.src, tc.kits)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v: err = %v, want %q", tc.src, err, tc.want)
		}
	}
}

func TestURL(t *testing.T) {
	cases := map[string]string{
		"alexvinola/kits":                    "https://github.com/alexvinola/kits.git",
		"https://gitlab.com/a/b.git":         "https://gitlab.com/a/b.git",
		"git@github.com:alexvinola/kits.git": "git@github.com:alexvinola/kits.git",
		"./local/repo":                       "./local/repo",
	}
	for in, want := range cases {
		if got := (Source{Repo: in}).URL(); got != want {
			t.Errorf("URL(%q) = %q, want %q", in, got, want)
		}
	}
}
