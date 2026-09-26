package cli

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/alexvinola/kits/internal/targets"
)

func runTargets(env Env, args []string) int {
	fs := newFlagSet(env, "targets")
	jsonOut := fs.Bool("json", false, "emit JSON")
	pos, code, ok := parseFlags(fs, args)
	if !ok {
		return code
	}
	if len(pos) > 0 {
		return fail(env, "targets", ExitUsage, fmt.Errorf("unexpected argument %q", pos[0]))
	}

	cfg, err := targets.Default()
	if err != nil {
		return fail(env, "targets", ExitError, err)
	}

	if *jsonOut {
		enc := json.NewEncoder(env.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(cfg); err != nil {
			return fail(env, "targets", ExitError, err)
		}
		return ExitOK
	}

	tw := tabwriter.NewWriter(env.Stdout, 0, 0, 2, ' ', 0)
	dash := func(s string) string {
		if s == "" {
			return "-"
		}
		return s
	}
	fmt.Fprintln(tw, "TARGET\tROOT FILE\tSKILLS DIR\tAGENTS DIR\tDESCRIPTION")
	for _, t := range cfg.Targets {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			t.Name, t.RootFile, dash(t.SkillsDir), dash(t.AgentsDir), t.Description)
	}
	if err := tw.Flush(); err != nil {
		return fail(env, "targets", ExitError, err)
	}
	return ExitOK
}
