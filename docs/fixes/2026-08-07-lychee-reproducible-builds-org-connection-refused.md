# Fix: exclude reproducible-builds.org from link checking (CI connection refused)

**Date:** 2026-08-07

## Summary

CI's "Check Markdown Links" job (GitHub job ID 92868414228) failed with one error:
`https://reproducible-builds.org/docs/source-date-epoch/` in `docs/prd/archive-step.md:175` —
`Connection refused - server may be down or port blocked`. The URL resolves fine outside CI
(`curl -sIL` returns `200 OK`). Excluded `reproducible-builds.org` from `lychee.toml`, following
the repo's extensive existing precedent for this exact class of CI-runner-specific connection
flakiness (e.g. `taskfile.dev`, `otelic.com`, `docs.docker.com`, `concourse-ci.org`).

## Context

The flagged link:

- `docs/prd/archive-step.md:175`: `https://reproducible-builds.org/docs/source-date-epoch/`, cited
  as the origin of the `SOURCE_DATE_EPOCH` convention that `mtime: epoch` mirrors conceptually.

`curl -sIL` against the URL returned `HTTP/1.1 200 OK` immediately, confirming the page is live and
reachable; the failure is CI-runner-specific (the CI runner's outbound request was refused, not a
real outage), matching the documented rationale already attached to numerous other `lychee.toml`
excludes for CI-hostile hosts.

## Changes

- `lychee.toml`: added one narrow `exclude` regex entry (`reproducible-builds\.org`) with a comment
  explaining the verified flakiness, matching the file's existing precedent style and placement
  alongside the similar `cis.upenn.edu` PDF entry.

## Validation

- `curl -sIL --max-time 15 https://reproducible-builds.org/docs/source-date-epoch/` — `200 OK`.
- `lychee --config lychee.toml docs/prd/archive-step.md` — `🔍 11 Total ✅ 9 OK 🚫 0 Errors 👻 2
  Excluded` (the `reproducible-builds.org` link now shows `[EXCLUDED]`, alongside the pre-existing
  `atmos.tools` exclude).

## Follow-ups

None. This host is excluded from automated checking going forward, consistent with how the other
connection-flaky hosts in `lychee.toml` are handled.
