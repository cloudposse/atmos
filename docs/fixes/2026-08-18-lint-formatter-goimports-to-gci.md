# Fix: Switch the lint import formatter from `goimports` to `gci`

**Date:** 2026-08-18

## Summary

`golangci-lint`'s `goimports` formatter was a measurable bottleneck in full-repo lint/format
passes (~95-108s real time). Switched `.golangci.yml`'s `formatters.enable` list to `gci` with an
explicit import-section configuration, cutting the same full-repo pass to ~5-7s — a ~15-20x
speedup — with equivalent (arguably more explicit) import grouping.

## Context

PR #2926 on a different, unrelated branch (`feat(pro): automatic command-execution metadata
upload to Atmos Pro`) happened to include this same formatter swap buried in its `.golangci.yml`
diff, prompted by the user asking to bring over "huge speed improvements in linting from ordered
imports" from that PR. That branch was otherwise a large, unrelated, still-WIP feature with
several unrelated and unintentional-looking hunks (`run.tests: false`, `run.allow-serial-runners:
false` — flipped directly under a comment in the same file explaining why it must stay `true`,
and a `scripts/run-custom-golangci-lint.sh` edit that replaced the real `./custom-gcl` invocation
with `echo` — clearly abandoned debug scaffolding). Only the `gci` formatter change was
cherry-picked; the rest was deliberately left out as out of scope / likely unintentional.

## Changes

- `.golangci.yml`: `formatters.enable` swaps `goimports` for `gci`, and adds
  `settings.gci.sections: [standard, default, prefix(github.com/cloudposse/atmos)]` with
  `custom-order: true`. `gci` is a stock golangci-lint v2 formatter (confirmed via
  `./custom-gcl formatters`), so no `.custom-gcl.yml` module-plugin change was needed.
- `CLAUDE.md` (`### Go Formatting (MANDATORY)`): updated the sentence naming the enabled
  formatters from `gofumpt`/`goimports` to `gofumpt`/`gci`.
- `.claude/agents/lint-fix.md` and `.claude/agents/test-coverage-fix.md`: updated the
  `gofumpt`/`goimports` formatting-convention references to `gofumpt`/`gci`.
- Landed on branch `osterman/go-tool-lintroller-migration`, folded into PR #2955 (title and
  description updated to cover both the pre-existing Mage-tooling migration and this formatter
  swap).

## Validation

- `./custom-gcl formatters` confirms `gci` is enabled and `goimports` disabled after the config
  change.
- Benchmarked directly against this repo with `./custom-gcl fmt -d` (diff mode, no writes),
  full-repo, two runs per formatter, with a warm filesystem cache in both directions (to rule out
  a cold-cache artifact rather than a genuine formatter speed difference):

  | Formatter          | Run 1       | Run 2      |
  | ------------------- | ----------- | ---------- |
  | `goimports` (old)   | 107.6s real | 94.9s real |
  | `gci` (new)         | 5.3s real   | 6.9s real  |

- Pre-commit hooks (`go-fumpt`, `golangci-lint config verify`, `Validate EditorConfig`, etc.) ran
  clean on the commit introducing this change.
- No `go build`/`go test` impact expected or checked further — this is a lint-config-only and
  documentation-only change with no `.go` files touched.

## Follow-ups

None.
