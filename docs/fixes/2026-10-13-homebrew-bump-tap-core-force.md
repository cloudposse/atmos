# Homebrew bump: `brew tap homebrew/core` now requires `--force`

**Date:** 2026-10-13
**Release affected:** v1.232.0 (run 38333322212, job "Bump Homebrew formula")
**Fix:** `.github/workflows/build.yml` - `brew tap homebrew/core` -> `brew tap homebrew/core --force`

## Symptom

The release `build.yml` "Bump Homebrew formula" step failed with exit code 1:

```
brew tap homebrew/core
##[error]Tapping homebrew/core is no longer typically necessary.
Add --force if you are sure you need it for contributing to Homebrew.
##[error]Process completed with exit code 1.
```

The step died on the `brew tap homebrew/core` line - the one added in PR #3336
(2026-10-09) - before `brew bump-formula-pr` ever ran, so no Homebrew PR was
opened for v1.232.0.

## Root cause

The Homebrew preinstalled on `ubuntu-latest` was upgraded to the 7.x line, which
now deprecates tapping `homebrew/core` directly (everything is served from the
formulae API) and **exits non-zero** on a plain `brew tap homebrew/core`,
instructing the caller to pass `--force` when the tap is genuinely needed for
contributing to Homebrew.

`brew bump-formula-pr` is a developer command that edits the formula `.rb`, so it
still needs `homebrew/core` checked out as a local tap (established across
v1.228.0-v1.231.0: without the tap the bump fails "No available formula with the
name atmos"). We therefore genuinely need the tap - `--force` is the sanctioned
path Homebrew's own error message points to.

## Fix

```diff
-          brew tap homebrew/core
+          brew tap homebrew/core --force
```

Plus an explanatory comment above the command.

## Why not drop the tap entirely

Tried-and-rejected across prior releases: a fresh runner uses the formulae API
with no tap, and `bump-formula-pr` cannot edit a formula it only knows through the
API -> "No available formula with the name atmos". The tap must be present; the
only change needed was the `--force` flag the newer brew now requires.

## Verification note

This step only runs on `release: published`, so PR CI cannot exercise it - the
next release is the first real confirmation. If brew churns again here, the
durable alternative is to stop shelling out to `brew` entirely and open the
homebrew-core PR via the GitHub API (fork + edit url/sha256 + create PR), which is
exactly the manual fallback already in use each release and is immune to brew CLI
changes.

## Lineage

Fourth fix in this saga - see `docs/fixes/2026-09-28-homebrew-bump-brew-not-on-path.md`
(PATH, #3229), `docs/fixes/2026-10-01-homebrew-bump-no-install-from-api.md`
(#3240, later shown wrong), `docs/fixes/2026-10-09-homebrew-bump-needs-core-tap.md`
(tap, #3336), and now this.
