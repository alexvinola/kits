// Package fetch brings kit directories from a git repository without cloning
// all of it: a shallow, blobless partial clone with a sparse checkout of just
// the requested kit folders. Only the tip commit's trees and the files under
// those folders are downloaded (cone mode also brings the repository's
// top-level files).
//
// It shells out to git. GitHub does not serve `git archive --remote`, the
// GitHub tarball API returns the whole repository, and go-git would be a
// large dependency; git also brings the user's credential setup for private
// repositories for free.
package fetch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alexvinola/kits/internal/kit"
	"github.com/alexvinola/kits/internal/relpath"
)

// Source says where kits live: Path is the directory inside Repo that holds
// one subdirectory per kit.
type Source struct {
	Repo string // owner/repo on GitHub, or any URL or path git can clone
	Ref  string // branch or tag; empty means the remote's default branch
	Path string
}

var shorthandRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// URL expands the owner/repo shorthand to a GitHub HTTPS URL.
func (s Source) URL() string {
	if shorthandRe.MatchString(s.Repo) && !strings.HasPrefix(s.Repo, ".") {
		return "https://github.com/" + s.Repo + ".git"
	}
	return s.Repo
}

// Validate checks s without touching the network.
func (s Source) Validate() error {
	if s.Repo == "" {
		return errors.New("empty repository")
	}
	if s.Path != "" {
		if err := relpath.Check(s.Path); err != nil {
			return fmt.Errorf("kits path: %w", err)
		}
	}
	if strings.HasPrefix(s.Ref, "-") {
		return fmt.Errorf("invalid ref %q", s.Ref)
	}
	return nil
}

// Checkout is a fetched sparse working tree. Close removes it.
type Checkout struct {
	Dir    string // directory holding the kit folders
	Commit string // commit the kits were read from
	root   string
}

func (c *Checkout) Close() error {
	return os.RemoveAll(c.root)
}

// Fetch checks out the given kits from src into a temporary directory.
func Fetch(ctx context.Context, src Source, kits []string) (*Checkout, error) {
	if err := src.Validate(); err != nil {
		return nil, err
	}
	if len(kits) == 0 {
		return nil, errors.New("no kits to fetch")
	}
	sparse := make([]string, len(kits))
	for i, k := range kits {
		if !kit.ValidName(k) {
			return nil, fmt.Errorf("invalid kit name %q", k)
		}
		sparse[i] = path.Join(src.Path, k)
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, errors.New("git not found in PATH; kits uses git to fetch kits")
	}

	root, err := os.MkdirTemp("", "kits-fetch-*")
	if err != nil {
		return nil, err
	}
	co := &Checkout{root: root}
	fail := func(err error) (*Checkout, error) {
		co.Close()
		return nil, fmt.Errorf("fetch %s: %w", src.URL(), err)
	}

	wt := filepath.Join(root, "repo")
	clone := []string{"clone", "--quiet", "--depth", "1", "--filter=blob:none", "--sparse", "--no-checkout"}
	if src.Ref != "" {
		clone = append(clone, "--branch", src.Ref)
	}
	clone = append(clone, "--", src.URL(), wt)
	if _, err := git(ctx, "", clone...); err != nil {
		return fail(fmt.Errorf("%w\n(a private repository needs git credentials: an SSH key for git@github.com:owner/repo.git URLs, or `gh auth setup-git` for https:// ones)", err))
	}
	if _, err := git(ctx, wt, append([]string{"sparse-checkout", "set", "--cone", "--"}, sparse...)...); err != nil {
		return fail(err)
	}
	if _, err := git(ctx, wt, "checkout", "--quiet"); err != nil {
		return fail(err)
	}
	commit, err := git(ctx, wt, "rev-parse", "HEAD")
	if err != nil {
		return fail(err)
	}

	co.Dir = filepath.Join(wt, filepath.FromSlash(src.Path))
	co.Commit = commit
	return co, nil
}

// git runs git without a terminal prompt, so a private repository without
// credentials fails fast instead of hanging.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(stdout.String()), nil
}
