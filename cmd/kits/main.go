// Command kits installs instruction and skill kits into a project, placing
// them where each coding agent (Claude Code, Cursor, Copilot, ...) expects.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/alexvinola/kits/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	env := cli.Env{
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Stdin:      os.Stdin,
		StdinIsTTY: isTerminal(os.Stdin),
	}
	os.Exit(cli.Run(ctx, env, os.Args[1:]))
}

// isTerminal reports whether f is an interactive terminal, using only the
// standard library: a character device is treated as interactive.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
