// Package cli implements the kits command line.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Env is everything a command may touch in the outside world, so tests can
// run commands in-process.
type Env struct {
	Stdout     io.Writer
	Stderr     io.Writer
	Stdin      io.Reader
	StdinIsTTY bool // whether prompts can be answered
}

// Exit codes.
const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

const usage = `kits installs instruction and skill kits into a project.

Usage:
  kits <command> [flags]

Commands:
  init      install kits into the integration paths the project already has
  config    show or change user settings (the kits repository)
  targets   list the configured coding-agent targets
  version   print the kits version

Run "kits <command> -h" for a command's flags.
`

// Run executes the command in args and returns the process exit code.
func Run(ctx context.Context, env Env, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(env.Stderr, usage)
		return ExitUsage
	}
	switch args[0] {
	case "init":
		return runInit(ctx, env, args[1:])
	case "config":
		return runConfig(env, args[1:])
	case "targets":
		return runTargets(env, args[1:])
	case "version", "--version":
		return runVersion(env, args[1:])
	case "help", "-h", "--help":
		fmt.Fprint(env.Stdout, usage)
		return ExitOK
	}
	fmt.Fprintf(env.Stderr, "kits: unknown command %q\n\n%s", args[0], usage)
	return ExitUsage
}

func newFlagSet(env Env, name string) *flag.FlagSet {
	fs := flag.NewFlagSet("kits "+name, flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	return fs
}

// parseFlags parses flags wherever they appear, so "init core --from x" works
// (the flag package alone stops at the first positional argument). It returns
// the positional arguments, or ok=false with the exit code to use when
// parsing stops the command, either because of -h or a bad flag.
func parseFlags(fs *flag.FlagSet, args []string) (pos []string, code int, ok bool) {
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, ExitOK, false
			}
			return nil, ExitUsage, false
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, ExitOK, true
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

// stringList is a repeatable flag that also accepts comma-separated values.
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*l = append(*l, part)
		}
	}
	return nil
}

func fail(env Env, cmd string, code int, err error) int {
	fmt.Fprintf(env.Stderr, "kits %s: %v\n", cmd, err)
	return code
}
