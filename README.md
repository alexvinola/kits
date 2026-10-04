# kits

Install **kits** — instructions, skills and subagents for coding agents — into
new or existing projects, placed where each agent expects them:
Claude Code, Cursor, GitHub Copilot, Codex, or any tool that reads `AGENTS.md`.

- Single static binary, no dependencies beyond `git`.
- Kits are fetched from a git repository with a shallow, sparse checkout of just
  the kits you ask for — never a full clone.
- Where each agent keeps its files is declarative config, not code.
- Re-running is safe: kits only touches what it installed, and never overwrites
  your own files without `--force`.

## Install

Homebrew (macOS, Linux), from the tap in this same repository:

```sh
brew tap alexvinola/kits https://github.com/alexvinola/kits
brew install alexvinola/kits/kits
```

Windows, with PowerShell 5.1 or later (no administrator needed):

```powershell
$kitsInstaller = Join-Path $env:TEMP 'kits-install.ps1'
Invoke-WebRequest -UseBasicParsing -Uri 'https://github.com/alexvinola/kits/releases/latest/download/install.ps1' -OutFile $kitsInstaller
powershell -NoProfile -ExecutionPolicy Bypass -File $kitsInstaller
```

The [installer](scripts/install.ps1) selects Windows x64 or ARM64, verifies the
binary's SHA-256 against the same release's `checksums.txt`, and installs into
`%LOCALAPPDATA%\Programs\kits\bin`. It adds that directory to your user PATH;
open a new terminal afterwards. Git must also be installed for `kits init`.
Re-run to update. Checksums verify download integrity, not an independent signature.

To select a release, directory, or leave PATH unchanged:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File $kitsInstaller -Version 0.1.0 -InstallDir "$env:USERPROFILE\bin" -NoPath
```

`-ExecutionPolicy Bypass` applies only to that process; it does not change your
saved execution policy. The download URL becomes available with the first release
that includes this installer.

With Go:

```sh
go install github.com/alexvinola/kits/cmd/kits@latest
```

Or download a binary from [Releases](https://github.com/alexvinola/kits/releases)
and check it against `checksums.txt`.

## Setup

kits has no built-in kit repository: point it at yours once.

```sh
kits config set repo git@github.com:you/my-kits.git
kits config set path kits      # optional: kits live in kits/ instead of the root
kits config set ref v2         # optional: a branch or tag instead of the default branch
kits config                    # show the settings and where they are stored
```

Settings live in `$XDG_CONFIG_HOME/kits/config.json` (`~/.config/kits/config.json`
by default; `%AppData%\kits\config.json` on Windows). `--repo`, `--ref` and
`--path` override them for one run.

## Usage

```sh
kits init core java-springboot-modulith      # detection mode
kits init core --target claude               # create what is missing, asking first
kits init core ia-base ia-agents --target codex --yes
kits targets                           # list configured targets
```

### Detection mode (no `--target`)

`init` scans the project for every target's integration paths — its root file
(`CLAUDE.md`, `AGENTS.md`, …), skills directory (`.claude/skills`, …) and agents
directory (`.claude/agents`) — and **only updates the ones that already exist**. Missing ones that the kits would use are
listed and left alone:

```
update  CLAUDE.md                          core, ia
create  .claude/skills/review/SKILL.md     core
skip    .github/copilot-instructions.md    not found (copilot)
```

If nothing is found, `init` exits with an error and writes nothing.

### Explicit targets (`--target`)

`--target claude,cursor` (or the flag repeated) restricts `init` to those targets
and offers to **create** their missing paths, asking once per path:

```
Create file CLAUDE.md (claude)? [y/N]
Create directory .claude/skills (claude)? [y/N]
```

Without a terminal to answer on, `init` refuses; pass `--yes` to approve every
missing path up front. `--dry-run` shows the plan without asking or writing.

### Flags

| Flag | Default | |
|---|---|---|
| `--repo` | `kits config get repo` | Repository holding the kits: `owner/repo` (GitHub over HTTPS) or any URL git can clone |
| `--ref` | `kits config get ref`, else default branch | Branch or tag to fetch (not a commit SHA) |
| `--path` | `kits config get path`, else repository root | Directory in the repository with one subdirectory per kit |
| `--from` | | Use kits from a local directory instead of fetching |
| `--target` | | Only these targets; create their missing paths after confirmation |
| `--yes` | | With `--target`, create missing paths without asking |
| `--dir` | `.` | Project directory |
| `--dry-run` | | Print the plan, write nothing |
| `--force` | | Overwrite files kits did not install, or that were edited since |

`kits` itself: `kits init`, `kits config`, `kits targets`, `kits version`.

### Private kit repositories

kits does not log in to anything: it runs `git`, which uses the credentials you
already have. For a private repository configure an SSH URL
(`git@github.com:you/my-kits.git`), or run `gh auth setup-git` so HTTPS URLs
authenticate. Access is whatever GitHub grants your account.

## What gets written

**Root files.** Each kit's `instructions.md` goes into the root file inside a
block that kits owns; everything outside the markers is yours and never
touched:

```markdown
# My project

<!-- kits:begin core -->
…contents of core/instructions.md…
<!-- kits:end core -->
```

A symlinked root file (e.g. `CLAUDE.md -> AGENTS.md`) is written through, once.

**Skills and subagents.** Each skill directory is copied to the target's
`skills_dir`; each agent file to its `agents_dir`. The selected target determines
the paths and filename suffixes. YAML frontmatter is copied unchanged; it describes
the skill/agent, not where kits installs it.

**`kits.lock.json`.** Records every file kits wrote with its hash,
and where each kit came from (repository, ref, commit). Commit it. On the next
`init`, for each file:

| On disk | Result |
|---|---|
| Missing | created |
| Identical to the kit | unchanged |
| Written by kits, untouched since | updated |
| Written by kits, edited since | conflict |
| Not written by kits | conflict |
| Dropped from the kit, untouched | deleted |

Any conflict aborts the whole run before writing anything. `--force` overwrites.

## Kit format

A kit repository holds one directory per kit:

```
core/
  instructions.md          optional — goes into each root file
  skills/
    review/                one directory per skill, copied whole
      SKILL.md
      scripts/check.sh
  agents/
    reviewer.md            one file per subagent
```

kits does not configure MCP servers or any other tool: that is the IDE's job.
A kit's instructions can tell the agent which tools exist and how the developer
enables them.

Kit, skill and agent names are lowercase letters, digits and `-`. Only regular
files are allowed (no symlinks). Other files in the kit directory, such as a
README, are ignored.

## Targets config

The built-in config ([internal/targets/default.json](internal/targets/default.json)):

| Target | Root file | Skills dir | Agents dir |
|---|---|---|---|
| `claude` | `CLAUDE.md` | `.claude/skills/{skill}` | `.claude/agents/{agent}.md` |
| `cursor` | `AGENTS.md` | `.cursor/skills/{skill}` | — |
| `copilot` | `.github/copilot-instructions.md` | `.github/skills/{skill}` | `.github/agents/{agent}.agent.md` |
| `codex` | `AGENTS.md` | `.agents/skills/{skill}` | — |
| `agents` | `AGENTS.md` | — | — |

Paths follow the documentation for [Cursor](https://cursor.com/docs/skills),
[GitHub Copilot skills](https://docs.github.com/en/copilot/how-tos/copilot-on-github/customize-copilot/customize-cloud-agent/add-skills)
and [Codex skills](https://learn.chatgpt.com/docs/build-skills).
Codex's `AGENTS.md` contains instructions scoped by directory; `.agents/skills`
is the separate location for on-demand skills. Use `codex` for both, or retain
`agents` for instructions only. To install into a subproject, use `--dir backend`;
paths in this table are relative to that directory. Kits does not recursively
create instructions in every source directory.

Claude and Copilot can share simple subagent definitions with `name`, `description`
and `tools: Read, Grep, Glob, Bash`: [Copilot documents these tool aliases](https://docs.github.com/en/copilot/reference/custom-agents-configuration).
Use this common subset for agents installed into both tools. Kits does not translate
or validate tool-specific frontmatter such as Claude's `permissionMode`, `hooks`,
model aliases or MCP tool names. Do not assume those settings are portable.
Reference skills by name or the destination's skill directory in the agent body.
The CLI tests verify file placement and preservation, not execution inside an IDE.

Cursor and Codex do not receive these Markdown subagent definitions in the current
mapping; portable review skills remain available to them.

Supporting another coding tool, or fixing a path, is an edit to that file and a new release.
Each target has:

- `root_file` — required; project-relative, no placeholders.
- `skills_dir` — optional; must contain `{skill}`, may contain `{kit}`, and must
  start with a fixed directory. Use `{kit}` when two kits may ship skills with
  the same name.
- `agents_dir` — optional; the file of one agent, same rules with `{agent}`.
- Paths are clean, relative, forward-slash, and cannot leave the project.
  Unknown fields are rejected.

## Development

```sh
make          # fmt, vet, test, build
make verify   # what CI runs, plus cross-compilation
make cross    # dist/kits-<os>-<arch>
```

Offline installer tests (also run on Windows CI with PowerShell 5.1 and 7):

```powershell
pwsh -NoProfile -File scripts/install.Tests.ps1
```

### Releasing

1. Push a version tag: `git tag v0.1.0 && git push origin v0.1.0`. CI tests,
   builds every platform and publishes the binaries, `install.ps1` and `checksums.txt`.
2. Pin the formula to it and commit:
   `scripts/bump-homebrew-formula.sh 0.1.0 Formula/kits.rb`

## License

[MIT](LICENSE)
