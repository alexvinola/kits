// Package userconfig holds per-user settings, such as which repository kits
// are fetched from. Nothing about any particular user's kits is compiled into
// the binary: without this file (or --repo), init asks to be configured.
package userconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/alexvinola/kits/internal/fsutil"
)

// Config is the user's settings file.
type Config struct {
	Repo string `json:"repo,omitempty"` // repository holding the kits
	Ref  string `json:"ref,omitempty"`  // branch or tag; empty is the default branch
	Path string `json:"path,omitempty"` // kits directory inside the repository; empty is its root
}

// Path is $XDG_CONFIG_HOME/kits/config.json, falling back to
// ~/.config/kits/config.json (also on macOS, like most CLIs) and to the
// roaming AppData directory on Windows.
func Path() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "kits", "config.json"), nil
	}
	if runtime.GOOS == "windows" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(dir, "kits", "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "kits", "config.json"), nil
}

// Load reads the config at path. A missing file is an empty config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// Save writes the config to path, creating its directory.
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return fsutil.WriteAtomic(path, append(data, '\n'), 0o644)
}

// Field returns a pointer to the setting called key, for get/set/unset.
func (c *Config) Field(key string) (*string, error) {
	switch key {
	case "repo":
		return &c.Repo, nil
	case "ref":
		return &c.Ref, nil
	case "path":
		return &c.Path, nil
	}
	return nil, fmt.Errorf("unknown setting %q (known: %v)", key, Keys())
}

// Keys lists the settings in display order.
func Keys() []string {
	return []string{"repo", "ref", "path"}
}
