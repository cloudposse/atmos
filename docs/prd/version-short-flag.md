# PRD: `atmos version --short` Flag

## Overview

`atmos version --short` (alias `-s`) prints only the bare version number to stdout: no banner, no alien icon, no update check, no OS/arch suffix. `--format plain` is the underlying canonical format; `ATMOS_VERSION_SHORT` is the corresponding environment variable.

## Problem Statement

Even in no-TTY/headless mode (where the ASCII art banner is suppressed), `atmos version` emits:

```
👽 Atmos 1.96.0 on linux/amd64
```

The emoji and OS/arch suffix make this output fragile to parse in scripts. Users resort to:

```bash
ATMOS_VER=$(atmos version | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)
```

The `--short` flag replaces that with a stable, single-token output:

```bash
ATMOS_VER=$(atmos version --short)
# Output: 1.96.0
```

## Goals

1. **Clean scripting output:** `atmos version --short` prints exactly the version number and a trailing newline.
2. **No unnecessary network calls:** The update check is skipped for `--format plain` to avoid latency and GitHub API usage in CI.
3. **Consistent flag design:** Follows the Atmos convention of long form, short form, and environment variable parity.
4. **Backwards compatible:** The new flag is purely additive.

## Non-Goals

- Changes to the default `atmos version` output.
- Changes to `--format json` / `--format yaml` behavior.

## Implementation

### New Flag: `--short` / `-s`

Added to `cmd/version/version.go`:

```go
flags.WithBoolFlag("short", "s", false, "Print just the version number"),
flags.WithEnvVars("short", "ATMOS_VERSION_SHORT"),
```

`--short` is sugar for `--format plain`. When `--short` is set and `--format` is not explicitly provided, `parseVersionOptions` promotes it:

```go
if opts.Short && opts.Format == "" {
    opts.Format = "plain"
}
```

If the user passes `--short --format json`, the explicit `--format` wins.

Combining `--short` with `--check` has no additional effect: the `plain` format path always skips the update check regardless of `--check`.

### New Format Value: `plain`

Added to `internal/exec/version.go`:

```go
case "plain":
    if err := data.Writeln(version.Version); err != nil {
        return errUtils.Build(errUtils.ErrVersionDisplayFailed).
            WithHint("Check if stdout is writable").
            WithContext("format", "plain").
            Err()
    }
    return nil
```

The `plain` case:
- Writes only `version.Version` (e.g., `1.96.0`) to stdout via `data.Writeln()`.
- Does not call `GetLatestVersion()` (no GitHub API call).
- Does not construct the `Version` struct with OS/arch fields.

Output goes to stdout (data channel), consistent with `json` and `yaml`, so it can be captured by `$(...)` or piped.

The unsupported-format error hints are updated to list all valid values:

```go
WithHint("Use --format plain for plain-text version output").
WithHint("Use --format json for JSON output").
WithHint("Use --format yaml for YAML output").
```

### Affected Files

| File | Change |
|------|--------|
| `cmd/version/version.go` | Add `Short bool` field, `--short`/`-s` flag, `ATMOS_VERSION_SHORT` env var, update `--format` help text |
| `cmd/version/version_test.go` | Tests for flag registration and `parseVersionOptions` short-to-plain promotion |
| `internal/exec/version.go` | Add `plain` case to `displayVersionInFormat`, update error hints |
| `internal/exec/version_test.go` | Add `plain` format test case |
| `internal/exec/examples/version_format.md` | Add `--short` usage example |
| `website/docs/cli/commands/version/usage.mdx` | Document `--short` and `--format plain` |
| `website/blog/2026-03-12-version-short-flag.mdx` | Blog post (author: `nitrocode`) |
| `website/blog/authors.yml` | Add `nitrocode` author entry |
| `website/src/data/roadmap.js` | DX initiative milestone entry |

## Usage

All three invocations are equivalent:

```bash
atmos version --short
atmos version -s
atmos version --format plain
```

Via environment variable:

```bash
ATMOS_VERSION_SHORT=true atmos version
```

Output:

```
1.96.0
```

## Version Output Formats

| Command | Output |
|---------|--------|
| `atmos version` | Styled banner with alien icon and optional update check |
| `atmos version --short` | `1.96.0` (version number only) |
| `atmos version --format json` | JSON object with `version`, `os`, `arch`, and optional `update_version` |
| `atmos version --format yaml` | YAML with same fields |

## Testing Strategy

### `cmd/version/version_test.go`

- `TestVersionCommand_Flags`: verifies `--short` is registered with shorthand `-s` and default `false`.
- `TestParseVersionOptions_ShortFlag`: table-driven tests covering:
  - `--short` with no format: promoted to `plain`
  - `--short --format json`: explicit format wins, stays `json`
  - No `--short`, no format: format stays empty
  - No `--short`, explicit format: format unchanged

### `internal/exec/version_test.go`

- `TestDisplayVersionInFormat`: adds `plain` as a valid format case alongside `json` and `yaml`.

## Design Decisions

### `--short` as sugar for `--format plain`

Both forms are exposed for different audiences: `--short`/`-s` for quick interactive use; `--format plain` for explicit, readable scripting. They share one code path.

### No update check for `plain`

The `plain` format targets CI and scripting where latency matters and GitHub API calls are unwanted. The `json`/`yaml` formats follow the same rule (update check requires explicit `--check`); `plain` is consistent with that.

### stdout for `plain` output

`plain` is machine-readable output, not a UI message. It goes to stdout via `data.Writeln()` so it can be captured by `$(...)` or piped, consistent with `json` and `yaml`.

## References

- **PR:** [#2174 feat(version): add `--short` flag](https://github.com/cloudposse/atmos/pull/2174)
- **Version command PRD:** `docs/prd/atmos-version-command.md`
- **Version command:** `cmd/version/version.go`
- **Version execution:** `internal/exec/version.go`
- **Documentation:** `website/docs/cli/commands/version/usage.mdx`
- **Blog post:** `website/blog/2026-03-12-version-short-flag.mdx`
