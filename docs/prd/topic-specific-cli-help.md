# PRD: Topic-Specific CLI Help

## Status: Implemented

**Last Updated**: 2026-07-06

## Overview

Atmos help output has grown large enough that default command help can obscure the command-specific information users need most. Add topic-specific help with `--help=<topic>` so default help stays useful while full reference output remains available.

The v1 topic set is intentionally small:

- Default `--help` shows the command description, usage, examples, subcommands, command-specific flags, and a hint for expanded help.
- `--help=usage` shows only usage and embedded usage examples.
- `--help=flags` shows command-specific flags, excluding inherited/global flags.
- `--help=all` shows the full help page, including inherited/global flags.

## Motivation

Large CLIs commonly split help by intent. GCC supports `--help=<class>` for large option surfaces such as warnings and target options. npm supports help topics. kubectl separates quick-reference command usage from exhaustive generated command reference pages.

Atmos already has the pieces needed for this:

- Help output is centralized in `cmd/root.go` through Cobra help handling.
- Help sections are already modeled in `internal/tui/templates/base_template.go`.
- Markdown usage snippets are already embedded through `cmd/markdown_help.go`.

## Goals

1. Keep default `--help` focused on the selected command.
2. Hide inherited/global flags by default.
3. Preserve a complete reference view through `--help=all`.
4. Reuse existing embedded `*_usage.md` snippets for `--help=usage`.
5. Keep pager behavior unchanged: flag-based help only uses the pager when explicitly requested.

## Non-Goals

1. Adding GCC-style category names such as `common`, `warnings`, `optimizers`, or `target`.
2. Reclassifying every Atmos flag into a new taxonomy.
3. Changing command execution behavior outside help rendering.
4. Supporting `help=topic` syntax without the leading `--`.

## User Experience

Default help:

```shell
atmos terraform plan --help
```

Shows description, usage, examples, subcommands if any, local command flags, compatibility flags if applicable, and a hint:

```text
Use `--help=usage` for examples or `--help=all` for all flags and full help.
```

Usage-only help:

```shell
atmos terraform plan --help=usage
```

Shows the usage line and the embedded usage example markdown for the command.

Command flags:

```shell
atmos terraform plan --help=flags
```

Shows command-specific flags and compatibility/pass-through flags. It does not show inherited/global flags.

Full reference:

```shell
atmos terraform plan --help=all
```

Shows the complete help page, including inherited/global flags.

Unknown topic:

```shell
atmos terraform plan --help=advanced
```

Exits non-zero and lists valid topics: `usage`, `flags`, `all`.

## Implementation Notes

- Normalize `--help=<topic>` before Cobra parses flags:
  - Store the requested topic.
  - Rewrite the argument to `--help` so Cobra's boolean help flag continues to work.
  - Preserve boolean help values such as `--help=true` and `--help=false`.
- Route help rendering through a topic-aware dispatcher:
  - default: current full help minus inherited/global flags.
  - `usage`: usage and examples.
  - `flags`: command-specific flags and compatibility flags.
  - `all`: current full help.
- For the root command, filter root persistent flags from default and `--help=flags` output because they are global flags even though Cobra exposes them as local root flags.

## Acceptance Criteria

- `atmos <command> --help` excludes inherited/global flags and includes command-specific flags.
- `atmos <command> --help=usage` excludes flags and includes usage examples when present.
- `atmos <command> --help=flags` excludes inherited/global flags.
- `atmos <command> --help=all` includes inherited/global flags.
- `atmos <command> --help=<unknown>` exits non-zero and lists valid topics.
- Existing `-h`, bare `--help`, and pager behavior continue to work.
