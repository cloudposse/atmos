# Fix: exclude docs.docker.com from link checking (CI connection resets)

**Date:** 2026-07-30

## Summary

CI's "Check Markdown Links" job failed with one error: a `Network error: Connection reset by peer
(os error 104)` against `docs.docker.com` in `docs/prd/ecr-authentication.md`, a file unrelated to
this PR's diff. The URL resolves fine outside CI (`curl` shows a `301` redirect to the current docs
path, then `200 OK`). Excluded `docs.docker.com` from `lychee.toml`, following the repo's extensive
existing precedent for this exact class of CI-runner-specific connection-reset flakiness (e.g.
`taskfile.dev`, `geminicli.com`, `concourse-ci.org`, `otelic.com`).

## Context

The failing run's diff was PR #2812's CI git-clone bootstrap work — nothing in `docs/prd/`. The
flagged link:

- `docs/prd/ecr-authentication.md:1021`:
  `https://docs.docker.com/engine/reference/commandline/cli/#configuration-files`

`curl -sIL` against the URL returned `HTTP/2 301` (CloudFront-served redirect to
`/reference/cli/docker/`) followed by `HTTP/2 200`, confirming the page is live and reachable; the
failure is CI-runner-specific (CloudFront intermittently resetting connections from automated/CI
traffic), matching the documented rationale already attached to several other `lychee.toml`
excludes for CDN-fronted or bot-unfriendly hosts.

## Changes

- `lychee.toml`: added one narrow `exclude` regex entry (`docs\.docker\.com`) with a comment
  explaining the verified flakiness, matching the file's existing precedent style and placement
  alongside the similar `otelic.com` entry.

## Validation

- `curl -sIL` against the URL — `301` then `200 OK`.
- `lychee --config lychee.toml docs/prd/ecr-authentication.md` — `🔍 3 Total ✅ 2 OK 🚫 0 Errors 👻
  1 Excluded` (the docs.docker.com link now shows `[EXCLUDED]`).

## Follow-ups

None. This URL is excluded from automated checking going forward, consistent with how the other
connection-reset-prone hosts in `lychee.toml` are handled.
