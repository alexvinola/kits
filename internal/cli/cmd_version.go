package cli

import (
	"fmt"

	"github.com/alexvinola/kits/internal/version"
)

func runVersion(env Env, args []string) int {
	fs := newFlagSet(env, "version")
	if _, code, ok := parseFlags(fs, args); !ok {
		return code
	}
	fmt.Fprintln(env.Stdout, version.Version)
	return ExitOK
}
