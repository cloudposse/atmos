# Fix: exclude two GitHub blob links that intermittently 404 from CI's gateway

**Date:** 2026-07-29

## Summary

CI's "Check Markdown Links" job failed with two `404 Not Found` errors, both on `github.com/.../blob/...`
deep links in `docs/prd/*.md` files unrelated to this PR's diff. Both URLs return `200 OK` when checked
directly with `curl` outside CI. Excluded both from `lychee.toml`, following the repo's existing, documented
pattern for this exact class of GitHub-gateway flakiness (e.g. the `google-gemini/gemini-cli` and
`gruntwork-io/terragrunt` repo-deep-link excludes, and the single `v1.203.0` release-tag exclude).

## Context

The failing run's diff only touched `internal/yq`, `pkg/utils`, `pkg/yaml`, `pkg/merge`, `pkg/container`,
and `website/package.json` — nothing in `docs/prd/`. The two flagged links:

- `docs/prd/atmos-ai-local-providers.md:1199`:
  `https://github.com/github/github-mcp-server/blob/main/docs/installation-guides/install-gemini-cli.md`
- `docs/prd/auth-console-command.md:797`: `https://github.com/99designs/aws-vault/blob/master/USAGE.md`

Both files exist on their repo's default branch (`github/github-mcp-server`'s `docs/installation-guides/`
lists `install-gemini-cli.md`; `99designs/aws-vault`'s default branch is `master`, which has `USAGE.md`), and
both URLs returned `HTTP/2 200` via `curl -sIL` when checked directly, twice, outside CI. `lychee.toml`
already documents multiple prior instances of GitHub's gateway intermittently rejecting valid `github.com`
deep links under CI's request volume (500/502/504 accepted broadly for this reason; `google-gemini/gemini-cli`
and `gruntwork-io/terragrunt` repo-wide, and a single `v1.203.0` release-tag URL, excluded narrowly) — this is
the same class of flakiness, just surfacing as `404` instead of `50x` this time.

## Changes

- `lychee.toml`: added two narrow `exclude` regex entries for the exact failing URLs, with a comment
  explaining the verified flakiness, matching the file's existing precedent style.

## Validation

- `curl -sIL` against both URLs — `200 OK`, checked twice.
- `atmos lint link-check` (`lychee --config lychee.toml '**/*.md'`) — `🚫 0 Errors` after the change.

## Follow-ups

These URLs are excluded from automated checking, so future staleness will require periodic manual
revalidation. Remove the exclusions once GitHub's gateway behavior stabilizes.
