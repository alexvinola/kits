// Package install plans and applies kit installs into a project.
//
// Planning only reads; Apply writes exactly what the plan says. A plan with
// conflicts cannot be applied, so a failed init leaves the project untouched.
package install

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexvinola/kits/internal/fsutil"
	"github.com/alexvinola/kits/internal/kit"
	"github.com/alexvinola/kits/internal/lock"
	"github.com/alexvinola/kits/internal/rootfile"
	"github.com/alexvinola/kits/internal/targets"
)

type Kind string

const (
	Create    Kind = "create"
	Update    Kind = "update"
	Delete    Kind = "delete"
	Unchanged Kind = "unchanged"
	MkDir     Kind = "mkdir"
)

// Op is one file change. Path is slash-separated and project-relative.
type Op struct {
	Kind    Kind
	Path    string
	Kits    []string
	Targets []string // for MkDir: the targets the directory is for
	data    []byte
	perm    fs.FileMode
}

// PathKind is the kind of an integration path.
type PathKind string

const (
	RootFile  PathKind = "root file"
	SkillsDir PathKind = "skills directory"
	AgentsDir PathKind = "agents directory"
)

// Missing is an integration path from the config that the project lacks.
type Missing struct {
	Path     string
	Kind     PathKind
	Targets  []string
	Declined bool // the user was asked to create it and said no
}

// IsFile reports whether creating m means creating a file (not a directory).
func (m Missing) IsFile() bool { return m.Kind == RootFile }

// Detection is which integration paths already exist in a project.
type Detection struct {
	RootFiles     []string         // existing root files, symlinks resolved, deduplicated
	SkillsTargets []targets.Target // targets whose skills root exists
	AgentsTargets []targets.Target // targets whose agents root exists
	Missing       []Missing
	Warnings      []string
}

func (d *Detection) Empty() bool {
	return len(d.RootFiles) == 0 && len(d.SkillsTargets) == 0 && len(d.AgentsTargets) == 0
}

// Uses says which kinds of integration path the kits being installed need.
// A missing path of a kind no kit uses is neither reported nor offered for
// creation: nobody wants to be asked about .claude/agents for a kit without
// agents. Existing paths are always planned, so stale content still goes.
type Uses struct {
	Instructions, Skills, Agents bool
}

// UsesOf reports what kits need.
func UsesOf(kits []*kit.Kit) Uses {
	var u Uses
	for _, k := range kits {
		u.Instructions = u.Instructions || k.Instructions != nil
		u.Skills = u.Skills || len(k.Skills) > 0
		u.Agents = u.Agents || len(k.Agents) > 0
	}
	return u
}

// Detect scans projectDir, which must have its symlinks resolved, for every
// integration path in cfg.
func Detect(projectDir string, cfg *targets.Config, uses Uses) (*Detection, error) {
	d := &Detection{}
	missingAt := map[string]int{}
	addMissing := func(p string, kind PathKind, target string) {
		if i, ok := missingAt[p]; ok {
			d.Missing[i].Targets = append(d.Missing[i].Targets, target)
			return
		}
		missingAt[p] = len(d.Missing)
		d.Missing = append(d.Missing, Missing{Path: p, Kind: kind, Targets: []string{target}})
	}
	warn := func(p, target string, err error) {
		d.Warnings = append(d.Warnings, fmt.Sprintf("%s (%s): %v; skipped", p, target, err))
	}
	seenRoot := map[string]bool{}

	for _, t := range cfg.Targets {
		rel, ok, err := resolveFile(projectDir, t.RootFile)
		switch {
		case err != nil:
			warn(t.RootFile, t.Name, err)
		case !ok:
			if uses.Instructions {
				addMissing(t.RootFile, RootFile, t.Name)
			}
		case !seenRoot[rel]:
			seenRoot[rel] = true
			d.RootFiles = append(d.RootFiles, rel)
		}

		for _, dir := range []struct {
			root string
			kind PathKind
			used bool
			into *[]targets.Target
		}{
			{t.SkillsRoot(), SkillsDir, uses.Skills, &d.SkillsTargets},
			{t.AgentsRoot(), AgentsDir, uses.Agents, &d.AgentsTargets},
		} {
			if dir.root == "" {
				continue
			}
			info, err := os.Stat(filepath.Join(projectDir, filepath.FromSlash(dir.root)))
			switch {
			case errors.Is(err, fs.ErrNotExist):
				if dir.used {
					addMissing(dir.root, dir.kind, t.Name)
				}
			case err != nil:
				warn(dir.root, t.Name, err)
			case !info.IsDir():
				warn(dir.root, t.Name, errors.New("not a directory"))
			default:
				*dir.into = append(*dir.into, t)
			}
		}
	}
	return d, nil
}

// resolveFile follows a symlinked root file (CLAUDE.md -> AGENTS.md is a
// common setup) to the real file, so the block is written there once instead
// of the atomic rename replacing the link with a regular file.
func resolveFile(projectDir, rel string) (string, bool, error) {
	abs := filepath.Join(projectDir, filepath.FromSlash(rel))
	info, err := os.Lstat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(abs)
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, errors.New("dangling symlink")
		}
		if err != nil {
			return "", false, err
		}
		r, err := filepath.Rel(projectDir, resolved)
		if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return "", false, errors.New("symlink points outside the project")
		}
		rel = filepath.ToSlash(r)
		if info, err = os.Stat(resolved); err != nil {
			return "", false, err
		}
	}
	if !info.Mode().IsRegular() {
		return "", false, errors.New("not a regular file")
	}
	return rel, true, nil
}

type Options struct {
	ProjectDir string
	Config     *targets.Config
	Kits       []*kit.Kit
	Source     *lock.Source // recorded in the lock for every kit; nil for local kits
	Force      bool         // overwrite files kits did not install or that were modified

	// Only restricts the plan to these targets. Empty means every target.
	Only []string
	// Confirm, when set, is asked once per missing integration path of the
	// planned targets; paths it approves are created. Nil is detection mode:
	// missing paths are only reported.
	Confirm func(Missing) (bool, error)
}

type Plan struct {
	Dir       string
	Detection *Detection
	Ops       []Op
	Conflicts []string
	Warnings  []string

	stopDirs map[string]bool // never pruned: the configured skills and agents roots
	newRoots map[string]bool // root files approved for creation
	lock     []byte          // new lock contents; nil when unchanged
}

// PlanInit plans installing opts.Kits into the project's integration paths:
// the ones that exist, plus the missing ones opts.Confirm approves.
func PlanInit(opts Options) (*Plan, error) {
	dir, err := filepath.EvalSymlinks(opts.ProjectDir)
	if err != nil {
		return nil, err
	}
	cfg := opts.Config
	if len(opts.Only) > 0 {
		if cfg, err = cfg.Select(opts.Only); err != nil {
			return nil, err
		}
	}
	det, err := Detect(dir, cfg, UsesOf(opts.Kits))
	if err != nil {
		return nil, err
	}
	prev, err := lock.Load(dir)
	if err != nil {
		return nil, err
	}

	p := &Plan{
		Dir: dir, Detection: det,
		stopDirs: map[string]bool{}, newRoots: map[string]bool{},
	}
	p.Warnings = append(p.Warnings, det.Warnings...)
	// Every configured root, not just the selected ones: pruning must never
	// remove another target's directory.
	for _, t := range opts.Config.Targets {
		for _, r := range []string{t.SkillsRoot(), t.AgentsRoot()} {
			if r != "" {
				p.stopDirs[r] = true
			}
		}
	}
	if opts.Confirm != nil {
		if err := p.confirmMissing(cfg, opts.Confirm); err != nil {
			return nil, err
		}
	}

	if err := p.planRootFiles(opts.Kits); err != nil {
		return nil, err
	}

	entries := map[string]*lock.Entry{}
	claimed := map[string]string{} // dest -> kit, within this run
	for _, k := range opts.Kits {
		entry := &lock.Entry{Source: opts.Source, Files: p.planFiles(k, prev, claimed, opts.Force)}
		if k.Instructions != nil {
			entry.RootFiles = p.rootFilesFor(prev.Kits[k.Name].RootFiles)
		}
		entries[k.Name] = entry
	}

	next := prev.Clone()
	for name, e := range entries {
		next.Kits[name] = *e
	}
	before, err := prev.Marshal()
	if err != nil {
		return nil, err
	}
	after, err := next.Marshal()
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(before, after) {
		p.lock = after
	}
	return p, nil
}

// confirmMissing asks about each missing path of cfg's targets and moves the
// approved ones into the detection, as if they existed.
func (p *Plan) confirmMissing(cfg *targets.Config, confirm func(Missing) (bool, error)) error {
	det := p.Detection
	var still []Missing
	for _, m := range det.Missing {
		ok, err := confirm(m)
		if err != nil {
			return err
		}
		if !ok {
			m.Declined = true
			still = append(still, m)
			continue
		}
		var ts []targets.Target
		for _, name := range m.Targets {
			t, _ := cfg.Get(name)
			ts = append(ts, t)
		}
		switch m.Kind {
		case RootFile:
			p.newRoots[m.Path] = true
			det.RootFiles = append(det.RootFiles, m.Path)
		case SkillsDir:
			p.Ops = append(p.Ops, Op{Kind: MkDir, Path: m.Path, Targets: m.Targets})
			det.SkillsTargets = append(det.SkillsTargets, ts...)
		case AgentsDir:
			p.Ops = append(p.Ops, Op{Kind: MkDir, Path: m.Path, Targets: m.Targets})
			det.AgentsTargets = append(det.AgentsTargets, ts...)
		}
	}
	det.Missing = still
	return nil
}

// rootFilesFor lists the root files a kit's block lives in after this plan:
// the planned ones, plus earlier ones outside the plan that still exist.
func (p *Plan) rootFilesFor(prev []string) []string {
	out := append([]string(nil), p.Detection.RootFiles...)
	planned := map[string]bool{}
	for _, r := range out {
		planned[r] = true
	}
	for _, r := range prev {
		if !planned[r] {
			if _, err := os.Stat(p.abs(r)); err == nil {
				out = append(out, r)
			}
		}
	}
	return out
}

func (p *Plan) planRootFiles(kits []*kit.Kit) error {
	for _, rel := range p.Detection.RootFiles {
		var orig []byte
		perm := fs.FileMode(0o644)
		if !p.newRoots[rel] {
			abs := p.abs(rel)
			info, err := os.Stat(abs)
			if err != nil {
				return err
			}
			perm = info.Mode().Perm()
			if orig, err = os.ReadFile(abs); err != nil {
				return err
			}
		}

		doc := orig
		var touched []string
		var ferr error
		for _, k := range kits {
			var out []byte
			if k.Instructions != nil {
				out, ferr = rootfile.Upsert(doc, k.Name, k.Instructions)
			} else {
				out, ferr = rootfile.Remove(doc, k.Name)
			}
			if ferr != nil {
				break
			}
			if !bytes.Equal(out, doc) {
				touched = append(touched, k.Name)
			}
			doc = out
		}
		if ferr != nil {
			p.Conflicts = append(p.Conflicts, fmt.Sprintf("%s: %v", rel, ferr))
			continue
		}

		kind := Unchanged
		switch {
		case p.newRoots[rel]:
			kind = Create
		case !bytes.Equal(doc, orig):
			kind = Update
		}
		p.Ops = append(p.Ops, Op{Kind: kind, Path: rel, Kits: touched, data: doc, perm: perm})
	}
	return nil
}

// fileItem is one file a kit wants in the project: a skill file or an agent.
type fileItem struct {
	dest    string
	data    []byte
	exec    bool
	pattern string // the config field that produced dest, for error messages
}

func (p *Plan) kitFiles(k *kit.Kit) []fileItem {
	var items []fileItem
	for _, t := range p.Detection.SkillsTargets {
		for _, s := range k.Skills {
			base := t.SkillPath(k.Name, s.Name)
			for _, f := range s.Files {
				items = append(items, fileItem{path.Join(base, f.Path), f.Data, f.Exec, "skills_dir"})
			}
		}
	}
	for _, t := range p.Detection.AgentsTargets {
		for _, a := range k.Agents {
			items = append(items, fileItem{t.AgentPath(k.Name, a.Name), a.Data, false, "agents_dir"})
		}
	}
	return items
}

// planFiles plans k's skill and agent files into every detected root and
// returns the lock entry files for k.
func (p *Plan) planFiles(k *kit.Kit, prev *lock.Lock, claimed map[string]string, force bool) map[string]string {
	prevFiles := prev.Kits[k.Name].Files
	files := map[string]string{}

	for _, it := range p.kitFiles(k) {
		dest := it.dest
		if other, ok := claimed[dest]; ok {
			if other != k.Name {
				p.conflict(dest, "wanted by kits %q and %q; add {kit} to %s", other, k.Name, it.pattern)
			}
			continue // two targets sharing one directory
		}
		claimed[dest] = k.Name
		if owner := prev.Owner(dest); owner != "" && owner != k.Name {
			p.conflict(dest, "installed by kit %q", owner)
			continue
		}

		want := lock.Hash(it.data)
		files[dest] = want
		perm := fs.FileMode(0o644)
		if it.exec {
			perm = 0o755
		}
		op := Op{Path: dest, Kits: []string{k.Name}, data: it.data, perm: perm}

		cur, exists, regular, err := p.state(dest)
		switch {
		case err != nil:
			p.conflict(dest, "%v", err)
			continue
		case !exists:
			op.Kind = Create
		case !regular:
			p.conflict(dest, "exists and is not a regular file")
			continue
		case cur == want:
			op.Kind = Unchanged
		case prevFiles[dest] == cur || force:
			op.Kind = Update
		case prevFiles[dest] != "":
			p.conflict(dest, "modified since kits installed it (--force overwrites)")
			continue
		default:
			p.conflict(dest, "exists and was not installed by kits (--force overwrites)")
			continue
		}
		p.Ops = append(p.Ops, op)
	}

	// Files the previous install wrote that this plan does not. Inside a
	// planned root they are stale: deleted when untouched since. Outside
	// (another target, not selected this run) they are kept as they are.
	var stale []string
	for dest := range prevFiles {
		if _, ok := files[dest]; !ok {
			stale = append(stale, dest)
		}
	}
	sort.Strings(stale)
	for _, dest := range stale {
		cur, exists, regular, err := p.state(dest)
		switch {
		case err != nil || !exists:
		case !p.inPlannedRoot(dest):
			files[dest] = prevFiles[dest]
		case regular && cur == prevFiles[dest]:
			p.Ops = append(p.Ops, Op{Kind: Delete, Path: dest, Kits: []string{k.Name}})
		default:
			p.Warnings = append(p.Warnings,
				fmt.Sprintf("%s: no longer part of kit %q but modified since install; left in place", dest, k.Name))
		}
	}
	return files
}

func (p *Plan) inPlannedRoot(dest string) bool {
	for _, t := range p.Detection.SkillsTargets {
		if strings.HasPrefix(dest, t.SkillsRoot()+"/") {
			return true
		}
	}
	for _, t := range p.Detection.AgentsTargets {
		if strings.HasPrefix(dest, t.AgentsRoot()+"/") {
			return true
		}
	}
	return false
}

func (p *Plan) conflict(dest, format string, args ...any) {
	p.Conflicts = append(p.Conflicts, dest+": "+fmt.Sprintf(format, args...))
}

// state reports the hash of the file at rel. exists is false when nothing is
// there; regular is false for directories, symlinks and other non-files.
func (p *Plan) state(rel string) (hash string, exists, regular bool, err error) {
	info, err := os.Lstat(p.abs(rel))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, false, nil
	}
	if err != nil {
		return "", false, false, err
	}
	if !info.Mode().IsRegular() {
		return "", true, false, nil
	}
	data, err := os.ReadFile(p.abs(rel))
	if err != nil {
		return "", false, false, err
	}
	return lock.Hash(data), true, true, nil
}

func (p *Plan) abs(rel string) string {
	return filepath.Join(p.Dir, filepath.FromSlash(rel))
}

// Count returns how many ops of kind the plan holds.
func (p *Plan) Count(kind Kind) int {
	n := 0
	for _, op := range p.Ops {
		if op.Kind == kind {
			n++
		}
	}
	return n
}

// Apply writes the plan. It refuses a plan with conflicts.
func (p *Plan) Apply() error {
	if len(p.Conflicts) > 0 {
		return fmt.Errorf("plan has %d conflicts", len(p.Conflicts))
	}
	for _, op := range p.Ops {
		if op.Kind == MkDir {
			if err := os.MkdirAll(p.abs(op.Path), 0o755); err != nil {
				return err
			}
		}
	}
	for _, op := range p.Ops {
		if op.Kind != Create && op.Kind != Update {
			continue
		}
		abs := p.abs(op.Path)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := fsutil.WriteAtomic(abs, op.data, op.perm); err != nil {
			return err
		}
	}
	for _, op := range p.Ops {
		if op.Kind != Delete {
			continue
		}
		if err := os.Remove(p.abs(op.Path)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		p.prune(path.Dir(op.Path))
	}
	if p.lock != nil {
		return fsutil.WriteAtomic(filepath.Join(p.Dir, lock.FileName), p.lock, 0o644)
	}
	return nil
}

// prune removes now-empty directories from dir upwards, stopping at a
// configured root or the first directory that still has something in it.
func (p *Plan) prune(dir string) {
	for dir != "." && dir != "/" && !p.stopDirs[dir] {
		if os.Remove(p.abs(dir)) != nil {
			return
		}
		dir = path.Dir(dir)
	}
}
