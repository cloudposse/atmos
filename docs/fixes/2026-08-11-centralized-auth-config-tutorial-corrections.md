# Fix: Correct token-resolution and identity-selector claims in the centralized-auth-config tutorial

**Date:** 2026-08-11

## Summary

A field test of `website/docs/tutorials/centralized-auth-config.mdx` (a new tutorial showing how to
centralize an org's Atmos `auth:` config in a private repo and `import:` it into every project) found
that its central claim — private-repo imports "just work" once a developer has run `gh auth login` —
is false for the exact `git::` import syntax the guide recommends. A second claim (that omitting
`--identity` shows an interactive selector) was also verified wrong against the guide's own example
config. Both are now corrected, along with two related gaps found during the same pass: a completely
silent import-failure mode with no diagnostic guidance, and an undocumented per-command re-clone (no
import caching).

## Context

The tutorial was field-tested per the `field-test` skill: built a fresh `atmos` binary, created a real
local git-repo fixture matching the guide's exact `aws/developers/auth.yaml` layout and `git::` import
syntax, and ran live commands against it, cross-checked with two parallel code-tracing research passes.

Findings, most severe first:

1. **`gh auth token` fallback claim was wrong for this import path.** The guide's "Private repo access
   — no token setup needed" section claimed Atmos resolves a GitHub token in order
   `--github-token` flag → `ATMOS_GITHUB_TOKEN` → `GITHUB_TOKEN` → `gh auth token` CLI fallback, and
   that `gh auth login` alone was therefore sufficient. Reading `pkg/downloader/custom_git_detector.go`
   (`CustomGitDetector.resolveToken`, the code that actually authenticates the `git clone` behind a
   `git::` import) shows it only checks `ATMOS_PRO_GITHUB_TOKEN` → `ATMOS_GITHUB_TOKEN` →
   `GITHUB_TOKEN` — no `--github-token` flag, no `gh auth token` fallback at all. That fallback chain is
   real, but it belongs to a different code path (`github.GetGitHubToken()`, used for plain HTTPS/API
   fetches and toolchain installs), not the `git::` clone this tutorial's import syntax triggers.

2. **A broken import fails completely silently.** Live-verified: pointing the import at a nonexistent
   `ref` produces `atmos auth list` → `"No providers or identities configured."` with **exit code 0**,
   no error. The real error (`fatal: couldn't find remote ref ...`) is generated correctly several
   layers down but discarded at `pkg/config/imports.go` (`log.Debug(...); continue`), one layer above
   where it's returned — and the default log level is `Warning`, so it's invisible without
   `ATMOS_LOGS_LEVEL=Debug`. This compounds directly with finding 1: a developer with only `gh auth
   login` done (no token env var) would have their import silently no-op, with zero indication why.

3. **The "interactive selector" claim was misleading given the tutorial's own example.** The Developer
   Workflow section said omitting `--identity` shows a selector. Every example identity in the guide's
   AWS/Azure/GCP tabs sets `default: true`, and `pkg/auth/manager.go` (`GetDefaultIdentity`) auto-selects
   silently in that case — no selector appears. A selector only appears with a bare `--identity` (no
   value) or when zero/multiple identities are marked default.

4. **No mention that this import form has no cache.** `git::...//subpath?ref=...` imports at the root
   `atmos.yaml` level have no cross-run TTL/cache (unlike stack-level imports), so every `atmos` command
   in a project using this pattern re-clones the central repo — an undocumented network dependency for
   every command, not just auth ones.

Two other claims from the same tutorial were checked and found accurate, so left unchanged: the
"quote `account.id` as a string" advice (an unquoted YAML integer really does fail a Go type assertion
in `pkg/auth/identities/aws/permission_set.go`), and `atmos auth shell` not revoking credentials on
exit (already corrected in an earlier pass this session, per a CodeRabbit review comment on this PR).

## Changes

All changes are in `website/docs/tutorials/centralized-auth-config.mdx`:

- Rewrote "§3. Private repo access" (renamed from "no token setup needed" to "one token, set once") to
  state the real token chain (`ATMOS_PRO_GITHUB_TOKEN` → `ATMOS_GITHUB_TOKEN` → `GITHUB_TOKEN`) and
  explain that `gh auth login` alone doesn't cover it — either export a token env var, or run
  `gh auth setup-git` once to wire `gh`'s session into Git's own credential helper.
- Updated the two other places that repeated the old "`gh auth token`"/"`gh auth login`, or a configured
  token" claim (the Tabs intro sentence, and the "What's Actually Zero-Setup" recap), and fixed the
  cross-reference anchor link after the section heading changed.
- Rewrote the Troubleshooting section to lead with the actual failure mode (silent, exit 0) and the
  real diagnostic (`ATMOS_LOGS_LEVEL=Debug`, grep for `failed to resolve import`) before the
  authentication-specific tip.
- Fixed the Developer Workflow comment on `atmos auth login` to describe default-identity auto-selection
  correctly instead of implying a selector always appears.
- Added a short paragraph in §2 noting this import form re-clones on every command with no local cache.

## Validation

- `cd website && npm run build` — clean build, no new broken-anchor warnings (confirmed the renamed
  section heading's new anchor, `#3-private-repo-access--one-token-set-once`, matches both updated
  cross-reference links via `grep -o 'id="3-private-repo[^"]*"' build/tutorials/centralized-auth-config/index.html`).
- No code changes were made (this is a docs-only fix), so no `go test`/`make lint` run applies.
- The corrected claims were themselves the product of live verification during the field test (real
  git-repo fixture, real `atmos auth list`/`atmos auth env` runs, direct reads of
  `custom_git_detector.go` and `pkg/config/imports.go`) — see the field-test conversation for the full
  repro commands.

## Follow-ups

None. All findings from the field-test report were either corrected here or already verified accurate
(no outstanding gaps identified beyond what's fixed above).
