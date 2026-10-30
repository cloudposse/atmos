# Development Guide

This guide covers the development workflow for contributing to Atmos.

## Prerequisites

- Go 1.26+ (see go.mod for exact version)
- Atmos
- Git

## Quick Start

1. Clone the repository
2. Run the development setup:
    ```bash
    atmos dev setup
    ```
    This will:
    - Install Go dependencies
    - Build Atmos with native Git sandbox support
    - Install pinned gofumpt, golangci-lint, and Gitleaks tools
    - Build custom-gcl and install the Atmos Git hook

## Development Workflow

We use Atmos custom commands for development (dogfooding our own tool). This ensures we experience the same workflows our users do. See [Atmos Custom Commands](https://atmos.tools/cli/configuration/commands) for more details.

### Available Commands

```bash
# Checking Commands (Read-only, no modifications)
atmos check staged     # Check staged files for issues
atmos check pr         # Check PR changes for issues
atmos check all        # Check all files for issues
atmos lint changed     # Run repository lint checks for changed files

# Formatting Commands (Modifies files)
atmos format staged    # Auto-format staged files
atmos format pr        # Auto-format PR changes
atmos format all       # Auto-format all files

# Build and Test Commands
atmos test             # Run short tests
atmos test --full      # Run full tests
atmos build     # Build the Atmos binary
atmos dev quick        # Quick build and test
atmos dev generate snapshots --all  # Regenerate all CLI golden snapshots
atmos dev --help       # Show all available dev commands
```

### Build, Test, and Lint Commands

Use grouped Atmos commands for repository tasks:

```bash
atmos build     # Build the Atmos binary
atmos test             # Run short tests
atmos test --full      # Run full tests
atmos lint changed     # Run golangci-lint on changed files
# Regenerate focused CLI golden snapshots
atmos dev generate snapshots --filter 'TestCLICommands/check_atmos_--help_in_empty-dir'
```

Use `atmos build --no-cache` (or `atmos build binary --no-cache`) to force
recompilation of all Go packages, including dependencies. This passes `-a` to
`go build`; it preserves the shared build cache and downloaded modules. Normal
builds reuse cached packages, and `--no-cache=false` keeps that default.

Build output appears in a live four-line viewport (three recent log lines and a
status row), which collapses when the build succeeds and reveals full logs on
failure. CI and redirected output stream normally.

## Atmos Git Hooks

`atmos dev setup` installs the `pre-commit` Git event through `atmos git hooks
install`. The shared check catalog lives in `.atmos-git-hooks.yaml` at the repository
root. `atmos.yaml` connects the Git event to `scripts/checks/entry.star`, whose
shared suite loads the catalog for hooks, developer commands, and CI.
Starlark runs named checks through `steps.test(...)`: passing output stays hidden,
failures appear with details, and independent checks continue. A terminal gets a
live progress tree; CI gets static results.

### Checking, formatting, and committing

- `atmos check staged` checks a temporary copy of the index without changing your
  files. `check pr` selects committed branch changes relative to `origin/main`
  and checks their current working-tree contents; `check all` checks all tracked files.
- `atmos format staged|pr|all` applies formatting and module cleanup as unstaged
  changes. It never stages files and reports operational failures.
- `git commit` runs the full suite against the index. Automatic fixes are returned
  as unstaged edits and the commit stops so you can review and restage them.
- For partially staged files, fixes are merged with unstaged edits only when they
  do not conflict. On overlap or concurrent edits, your original file is left
  intact and the diagnostic identifies a recovery directory containing a patch
  and before/after files. Review and apply the intended fix manually, then retry.
- Cancellation discards the temporary checkout without applying incomplete fixes.

Use `--base-ref <branch-or-SHA>` with PR commands to choose another base. Missing
refs fail with a fetch hint; checks never fetch or modify branches implicitly.
Use `--only trailing-whitespace,end-of-file-fixer` to select specific checks.
Formatting exclusions are check-specific and preserve golden snapshots, generated
SVGs, and other existing exclusions. The suite also verifies symlinks, instruction
file sizes, YAML syntax, large files, EditorConfig, module compatibility, and
secrets. The `branch-file-limit` check permits at most 150 files in the branch diff
against the merge base with `origin/main`, including the pending staged changes.
Deletions count, renames count once, and unstaged edits are excluded. Initial commits
compare against an empty tree. This is our review-size policy; CodeRabbit's published
file allowance varies by plan. Split larger changes into smaller PRs or a PR stack;
`--base-ref` can select the parent branch when checking a stack.
Go module tidy runs once per affected module, including nested modules.

### Setup and recovery

Setup installs pinned tools into `build/check-tools`, which the Starlark suite puts first
on PATH, including for GUI Git clients. It pins gofumpt **v0.9.1**,
golangci-lint **v2.5.0**, and Gitleaks **v8.30.1**. Upgrade these pins together in
development setup and CI; keep gofumpt aligned with the linter's formatter.

```bash
# Prepare all tools and install the hook.
atmos dev setup

# Rebuild Atmos after changing its source, outside commit time.
atmos dev checks-build

# Rebuild custom-gcl when the hook reports that it is missing or stale.
go tool mage lint:customGCL

# Run the installed suite explicitly (may apply fixes).
atmos git hooks run pre-commit

# Read-only whole-repository checks.
atmos check all
```

Setup refuses to overwrite hooks from other tools. To replace an existing hook
after inspecting it, run `./build/atmos git hooks install pre-commit --force`;
recognized pre-commit-framework shims are backed up before replacement. Resolve
any `core.hooksPath` configuration explicitly. Hook directories can be shared by linked worktrees, so
prepare each worktree's binaries before committing there. The hook loads that
worktree's configuration. Neither Atmos nor custom-gcl is built
inside a commit hook, and hooks do not install tools.

The shared check catalog lives in `.atmos-git-hooks.yaml`. Each YAML entry
declares its handler, selection, exclusions, and whether it applies fixes. Handler
functions live in `checks.star`; `yaml.decode` loads the catalog as plain data. `entry.star` runs
the suite through Atmos's native `git.sandbox(...)`; there is no repository helper
binary. The sandbox uses private Git metadata and reads the source object database.
It does not register a Git worktree or create a branch. `fs.write_file` supplies
atomic file updates and `yaml.validate` checks syntax without resolving Atmos tags.
An older Atmos binary that does not recognize these functions must be rebuilt with
`atmos dev checks-build`; setup installs the hook using the rebuilt binary.

## CI/CD

### Recording the repository Git hooks

To regenerate the changelog's recording of the real repository hook, prepare the
check tools and source-built Atmos, then run:

```shell
./build/atmos --chdir=demo/casts casts generate repository git-hooks
./build/atmos --chdir=demo/casts casts validate repository git-hooks
```

The recording selects actual working changes in `.atmos-git-hooks.yaml`, the check
scripts and commands, and this guide using a temporary index. It checks those
inputs read-only before recording `atmos git hooks run pre-commit`, verifies the
live index and selected working files are unchanged, and removes the temporary
index afterward. Make an intended edit to one of those files before regenerating;
the recorder does not manufacture sample changes. Go checks are skipped when the
selected changes contain no Go files.

### Repository checks in CI

The existing `Pre-commit` workflow/job name is retained for required-check
compatibility, but it runs the same Atmos suite against the explicit PR base SHA.
The custom golangci-lint check runs in its dedicated lint job and is omitted here
with `--skip golangci-lint`. Gitleaks scans the PR commit range in CI
and the index locally. Formatting differences fail the read-only job. The autofix
workflow uses the same whitespace formatters without suppressing failures.

## Developer Tools

### Policy Engine

The experimental `pkg/policy` core is documented in the
[policy engine contributor guide](policy-engine.md). It covers the Go API,
CEL evaluation, provider-native CI snapshots, affected inputs, and the boundary between evaluation
and enforcement. The opt-in [saved-plan gate](../website/docs/policies/terraform-plan.mdx)
wires CEL plan policies into Terraform commands; existing component validation is unchanged.
The [policy library](../policy-library/README.md) includes individually tested policies and lifecycle hooks;
the [catalog contributor guide](policy-library.md) describes its fixtures and verification commands.
The [capability reference](policy-capabilities.md) documents implemented component and CI inputs.
Use the `atmos-policy` consumer skill when authoring
these policies, rather than treating PRD syntax as shipped CLI functionality.

### Testing Atmos in Geodesic

The `scripts/test-geodesic-prebuilt.sh` script allows you to quickly test Atmos changes inside a Geodesic container without rebuilding the entire Geodesic image.

**Usage:**
```bash
./scripts/test-geodesic-prebuilt.sh <path-to-infrastructure>
```

**Example:**
```bash
./scripts/test-geodesic-prebuilt.sh ~/Dev/cloudposse/infra/infra-live
```

**What it does:**
1. Builds Atmos for Linux (cross-compiles if needed for your architecture)
2. Launches a Geodesic container with:
    - The pre-built Atmos binary mounted to `/usr/local/bin/atmos`
    - Your infrastructure directory mounted to `/workspace`
    - Atmos-managed AWS credentials from `$XDG_CONFIG_HOME/atmos` (defaults to `~/.config/atmos`)
    - Standard XDG environment variables configured

This workflow is much faster than rebuilding Geodesic images during development and allows you to iterate quickly on Atmos changes while testing in a realistic containerized environment.

## Configuration Files

- `atmos.yaml` (`git.hooks`) and `scripts/checks/` - Git hook entry and shared check suite
- `.golangci.yml` - golangci-lint rules
- `.github/workflows/pre-commit.yml` - CI workflow for Atmos repository checks
- `.atmos.d/dev.yaml` - Atmos custom development commands
