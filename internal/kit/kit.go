// Package kit reads a kit: the instructions and skills that init installs.
//
// A kit is a directory named after the kit:
//
//	<kit>/
//	  instructions.md       optional; goes into each target's root file
//	  skills/<skill>/...    optional; each skill directory is copied whole
//	  agents/<agent>.md     optional; subagent definitions
//
// Anything else in the kit directory is ignored. Only regular files are
// accepted: a symlink in a kit could point anywhere on the machine that
// fetched it.
package kit

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
)

const (
	InstructionsFile = "instructions.md"
	SkillsDir        = "skills"
	AgentsDir        = "agents"
)

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ValidName reports whether s is usable as a kit or skill name.
func ValidName(s string) bool { return nameRe.MatchString(s) }

type Kit struct {
	Name         string
	Instructions []byte // nil when the kit has no instructions.md
	Skills       []Skill
	Agents       []Agent
}

// Agent is one subagent definition, agents/<Name>.md.
type Agent struct {
	Name string
	Data []byte
}

type Skill struct {
	Name  string
	Files []File
}

type File struct {
	Path string // slash-separated, relative to the skill directory
	Data []byte
	Exec bool
}

// Load reads the kit called name from the root of fsys.
func Load(fsys fs.FS, name string) (*Kit, error) {
	if !ValidName(name) {
		return nil, fmt.Errorf("invalid kit name %q (want %s)", name, nameRe)
	}
	entries, err := fs.ReadDir(fsys, name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("kit %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("kit %q: %w", name, err)
	}

	k := &Kit{Name: name}
	for _, e := range entries {
		p := path.Join(name, e.Name())
		switch e.Name() {
		case InstructionsFile:
			if !e.Type().IsRegular() {
				return nil, fmt.Errorf("%s: must be a regular file", p)
			}
			if k.Instructions, err = fs.ReadFile(fsys, p); err != nil {
				return nil, err
			}
		case SkillsDir:
			if !e.IsDir() {
				return nil, fmt.Errorf("%s: must be a directory", p)
			}
			if k.Skills, err = loadSkills(fsys, p); err != nil {
				return nil, err
			}
		case AgentsDir:
			if !e.IsDir() {
				return nil, fmt.Errorf("%s: must be a directory", p)
			}
			if k.Agents, err = loadAgents(fsys, p); err != nil {
				return nil, err
			}
		}
	}
	if k.Instructions == nil && len(k.Skills) == 0 && len(k.Agents) == 0 {
		return nil, fmt.Errorf("kit %q has none of %s, %s/, %s/", name, InstructionsFile, SkillsDir, AgentsDir)
	}
	return k, nil
}

func loadAgents(fsys fs.FS, dir string) ([]Agent, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var agents []Agent
	for _, e := range entries {
		p := path.Join(dir, e.Name())
		name, ok := strings.CutSuffix(e.Name(), ".md")
		switch {
		case !e.Type().IsRegular():
			return nil, fmt.Errorf("%s: only regular .md files are allowed in %s/", p, AgentsDir)
		case !ok || !ValidName(name):
			return nil, fmt.Errorf("%s: want <name>.md with a name matching %s", p, nameRe)
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, err
		}
		agents = append(agents, Agent{Name: name, Data: data})
	}
	return agents, nil
}

func loadSkills(fsys fs.FS, dir string) ([]Skill, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	var skills []Skill
	for _, e := range entries {
		p := path.Join(dir, e.Name())
		if !e.IsDir() {
			return nil, fmt.Errorf("%s: expected a skill directory", p)
		}
		if !ValidName(e.Name()) {
			return nil, fmt.Errorf("%s: invalid skill name (want %s)", p, nameRe)
		}
		s, err := loadSkill(fsys, p, e.Name())
		if err != nil {
			return nil, err
		}
		if len(s.Files) == 0 {
			return nil, fmt.Errorf("%s: empty skill", p)
		}
		skills = append(skills, s)
	}
	return skills, nil
}

func loadSkill(fsys fs.FS, dir, name string) (Skill, error) {
	s := Skill{Name: name}
	err := fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s: only regular files are allowed in a kit", p)
		}
		rel := strings.TrimPrefix(p, dir+"/")
		if strings.Contains(rel, `\`) {
			return fmt.Errorf("%s: backslash in file name", p)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		s.Files = append(s.Files, File{Path: rel, Data: data, Exec: info.Mode()&0o111 != 0})
		return nil
	})
	return s, err
}
