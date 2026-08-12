# Fix: gh CLI token fallback, silent import failures, and import caching

**Date:** 2026-08-11

## Summary

A field test of `website/docs/tutorials/centralized-auth-config.mdx` (a new tutorial showing how to
centralize an org's Atmos `auth:` config in a private repo and `import:` it into every project) found
three real gaps in Atmos itself — not just doc inaccuracies — plus one genuinely doc-only inaccuracy.
This record covers both stages: the initial pass corrected the tutorial to accurately describe Atmos's
*then-current* (broken) behavior; a follow-up pass in the same PR fixed the underlying behavior in code,
which meant the tutorial needed a second update to describe the *new, fixed* behavior. **The findings
below (1, 2, 4) describe pre-fix behavior and are historical** — see "Final Behavior" under each for what
Atmos does now. Finding 3 (the identity-selector claim) was doc-only from the start and required no code
change.

## Context

The tutorial was field-tested per the `field-test` skill: built a fresh `atmos` binary, created a real
local git-repo fixture matching the guide's exact `aws/developers/auth.yaml` layout and `git::` import
syntax, and ran live commands against it, cross-checked with two parallel code-tracing research passes.

Findings, most severe first:

1. **[Pre-fix] `gh auth token` fallback claim was wrong for this import path.** The guide's "Private repo
    access — no token setup needed" section claimed Atmos resolves a GitHub token in order
    `--github-token` flag → `ATMOS_GITHUB_TOKEN` → `GITHUB_TOKEN` → `gh auth token` CLI fallback, and
    that `gh auth login` alone was therefore sufficient. Reading `pkg/downloader/custom_git_detector.go`
    (`CustomGitDetector.resolveToken`, the code that actually authenticates the `git clone` behind a
    `git::` import) showed it only checked `ATMOS_PRO_GITHUB_TOKEN` → `ATMOS_GITHUB_TOKEN` →
    `GITHUB_TOKEN` — no `--github-token` flag, no `gh auth token` fallback at all. That fallback chain was
    real, but belonged to a different code path (`github.GetGitHubToken()`, used for plain HTTPS/API
    fetches and toolchain installs), not the `git::` clone this tutorial's import syntax triggers.
    **Final behavior:** `CustomGitDetector.resolveToken` now has a fourth tier that falls back to
    `gh auth token` (via a newly exported `github.GetGitHubTokenFromCLI()`), matching the sibling code
    path. `gh auth login` alone is sufficient again — the tutorial's original claim is now true.

2. **[Pre-fix] A broken import failed completely silently.** Live-verified: pointing the import at a
    nonexistent `ref` produced `atmos auth list` → `"No providers or identities configured."` with
    **exit code 0**, no error. The real error (`fatal: couldn't find remote ref ...`) was generated
    correctly several layers down but discarded at `pkg/config/imports.go` (`log.Debug(...); continue`),
    one layer above where it's returned — and the default log level is `Warning`, so it was invisible
    without `ATMOS_LOGS_LEVEL=Debug`. This compounded directly with finding 1: a developer with only
    `gh auth login` done (no token env var) would have their import silently no-op, with zero indication
    why. **Final behavior:** the same swallow points in `processImports` and `mergeResolvedImports` now
    log at `Warn`, which is visible at the default log level — no `ATMOS_LOGS_LEVEL=Debug` needed to see
    that (and why) an import failed. Still non-fatal by design (one bad import doesn't abort the whole
    config load), just no longer silent.

3. **[Doc-only, no code change] The "interactive selector" claim was misleading given the tutorial's own
    example.** The Developer Workflow section said omitting `--identity` shows a selector. Every example
    identity in the guide's AWS/Azure/GCP tabs sets `default: true`, and `pkg/auth/manager.go`
    (`GetDefaultIdentity`) auto-selects silently in that case — no selector appears. A selector only
    appears with a bare `--identity` (no value) or when zero/multiple identities are marked default. This
    was purely a wrong doc claim about existing, correct behavior; nothing to fix in code.

4. **[Pre-fix] No mention that this import form has no cache.** `git::...//subpath?ref=...` imports at the
    root `atmos.yaml` level had no cross-run TTL/cache (unlike stack-level imports), so every `atmos`
    command in a project using this pattern re-cloned the central repo — an undocumented network
    dependency for every command, not just auth ones. **Final behavior:** the existing top-level
    `imports.ttl` atmos.yaml setting (already used by stack-manifest imports) is now also read by
    `RemoteImporter.Resolve`, the root-config path — one shared setting now caches both. This applies
    specifically to `git::` imports that use a subdirectory; plain remote URLs and git imports without a
    subdirectory use a separate, non-expiring cache and are unaffected either way.

Two other claims from the same tutorial were checked and found accurate, so left unchanged: the
"quote `account.id` as a string" advice (an unquoted YAML integer really does fail a Go type assertion
in `pkg/auth/identities/aws/permission_set.go`), and `atmos auth shell` not revoking credentials on
exit (already corrected in an earlier pass this session, per a CodeRabbit review comment on this PR).

## Changes

**Stage 1 (docs-only, describing the then-current broken behavior):**

- Rewrote "§3. Private repo access" (renamed from "no token setup needed" to "one token, set once") to
  state the real (pre-fix) token chain and explain that `gh auth login` alone didn't cover it.
- Rewrote the Troubleshooting section to lead with the silent-failure mode and the `ATMOS_LOGS_LEVEL=Debug`
  diagnostic.
- Fixed the Developer Workflow comment on `atmos auth login` to describe default-identity auto-selection
  correctly (this one was never superseded — see finding 3).
- Added a paragraph in §2 noting the import form re-clones on every command with no local cache.

**Stage 2 (code fixes, this same PR):**

- `pkg/downloader/custom_git_detector.go` + `pkg/github/token.go`: added the `gh auth token` CLI fallback
  tier to `CustomGitDetector.resolveToken`.
- `pkg/config/imports.go`: elevated the two silent-failure log points from `Debug` to `Warn`.
- `pkg/stack/imports/remote.go`: `RemoteImporter.Resolve` now reads `atmosConfig.Imports.TTL` instead of
  hardcoding an empty ttl.
- `website/docs/tutorials/centralized-auth-config.mdx`: reverted §3's heading and content back to "no
  token setup needed" (now true again), removed the "`gh auth login` alone isn't enough" caveat from the
  "What's Actually Zero-Setup" section and Troubleshooting, and updated §2's caching note to point at the
  new `imports.ttl` option instead of stating there's no cache.
- `website/docs/cli/configuration/imports.mdx`, `website/blog/2026-08-11-remote-import-github-auth-and-caching.mdx`,
  `website/src/data/roadmap.js`: scoped all TTL-caching claims to `git::` imports with a subdirectory
  specifically (not all remote imports), matching `RemoteImporter.Resolve`'s actual behavior.

## Validation

- `cd website && npm run build` — clean build, no new broken-anchor warnings, at every stage including
  after the anchor reverted back to `#3-private-repo-access--no-token-setup-needed`.
- Code changes verified with `go build ./...`, `go test` on every touched package
  (`pkg/config`, `pkg/downloader`, `pkg/github`, `pkg/stack/imports`), `atmos lint --changed`, and the
  authoritative `./custom-gcl run --new-from-rev=origin/main` CI-equivalent gate (only pre-existing,
  unrelated findings remained, confirmed via zero-diff check against those files).
- Both the original pre-fix claims and the final fixed behavior were verified live: a real local
  `git::file://` fixture repo, `atmos auth list`/`atmos auth env` runs before and after each fix, and
  direct reads of `custom_git_detector.go`/`pkg/config/imports.go`/`pkg/stack/imports/remote.go` — see the
  field-test and fix-implementation conversation for full repro commands.

## Follow-ups

None. All findings from the field-test report were fixed in code (1, 2, 4) or confirmed as doc-only with
no code-side gap (3). Doc content was updated a second time after the code fixes landed, so it matches
final behavior, not the intermediate pre-fix state.
