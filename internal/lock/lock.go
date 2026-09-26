// Package lock reads and writes kits.lock.json, the record of what kits
// installed into a project. It is what lets a second init tell its own files
// (safe to update or delete) from the user's (never touched without --force).
package lock

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/alexvinola/kits/internal/relpath"
)

const (
	FileName      = "kits.lock.json"
	SchemaVersion = 1
)

type Lock struct {
	Version int              `json:"version"`
	Kits    map[string]Entry `json:"kits"`
}

// Entry is one installed kit. Files maps each skill and agent file kits wrote
// to the hash of the bytes it wrote. Source is nil for kits installed with
// --from.
type Entry struct {
	Source    *Source           `json:"source,omitempty"`
	RootFiles []string          `json:"root_files,omitempty"`
	Files     map[string]string `json:"files,omitempty"`
}

// Source records where a kit was fetched from.
type Source struct {
	Repo   string `json:"repo"`
	Ref    string `json:"ref,omitempty"`
	Path   string `json:"path,omitempty"`
	Commit string `json:"commit"`
}

func New() *Lock {
	return &Lock{Version: SchemaVersion, Kits: map[string]Entry{}}
}

// Load reads the lock in projectDir. A missing lock is an empty one.
func Load(projectDir string) (*Lock, error) {
	name := filepath.Join(projectDir, FileName)
	data, err := os.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return New(), nil
	}
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var l Lock
	if err := dec.Decode(&l); err != nil {
		return nil, fmt.Errorf("%s: %w", FileName, err)
	}
	if l.Version != SchemaVersion {
		return nil, fmt.Errorf("%s: version %d, want %d", FileName, l.Version, SchemaVersion)
	}
	if l.Kits == nil {
		l.Kits = map[string]Entry{}
	}
	// The lock lives in the repository, so treat its paths as untrusted: they
	// decide which files init may delete.
	for kit, e := range l.Kits {
		for _, p := range e.RootFiles {
			if err := relpath.Check(p); err != nil {
				return nil, fmt.Errorf("%s: kit %q: %w", FileName, kit, err)
			}
		}
		for p := range e.Files {
			if err := relpath.Check(p); err != nil {
				return nil, fmt.Errorf("%s: kit %q: %w", FileName, kit, err)
			}
		}
	}
	return &l, nil
}

// Clone returns a copy whose Kits map can be changed independently. Entries
// are replaced whole, never mutated, so they are shared.
func (l *Lock) Clone() *Lock {
	c := New()
	for k, e := range l.Kits {
		c.Kits[k] = e
	}
	return c
}

// Owner returns the kit that installed path p, or "".
func (l *Lock) Owner(p string) string {
	for name, e := range l.Kits {
		if _, ok := e.Files[p]; ok {
			return name
		}
	}
	return ""
}

// Marshal encodes the lock deterministically (encoding/json sorts map keys).
func (l *Lock) Marshal() ([]byte, error) {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Hash is the content hash stored in the lock.
func Hash(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
