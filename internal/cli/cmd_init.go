package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/alexvinola/kits/internal/fetch"
	"github.com/alexvinola/kits/internal/install"
	"github.com/alexvinola/kits/internal/kit"
	"github.com/alexvinola/kits/internal/lock"
	"github.com/alexvinola/kits/internal/targets"
	"github.com/alexvinola/kits/internal/userconfig"
)

func runInit(ctx context.Context, env Env, args []string) int {
	fs := newFlagSet(env, "init")
	repo := fs.String("repo", "", "git repository holding the kits: owner/repo on GitHub, or any URL git can clone (default: `kits config get repo`)")
	ref := fs.String("ref", "", "branch or tag to fetch (default: `kits config get ref`, else the default branch)")
	kitsPath := fs.String("path", "", "directory inside the repository with one subdirectory per kit (default: `kits config get path`, else the root)")
	from := fs.String("from", "", "use kits from this local directory instead of fetching them")
	dir := fs.String("dir", ".", "project directory")
	dryRun := fs.Bool("dry-run", false, "show what would change without writing anything")
	force := fs.Bool("force", false, "overwrite files kits did not install or that were modified since")
	var only stringList
	fs.Var(&only, "target", "install only for this target, creating its missing paths after confirmation (repeatable or comma-separated)")
	yes := fs.Bool("yes", false, "with --target, create missing paths without asking")
	names, code, ok := parseFlags(fs, args)
	if !ok {
		return code
	}
	if len(names) == 0 {
		return fail(env, "init", ExitUsage, errors.New("name at least one kit, e.g. kits init core --from <dir>"))
	}
	if *yes && len(only) == 0 {
		return fail(env, "init", ExitUsage, errors.New("--yes only applies with --target"))
	}
	if *from != "" {
		var clash []string
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "repo" || f.Name == "ref" || f.Name == "path" {
				clash = append(clash, "--"+f.Name)
			}
		})
		if len(clash) > 0 {
			return fail(env, "init", ExitUsage, fmt.Errorf("--from cannot be combined with %s", strings.Join(clash, ", ")))
		}
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			return fail(env, "init", ExitUsage, fmt.Errorf("kit %q given twice", n))
		}
		seen[n] = true
	}

	cfg, err := targets.Default()
	if err != nil {
		return fail(env, "init", ExitError, err)
	}
	if len(only) > 0 {
		if _, err := cfg.Select(only); err != nil {
			return fail(env, "init", ExitUsage, err)
		}
	}

	kitsDir := *from
	var source *lock.Source
	if kitsDir == "" {
		src, err := resolveSource(fs, *repo, *ref, *kitsPath)
		if err != nil {
			return fail(env, "init", ExitUsage, err)
		}
		fmt.Fprintf(env.Stderr, "fetching %s from %s\n", strings.Join(names, ", "), src.URL())
		co, err := fetch.Fetch(ctx, src, names)
		if err != nil {
			return fail(env, "init", ExitError, err)
		}
		defer co.Close()
		kitsDir = co.Dir
		source = &lock.Source{Repo: src.URL(), Ref: src.Ref, Path: src.Path, Commit: co.Commit}
	}

	var kits []*kit.Kit
	for _, n := range names {
		k, err := kit.Load(os.DirFS(kitsDir), n)
		if err != nil {
			if source != nil {
				where := source.Path
				if where == "" {
					where = "/"
				}
				err = fmt.Errorf("%w in %s (path %s, commit %.12s)", err, source.Repo, where, source.Commit)
			}
			return fail(env, "init", ExitError, err)
		}
		kits = append(kits, k)
	}

	opts := install.Options{ProjectDir: *dir, Config: cfg, Kits: kits, Source: source, Force: *force, Only: only}
	if len(only) > 0 {
		opts.Confirm = newConfirmer(env, *yes, *dryRun)
	}
	plan, err := install.PlanInit(opts)
	if err != nil {
		return fail(env, "init", ExitError, err)
	}
	for _, w := range plan.Warnings {
		fmt.Fprintf(env.Stderr, "warning: %s\n", w)
	}
	if plan.Detection.Empty() {
		printPlan(env.Stdout, plan)
		if len(only) > 0 {
			return fail(env, "init", ExitError, errors.New("every missing path was declined; nothing to update"))
		}
		return fail(env, "init", ExitError, fmt.Errorf("no integration paths found in %s; nothing to update (use --target to create them)", plan.Dir))
	}
	if len(plan.Conflicts) > 0 {
		fmt.Fprintf(env.Stderr, "kits init: %d conflict(s), nothing was written:\n", len(plan.Conflicts))
		for _, c := range plan.Conflicts {
			fmt.Fprintf(env.Stderr, "  %s\n", c)
		}
		return ExitError
	}

	printPlan(env.Stdout, plan)
	summary := fmt.Sprintf("%d created, %d updated, %d deleted, %d unchanged",
		plan.Count(install.Create), plan.Count(install.Update), plan.Count(install.Delete), plan.Count(install.Unchanged))
	if *dryRun {
		fmt.Fprintf(env.Stdout, "dry run: %s; nothing written\n", summary)
		return ExitOK
	}
	if err := plan.Apply(); err != nil {
		return fail(env, "init", ExitError, err)
	}
	fmt.Fprintln(env.Stdout, summary)
	return ExitOK
}

// printPlan lists every change and every missing integration path. Unchanged
// files are only counted, to keep a re-run quiet.
func printPlan(w io.Writer, plan *install.Plan) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, op := range plan.Ops {
		switch op.Kind {
		case install.Unchanged:
		case install.MkDir:
			fmt.Fprintf(tw, "%s\t%s\t(%s)\n", op.Kind, op.Path, strings.Join(op.Targets, ", "))
		default:
			fmt.Fprintf(tw, "%s\t%s\t%s\n", op.Kind, op.Path, strings.Join(op.Kits, ", "))
		}
	}
	for _, m := range plan.Detection.Missing {
		why := "not found"
		if m.Declined {
			why = "declined"
		}
		fmt.Fprintf(tw, "skip\t%s\t%s (%s)\n", m.Path, why, strings.Join(m.Targets, ", "))
	}
	tw.Flush()
}

// newConfirmer asks on the terminal before each missing path is created.
// --yes approves them all up front; a dry run approves them without asking,
// since it writes nothing. Without a terminal to ask on, it refuses rather
// than guessing.
func newConfirmer(env Env, yes, dryRun bool) func(install.Missing) (bool, error) {
	in := bufio.NewReader(env.Stdin)
	return func(m install.Missing) (bool, error) {
		if yes || dryRun {
			return true, nil
		}
		if !env.StdinIsTTY || env.Stdin == nil {
			return false, fmt.Errorf("%s does not exist and stdin is not a terminal to confirm creating it; pass --yes", m.Path)
		}
		what := "directory"
		if m.IsFile() {
			what = "file"
		}
		fmt.Fprintf(env.Stderr, "Create %s %s (%s)? [y/N] ", what, m.Path, strings.Join(m.Targets, ", "))
		line, err := in.ReadString('\n')
		if err != nil && (err != io.EOF || line == "") {
			return false, fmt.Errorf("no answer for %s: %w", m.Path, err)
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		}
		return false, nil
	}
}

// resolveSource merges the user config with the --repo/--ref/--path flags
// that were given; a flag given empty still overrides.
func resolveSource(fs *flag.FlagSet, repo, ref, kitsPath string) (fetch.Source, error) {
	path, err := userconfig.Path()
	if err != nil {
		return fetch.Source{}, err
	}
	cfg, err := userconfig.Load(path)
	if err != nil {
		return fetch.Source{}, err
	}
	src := fetch.Source{Repo: cfg.Repo, Ref: cfg.Ref, Path: cfg.Path}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "repo":
			src.Repo = repo
		case "ref":
			src.Ref = ref
		case "path":
			src.Path = kitsPath
		}
	})
	if src.Repo == "" {
		return src, errors.New("no kits repository configured; run `kits config set repo <url>`, or pass --repo <url> or --from <dir>")
	}
	return src, nil
}
