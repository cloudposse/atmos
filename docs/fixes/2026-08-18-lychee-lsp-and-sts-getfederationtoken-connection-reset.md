# Fix: exclude microsoft.github.io/language-server-protocol and AWS STS GetFederationToken docs from link checking (CI connection reset)

**Date:** 2026-08-18

## Summary

CI's "Check Markdown Links" job (GitHub job ID 95759938607) failed with two errors, both
`Network error: Connection reset by peer (os error 104)`:

- `https://microsoft.github.io/language-server-protocol/` (referenced from `docs/prd/atmos-ai.md:2032`,
  `docs/prd/atmos-lsp.md:1367`, `website/docs/lsp/lsp-server.mdx:951`, and
  `website/blog/2026-03-09-introducing-atmos-lsp.mdx:771`)
- `https://docs.aws.amazon.com/STS/latest/APIReference/API_GetFederationToken.html` (referenced from
  `docs/prd/auth-console-command.md:799`)

Both URLs resolve fine outside CI (`curl -sL` returns `200`). Excluded both from `lychee.toml`,
following the repo's extensive existing precedent for this exact class of CI-runner-specific
connection flakiness (e.g. `taskfile.dev`, `otelic.com`, `docs.docker.com`, `concourse-ci.org`,
`reproducible-builds.org`).

## Context

Both flagged links are canonical, authoritative documentation pages (Microsoft's official LSP spec
site and AWS's official STS API reference) cited from PRD docs — not stale or genuinely broken
links. `curl` against both returned `200 OK` immediately outside CI, confirming the pages are live
and reachable; the failure is CI-runner-specific (the CI runner's outbound connection was reset, not
a real outage), matching the documented rationale already attached to numerous other `lychee.toml`
excludes for CI-hostile hosts.

## Changes

- `lychee.toml`: added two narrow `exclude` regex entries
  (`microsoft\.github\.io/language-server-protocol` and
  `docs\.aws\.amazon\.com/STS/latest/APIReference/API_GetFederationToken\.html`), each with a comment
  explaining the verified flakiness, matching the file's existing precedent style and placement at
  the end of the exclude list alongside the other recently-added connection-reset entries.

## Validation

- `curl -s -o /dev/null -w "%{http_code}" -L https://microsoft.github.io/language-server-protocol/` — `200`.
- `curl -s -o /dev/null -w "%{http_code}" -L https://docs.aws.amazon.com/STS/latest/APIReference/API_GetFederationToken.html` — `200`.
- `lychee --config lychee.toml docs/prd/atmos-ai.md docs/prd/auth-console-command.md` — both URLs now
  show `[EXCLUDED]`; `🔍 13 Total ✅ 6 OK 🚫 0 Errors 👻 6 Excluded 🔀 1 Redirects`.
- `lychee --config lychee.toml --root-dir "$(pwd)" '**/*.md'` (the same glob the CI workflow scans) —
  `🔍 1912 Total ✅ 1456 OK 🚫 1 Error`. The one remaining error (`bridgecrew.io`, connection refused)
  is inside a gitignored, locally-generated `.terraform/modules/this/README.md` artifact under
  `tests/fixtures/scenarios/native-ci-gha-plan/` from a prior local test run — not a tracked file, so
  it isn't part of what CI actually checks out and scans, and isn't touched by this fix.

## Follow-ups

None. Both hosts/paths are excluded from automated checking going forward, consistent with how the
other connection-flaky hosts in `lychee.toml` are handled.
