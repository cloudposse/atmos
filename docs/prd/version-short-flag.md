# PRD: `atmos version --short` Flag

## Overview

The `atmos version --short` flag (and its alias `-s`) prints only the bare version number to stdout, with no banner, no alien icon, no update check, and no OS/arch suffix. This makes it trivial to capture the Atmos version in shell scripts and CI pipelines.

A new `--format plain` value is also added as the explicit underlying format, and `ATMOS_VERSION_SHORT` is exposed as the corresponding environment variable.

## Problem Statement

Before this change, extracting the Atmos version string in an automated context required fragile text parsing:

```bash
# Fragile: relies on output format never changing
ATMOS_VER=$(atmos version | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
```

Even in headless/no-TTY mode (where the ASCII art banner is suppressed), `atmos version` still emits the full decorated line:

```
👽 Atmos 1.96.0 on linux/amd64
```

This is not machine-parseable without a `grep` workaround because it includes an emoji and OS/arch information.

Users need a clean, stable, single-token output path:

```bash
ATMOS_VER=$(atmos version --short)
# Result: 1.96.0
```

## Goals

1. **Clean scripting output** — `atmos version --short` prints exactly the version number and a newline. Nothing else.
2. **Skip unnecessary network calls** — The update check is skipped for `--short` / `--format plain` to avoid latency and GitHub API usage in CI.
3. **Consistent flag design** — Follows the pattern of other Atmos flags: long form, short form, and environment variable parity.
4. **Backwards compatibility** — No existing behavior changes. New flag is purely additive.

## Non-Goals

- Changes to the default `atmos version` output.
- Changes to `--format json` / `--format yaml` behavior.
- Auto-installing or upgrading Atmos.

## Implementation

### New Flag: `--short` / `-s`

Added to `cmd/version/version.go`:

```go
flags.WithBoolFlag("short", "s", false, "Print just the version number"),
flags.WithEnvVars("short", "ATMOS_VERSION_SHORT"),
```

When `--short` is set and no explicit `--format` is given, `parseVersionOptions` promotes it to `--format plain`:

```go
if opts.Short && opts.Format == "" {
    opts.Format = "plain"
}
```

This means `--short` is sugar for `--format plain`. If the user explicitly passes `--short --format json`, the `json` format wins.

### New Format Value: `plain`

Added to `internal/exec/version.go` in the `displayVersionInFormat` switch:

```go
case "plain":
    // Plain format outputs just the version number without update checking.
    if err := data.Writeln(version.Version); err != nil {
        return errUtils.Build(errUtils.ErrVersionDisplayFailed).
            WithHint("Check if stdout is writable").
            WithContext("format", "plain").
            Err()
    }
    return nil
```

Key properties of the `plain` format:
- Writes **only** `version.Version` (e.g., `1.96.0`) to stdout via the data channel.
- Does **not** call `GetLatestVersion()`, so no GitHub API call is made.
- Does **not** construct the `Version` struct with OS/arch fields.

The error message for unsupported formats is updated to hint at the new valid values:

```go
WithHint("Use --format plain for plain-text version output").
WithHint("Use --format json for JSON output").
WithHint("Use --format yaml for YAML output").
```

### Output Channel

`plain` output uses `data.Writeln()` (stdout), consistent with `json` and `yaml` formats. This enables clean piping:

```bash
ATMOS_VER=$(atmos version --short)
```

### Affected Files

| File | Change |
|------|--------|
| `cmd/version/version.go` | Add `Short bool` field, `--short`/`-s` flag, `ATMOS_VERSION_SHORT` env var, `--format` help text update |
| `cmd/version/version_test.go` | Tests for `--short` flag registration and `parseVersionOptions` short-to-plain promotion |
| `internal/exec/version.go` | Add `plain` case to `displayVersionInFormat`, update error hints |
| `internal/exec/version_test.go` | Add `plain` format test case |
| `internal/exec/examples/version_format.md` | Add `--short` example |
| `website/docs/cli/commands/version/usage.mdx` | Document `--short` and `--format plain` |
| `website/blog/2026-03-12-version-short-flag.mdx` | Blog post announcing the feature |
| `website/blog/authors.yml` | Add `nitrocode` author entry |
| `website/src/data/roadmap.js` | DX initiative milestone entry |

## Usage

All three invocations are equivalent:

```bash
atmos version --short
atmos version -s
atmos version --format plain
```

Environment variable:

```bash
ATMOS_VERSION_SHORT=true atmos version
```

Example output:

```
1.96.0
```

## Scripting Examples

```bash
# Store version in a variable
ATMOS_VER=$(atmos version --short)
echo "Running Atmos $ATMOS_VER"

# Enforce minimum version in CI
MIN_VERSION="1.90.0"
ATMOS_VER=$(atmos version --short)
if [[ "$(printf '%s\n' "$MIN_VERSION" "$ATMOS_VER" | sort -V | head -n1)" != "$MIN_VERSION" ]]; then
  echo "Error: Atmos $MIN_VERSION or higher is required (found $ATMOS_VER)"
  exit 1
fi

# Use in version badge generation
echo "atmos-v$(atmos version --short)"
```

## All Version Output Formats

| Command | Output |
|---------|--------|
| `atmos version` | Styled banner with alien icon and optional update check |
| `atmos version --short` | `1.96.0` (version number only, stdout) |
| `atmos version --format json` | JSON object with `version`, `os`, `arch`, and optional `update_version` |
| `atmos version --format yaml` | YAML with same fields |

## Testing Strategy

### Unit Tests (`cmd/version/version_test.go`)

- `TestVersionCommand_Flags` — Verifies `--short` flag is registered with shorthand `-s` and default value `false`.
- `TestParseVersionOptions_ShortFlag` — Table-driven tests covering:
  - `--short` alone → format promoted to `plain`
  - `--short --format json` → format stays `json`
  - No `--short`, no format → format stays empty
  - No `--short` with explicit format → format unchanged

### Unit Tests (`internal/exec/version_test.go`)

- `TestDisplayVersionInFormat` — Adds `plain` as a valid format test case alongside existing `json` and `yaml`.

## Design Decisions

### Why `--format plain` Instead of a Standalone Flag

The `--short` flag is user-friendly shorthand. The `--format plain` value is the canonical underlying representation. Both are exposed so:
- Scripts can use the more explicit `--format plain` for readability.
- Interactive users can use the convenient `-s` shorthand.
- The implementation shares a single code path (`displayVersionInFormat("plain", ...)`).

### Why Skip the Update Check for `plain` Format

The `plain` format is designed for scripting and CI contexts where latency matters and network calls to GitHub should be avoided. The update check is already skipped for `json`/`yaml` unless `--check` is passed explicitly; `plain` follows the same pattern.

### Why `data.Writeln()` (stdout) Instead of `ui.Writeln()` (stderr)

Plain format is machine-readable output, not a UI message. Sending it to stdout allows the result to be captured by `$(...)` command substitution or piped to downstream commands. This is consistent with `json` and `yaml` output channels.

## References

- **PR:** [#2174 feat(version): add `--short` flag to print version number without banner](https://github.com/cloudposse/atmos/pull/2174)
- **Existing version command PRD:** `docs/prd/atmos-version-command.md`
- **Version command:** `cmd/version/version.go`
- **Version execution:** `internal/exec/version.go`
- **Documentation:** `website/docs/cli/commands/version/usage.mdx`
- **Blog post:** `website/blog/2026-03-12-version-short-flag.mdx`
