package userconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	c, err := Load(path)
	if err != nil || *c != (Config{}) {
		t.Fatalf("missing file: %+v, %v", c, err)
	}
	c.Repo, c.Path = "git@example.com:me/kits.git", "kits"
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || *got != *c {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{"repos":"x"}`), 0o644)
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("err = %v", err)
	}
}

func TestPathHonoursXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/cfg")
	if p, _ := Path(); p != filepath.Join("/cfg", "kits", "config.json") {
		t.Fatalf("path = %s", p)
	}
}
