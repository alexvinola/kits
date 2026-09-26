package cli

import (
	"fmt"

	"github.com/alexvinola/kits/internal/fetch"
	"github.com/alexvinola/kits/internal/userconfig"
)

const configUsage = `usage:
  kits config                     show every setting and the file they live in
  kits config get <key>
  kits config set <key> <value>
  kits config unset <key>

keys: repo, ref, path`

func runConfig(env Env, args []string) int {
	fs := newFlagSet(env, "config")
	fs.Usage = func() { fmt.Fprintln(env.Stderr, configUsage) }
	pos, code, ok := parseFlags(fs, args)
	if !ok {
		return code
	}

	path, err := userconfig.Path()
	if err != nil {
		return fail(env, "config", ExitError, err)
	}
	cfg, err := userconfig.Load(path)
	if err != nil {
		return fail(env, "config", ExitError, err)
	}

	usage := func() int {
		fmt.Fprintln(env.Stderr, configUsage)
		return ExitUsage
	}
	if len(pos) == 0 {
		fmt.Fprintf(env.Stdout, "# %s\n", path)
		for _, k := range userconfig.Keys() {
			v, _ := cfg.Field(k)
			fmt.Fprintf(env.Stdout, "%s = %s\n", k, *v)
		}
		return ExitOK
	}

	switch pos[0] {
	case "get":
		if len(pos) != 2 {
			return usage()
		}
		v, err := cfg.Field(pos[1])
		if err != nil {
			return fail(env, "config", ExitUsage, err)
		}
		fmt.Fprintln(env.Stdout, *v)
		return ExitOK
	case "set", "unset":
		if (pos[0] == "set" && len(pos) != 3) || (pos[0] == "unset" && len(pos) != 2) {
			return usage()
		}
		v, err := cfg.Field(pos[1])
		if err != nil {
			return fail(env, "config", ExitUsage, err)
		}
		*v = ""
		if pos[0] == "set" {
			*v = pos[2]
			if err := validateSetting(cfg); err != nil {
				return fail(env, "config", ExitUsage, err)
			}
		}
		if err := cfg.Save(path); err != nil {
			return fail(env, "config", ExitError, err)
		}
		return ExitOK
	}
	return usage()
}

// validateSetting checks the stored source the way init will use it. An
// empty repo is allowed here: ref or path may be set before it.
func validateSetting(cfg *userconfig.Config) error {
	src := fetch.Source{Repo: cfg.Repo, Ref: cfg.Ref, Path: cfg.Path}
	if src.Repo == "" {
		src.Repo = "placeholder"
	}
	return src.Validate()
}
