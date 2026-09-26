// Package targets loads the declarative mapping from a coding-agent target
// (claude, cursor, copilot, ...) to the paths where kits install into a
// project.
//
// Nothing about a specific tool lives in Go code: every path comes from
// default.json, embedded in the binary. Supporting another agent, or fixing a
// path, is an edit to that file.
package targets

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/alexvinola/kits/internal/relpath"
)

//go:embed default.json
var defaultConfig []byte

// SchemaVersion is the only config version this binary understands.
const SchemaVersion = 1

// Placeholders accepted in skills_dir and agents_dir.
const (
	PlaceholderKit   = "{kit}"
	PlaceholderSkill = "{skill}"
	PlaceholderAgent = "{agent}"
)

// Config is the whole targets file.
type Config struct {
	Version int      `json:"version"`
	Targets []Target `json:"targets"`
}

// Target describes where one coding agent expects kit content.
//
// RootFile is the project-level instructions file. SkillsDir is a pattern for
// the directory of one skill; it must contain {skill} and may contain {kit}.
// AgentsDir is a pattern for the file of one subagent; it must contain
// {agent} and may contain {kit}. Empty fields mean the target has no such
// location.
type Target struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	RootFile    string `json:"root_file"`
	SkillsDir   string `json:"skills_dir,omitempty"`
	AgentsDir   string `json:"agents_dir,omitempty"`
}

// Default returns the config embedded in the binary.
func Default() (*Config, error) {
	return Parse(defaultConfig)
}

// Parse decodes and validates a config. Unknown fields are rejected so a typo
// like "skill_dir" fails loudly instead of silently disabling skills.
func Parse(data []byte) (*Config, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse targets config: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("parse targets config: trailing data after the top-level object")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

var (
	nameRe        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	placeholderRe = regexp.MustCompile(`\{[^{}]*\}`)
)

// Validate reports every problem in the config at once.
func (c *Config) Validate() error {
	var errs []error
	if c.Version != SchemaVersion {
		errs = append(errs, fmt.Errorf("version: got %d, want %d", c.Version, SchemaVersion))
	}
	if len(c.Targets) == 0 {
		errs = append(errs, errors.New("targets: at least one target is required"))
	}
	seen := make(map[string]bool, len(c.Targets))
	for i, t := range c.Targets {
		where := fmt.Sprintf("targets[%d]", i)
		if t.Name != "" {
			where = fmt.Sprintf("target %q", t.Name)
		}
		if !nameRe.MatchString(t.Name) {
			errs = append(errs, fmt.Errorf("%s: name must match %s", where, nameRe))
		} else if seen[t.Name] {
			errs = append(errs, fmt.Errorf("%s: duplicate name", where))
		}
		seen[t.Name] = true

		if t.RootFile == "" {
			errs = append(errs, fmt.Errorf("%s: root_file is required", where))
		} else if err := relpath.Check(t.RootFile); err != nil {
			errs = append(errs, fmt.Errorf("%s: root_file: %w", where, err))
		} else if strings.ContainsAny(t.RootFile, "{}") {
			errs = append(errs, fmt.Errorf("%s: root_file: placeholders are not allowed", where))
		}

		if t.SkillsDir != "" {
			if err := checkPattern(t.SkillsDir, PlaceholderSkill); err != nil {
				errs = append(errs, fmt.Errorf("%s: skills_dir: %w", where, err))
			}
		}
		if t.AgentsDir != "" {
			if err := checkPattern(t.AgentsDir, PlaceholderAgent); err != nil {
				errs = append(errs, fmt.Errorf("%s: agents_dir: %w", where, err))
			}
		}
	}
	return errors.Join(errs...)
}

// checkPattern validates a skills_dir or agents_dir pattern: a clean relative
// path starting with a fixed directory, holding the required placeholder and
// optionally {kit}.
func checkPattern(p, required string) error {
	if err := relpath.Check(p); err != nil {
		return err
	}
	for _, ph := range placeholderRe.FindAllString(p, -1) {
		if ph != PlaceholderKit && ph != required {
			return fmt.Errorf("%q: unknown placeholder %s (allowed: %s, %s)", p, ph, PlaceholderKit, required)
		}
	}
	if strings.ContainsAny(placeholderRe.ReplaceAllString(p, ""), "{}") {
		return fmt.Errorf("%q: unbalanced braces", p)
	}
	if !strings.Contains(p, required) {
		return fmt.Errorf("%q: must contain %s", p, required)
	}
	if fixedRoot(p) == "" {
		return fmt.Errorf("%q: must start with a fixed directory before any placeholder", p)
	}
	return nil
}

// Get returns the target with the given name.
func (c *Config) Get(name string) (Target, bool) {
	for _, t := range c.Targets {
		if t.Name == name {
			return t, true
		}
	}
	return Target{}, false
}

// Select returns a config holding only the named targets, in the given
// order. Unknown names are an error that lists the known ones.
func (c *Config) Select(names []string) (*Config, error) {
	out := &Config{Version: c.Version}
	seen := map[string]bool{}
	for _, n := range names {
		t, ok := c.Get(n)
		if !ok {
			return nil, fmt.Errorf("unknown target %q (known: %s)", n, strings.Join(c.Names(), ", "))
		}
		if !seen[n] {
			seen[n] = true
			out.Targets = append(out.Targets, t)
		}
	}
	return out, nil
}

// Names returns target names in config order.
func (c *Config) Names() []string {
	names := make([]string, len(c.Targets))
	for i, t := range c.Targets {
		names[i] = t.Name
	}
	return names
}

// SkillsRoot is the fixed directory prefix of SkillsDir, before the first
// segment holding a placeholder: ".claude/skills" for ".claude/skills/{skill}".
// Detection mode checks this path to decide whether a project already uses
// the target's skills. It is empty when the target has no SkillsDir.
func (t Target) SkillsRoot() string {
	return fixedRoot(t.SkillsDir)
}

// AgentsRoot is the fixed directory prefix of AgentsDir: ".claude/agents" for
// ".claude/agents/{agent}.md". Empty when the target has no AgentsDir.
func (t Target) AgentsRoot() string {
	return fixedRoot(t.AgentsDir)
}

// AgentPath expands AgentsDir for one agent of one kit, or "" without one.
func (t Target) AgentPath(kit, agent string) string {
	if t.AgentsDir == "" {
		return ""
	}
	return strings.NewReplacer(PlaceholderKit, kit, PlaceholderAgent, agent).Replace(t.AgentsDir)
}

// fixedRoot is the part of a pattern before the first segment holding a
// placeholder.
func fixedRoot(pattern string) string {
	segs := strings.Split(pattern, "/")
	for i, seg := range segs {
		if strings.Contains(seg, "{") {
			return strings.Join(segs[:i], "/")
		}
	}
	return pattern
}

// SkillPath expands SkillsDir for one skill of one kit. It returns "" when the
// target has no SkillsDir. Callers validate kit and skill names beforehand.
func (t Target) SkillPath(kit, skill string) string {
	if t.SkillsDir == "" {
		return ""
	}
	return strings.NewReplacer(PlaceholderKit, kit, PlaceholderSkill, skill).Replace(t.SkillsDir)
}
