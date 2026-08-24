# Planfile CLI: Component/Stack Addressing — SHIPPED

> Related: [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Implementation Status](../framework/implementation-status.md) | [Unify Artifact Stores](unify-artifact-stores.md)

## Status: SHIPPED

Implemented on branch `goruha/native-ci-terraform-plan-step-2-5`.

## Problem Statement

The planfile CLI commands (`atmos terraform planfile {upload,download,list,show,delete}`) used raw storage keys and `--component`/`--stack` flags, inconsistent with the main terraform commands which use `atmos terraform plan <component> -s <stack>`. This creates a confusing developer experience where users must manually construct storage keys.

### Issues

1. **Inconsistent addressing**: Main terraform commands use `<component> -s <stack>` positional+flag pattern. Planfile commands used `<key>` or `--component`/`--stack` flags.
2. **Exposed storage internals**: Users had to know the storage key format (`stack/component/sha.tfplan.tar`) to download, show, or delete planfiles.
3. **No automatic SHA resolution**: Users had to manually specify SHA or know the key. No fallback to current git HEAD or CI env vars.
4. **No batch operations**: Delete required exact key. No way to delete all planfiles for a component/stack or across all SHAs.

## Solution

### Target CLI Interface

```bash
# List — component and stack are optional filters.
atmos terraform planfile list [component] [-s stack]         # current SHA.
atmos terraform planfile list [component] [-s stack] --all   # all SHAs.

# Upload — component is required positional arg, stack is required flag.
atmos terraform planfile upload <component> -s <stack> [--planfile path] [--sha sha]

# Download — component is required positional arg, stack is required flag.
atmos terraform planfile download <component> -s <stack> [--output path]

# Show — component is required positional arg, stack is required flag.
atmos terraform planfile show <component> -s <stack>

# Delete — component and stack are optional filters.
atmos terraform planfile delete [component] [-s stack]          # current SHA, confirmation.
atmos terraform planfile delete [component] [-s stack] --all    # all SHAs, confirmation.
atmos terraform planfile delete [component] [-s stack] --force  # skip confirmation.
```

### Key Design Decisions

1. **Component = positional arg**, stack = `-s`/`--stack` flag — matches `atmos terraform plan <component> -s <stack>`.
2. **No raw key access** — keys always derived from component + stack + SHA via `KeyPattern.GenerateKey()`.
3. **SHA defaults to current context** on all commands — resolved from env vars (`ATMOS_CI_SHA`, `GIT_COMMIT`, `CI_COMMIT_SHA`, `COMMIT_SHA`) or git HEAD.
4. **`--all` flag only on `list` and `delete`** — bypasses SHA filtering. Not available on `upload`, `download`, or `show` (which operate on a single planfile).
5. **`list` and `delete`**: component and stack are both optional (filter dimensions).
6. **`upload`, `download`, `show`**: component is required positional arg, stack is required `-s` flag.
7. **Delete confirmation**: shows list of affected planfiles, requires `--force` to proceed without interactive prompt.

### SHA Resolution

All commands resolve SHA using the same priority chain:

1. `ATMOS_CI_SHA` env var
2. `GIT_COMMIT` env var
3. `CI_COMMIT_SHA` env var
4. `COMMIT_SHA` env var
5. `git rev-parse HEAD` (via `pkg/git.NewDefaultGitRepo().GetCurrentCommitSHA()`)

This matches the generic provider's `getFirstEnvOrGit()` logic.

### Flag Architecture

- **`--stack`/`-s`** — persistent flag on `PlanfileCmd`, inherited by all subcommands.
- **`--all`** — command-specific flag on `list` and `delete` only.
- **`--force`/`-f`** — command-specific flag on `delete` only.
- **`--output`/`-o`** — command-specific flag on `download` (replaces positional output-path).
- **`--store`** — command-specific flag on each subcommand.
- **`--format`** — command-specific flag on `list` and `show`.

## Files Changed

| File | Action | Description |
|------|--------|-------------|
| `cmd/terraform/planfile/resolve.go` | **New** | SHA resolution (`resolveContext`), key generation (`resolveKey`), query building (`buildQuery`) helpers |
| `cmd/terraform/planfile/resolve_test.go` | **New** | Tests for resolve helpers |
| `cmd/terraform/planfile/planfile.go` | **Modified** | Added persistent `--stack`/`-s` flag, updated `BaseOptions` with `Stack` field |
| `cmd/terraform/planfile/list.go` | **Modified** | Component positional arg, `--all` flag, SHA-filtered query via `resolveContext`/`buildQuery` |
| `cmd/terraform/planfile/upload.go` | **Modified** | Component positional arg, removed `--component`/`--stack`/`--key` flags, SHA from `resolveContext` |
| `cmd/terraform/planfile/download.go` | **Modified** | Component positional arg, `--output`/`-o` flag, key from `resolveKey` |
| `cmd/terraform/planfile/show.go` | **Modified** | Component positional arg, key from `resolveKey` |
| `cmd/terraform/planfile/delete.go` | **Modified** | Optional component, `--all` flag, list-then-delete with confirmation prompt |

## Verification

1. `go build ./...` — passes.
2. `go test ./cmd/terraform/planfile/...` — passes.
3. `go vet ./cmd/terraform/planfile/...` — clean.
