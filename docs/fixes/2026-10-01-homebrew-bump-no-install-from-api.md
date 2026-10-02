# Fix: Homebrew formula bump fails to resolve the atmos formula

**Date:** 2026-10-01

## Summary

The release workflow's "Bump Homebrew formula" step failed with
`No available formula with the name "atmos". Did you mean aptos?` even though
`atmos` exists in `Homebrew/homebrew-core`. Removing `HOMEBREW_NO_INSTALL_FROM_API: "1"`
from the step lets `brew bump-formula-pr` run in its default API mode, which resolves
the formula correctly.

## Context

PR #3229 fixed an earlier failure in the same step (`brew: command not found`, a PATH
problem) by sourcing `brew shellenv`. That fix let the step actually run `brew
bump-formula-pr` for the first time, which surfaced a latent second bug: the step set
`HOMEBREW_NO_INSTALL_FROM_API: "1"`, forcing a full `homebrew-core` tap clone and making
`bump-formula-pr` resolve the formula from that freshly cloned tree. That resolution
fails ("No available formula with the name atmos") on every release run sampled
(v1.228.0, v1.229.0, v1.230.0, v1.230.1), so every Homebrew bump has been done manually.

The formula genuinely exists (`Formula/a/atmos.rb` on homebrew-core's `main` branch,
served live by `formulae.brew.sh`). The env var was added on the mistaken premise that
it was needed "to make the formula file editable"; `bump-formula-pr` edits, forks, and
opens the PR itself without it.

## Changes

- `.github/workflows/build.yml`: removed `HOMEBREW_NO_INSTALL_FROM_API: "1"` from the
  "Bump Homebrew formula" step so `brew bump-formula-pr` uses the default formulae-API
  mode. `HOMEBREW_NO_AUTO_UPDATE: "1"` is kept. Updated the step comment to explain why
  the var must not be set.

## Validation

- `brew bump-formula-pr --dry-run --version=1.230.1 atmos` in API mode resolved `atmos`
  cleanly and proceeded to the duplicate-PR check with no "No available formula" error -
  the exact failure seen in CI with `HOMEBREW_NO_INSTALL_FROM_API=1`.
- `actionlint .github/workflows/build.yml`: no findings in the edited step (the only
  output is a pre-existing shellcheck note in an unrelated step).
- Python `yaml.safe_load` parses the workflow.

Full end-to-end verification only happens on a real `release: published` run; a
maintainer re-run or the next release will confirm the automated bump opens its PR.

## Follow-ups

None blocking. The v1.230.1 Homebrew formula was bumped manually in the interim
(homebrew-core PR #314644) since the automated step has never succeeded.
