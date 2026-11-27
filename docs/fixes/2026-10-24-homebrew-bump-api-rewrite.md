# Homebrew bump: drop `brew`, open the homebrew-core PR via the GitHub API

**Date:** 2026-10-24
**Release affected:** every stable release (the automated bump never once succeeded)
**Fix:** `.github/workflows/release-distribution.yml` - replace the `brew bump-formula-pr` step with `node .github/scripts/homebrew-bump.mjs`

## Symptom

The release "Bump Homebrew formula" step failed on every stable release, each
time in a new way, because `brew bump-formula-pr` is a developer command that
assumes a working `brew` + `git` checkout on the runner. Four distinct failure
modes were observed across five CI fixes (one, #3240, was a misdiagnosis that
never changed the outcome):

1. `brew: command not found` (exit 127) - brew not on the non-login shell's PATH.
2. `No available formula with the name "atmos"` - `homebrew/core` not tapped.
3. `brew tap homebrew/core` refused (exit 1) on Homebrew 7.x without `--force`.
4. `Author identity unknown ... fatal: empty ident name ... git commit ... exited with 128` - the runner has no git identity for the commit `bump-formula-pr` makes.

Each fix (PRs #3229, #3240/#3336, #3370) merely uncovered the next precondition.
After #3370 the step finally reached `git commit` on v1.233.0 and died there.

## Root cause

`brew bump-formula-pr` edits the formula `.rb` locally and commits with local
`git`, so it depends on brew being installed, `homebrew/core` being tapped, the
tap command being accepted by the current brew, and a configured git identity -
a chain of runner preconditions that keeps shifting with each brew/runner image
update. The tool is built for an interactive maintainer workstation, not a fresh
CI runner.

## Fix

Stop shelling out to `brew` entirely. `.github/scripts/homebrew-bump.mjs` opens
the homebrew-core PR through the GitHub REST API:

1. Download the release source tarball and compute its `sha256`.
2. Resolve the bot account from the token (`GET /user`).
3. Ensure a fork of `Homebrew/homebrew-core` exists and fast-forward it to upstream (`merge-upstream`).
4. Create/reset a `bump-atmos-<version>` branch on the fork.
5. Read `Formula/a/atmos.rb`, replace **only** the source `url` and the bare source `sha256` (bottle checksums and everything else untouched), asserting each line matches exactly once.
6. Commit via the contents API - **GitHub stamps the commit with the token's identity server-side, so no runner git identity is needed** - and open the PR.

The PR body embeds the live upstream `PULL_REQUEST_TEMPLATE.md` verbatim (so it
always keeps >=75% of the checklist/heading lines `check_template.rb` requires,
even if Homebrew edits the template) plus the required AI/LLM disclosure.

This is a `no-release` CI-only change.

## Why the API, not another brew patch

The contents API has none of brew's preconditions: no brew install, no tap, no
tap-command gate, no local git identity. It is immune to all four observed
failure modes at once, and it is exactly the manual fallback already run by hand
each release - now automated. Prior fix-logs (`2026-10-13-homebrew-bump-tap-core-force.md`)
already named this rewrite as the durable alternative.

## Testing

`.github/scripts/homebrew-bump.test.mjs` runs offline (injected `fetchImpl`) under
`node --test` and is picked up by `pre-commit.yml`'s CI-helper test step. It
covers version normalization, the exactly-once formula edit (and its loud failure
on a format change), the template+disclosure PR body, the happy-path PR open with
the computed url/sha, branch force-update on re-run, idempotent return of an
already-open PR, and the fork-readiness poll.

## Verification note

The `homebrew:` job only runs on `release: published`, so PR CI cannot exercise
the live GitHub-API path end to end - the next stable release is the first real
confirmation, as with every prior fix in this saga. The unit tests exercise the
full control flow against a GitHub REST double.

## Lineage

Fifth and (intended) final fix in this saga - see
`docs/fixes/2026-09-28-homebrew-bump-brew-not-on-path.md` (PATH, #3229),
`docs/fixes/2026-10-01-homebrew-bump-no-install-from-api.md` (#3240, later shown
wrong), `docs/fixes/2026-10-09-homebrew-bump-needs-core-tap.md` (tap, #3336),
`docs/fixes/2026-10-13-homebrew-bump-tap-core-force.md` (`--force`, #3370), and
now this API rewrite that removes `brew` from the loop.
