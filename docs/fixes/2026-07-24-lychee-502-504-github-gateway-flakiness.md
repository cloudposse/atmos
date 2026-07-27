# Fix: Accept 502/504 from GitHub's gateway in lychee link checks

**Date:** 2026-07-24

## Summary

The `Check Markdown Links` CI job (lychee) intermittently fails with `502 Bad Gateway` and
`504 Gateway Timeout` on arbitrary `github.com` URLs. Added both status codes to `lychee.toml`'s
`accept` list, extending the existing precedent set for `500`.

## Context

CI run 89532521924 failed with 9 broken-link errors, all `github.com` URLs returning `502` or
`504`: two different `cloudposse/atmos` issue links, `cloudposse/atmos` and `cloudposse/.github`
tree/blob links, `github/github-mcp-server`, `99designs/aws-vault`, `opentofu/opentofu`, and even
a plain GitHub user profile (`github.com/mitchellh`). Curling all 9 outside CI returned `200` for
every one — this is the same "GitHub's gateway intermittently rejects requests under CI's
aggregate volume" flakiness `lychee.toml` already documents and accepts for `500` (see the
`accept` comment and the `v1.203.0` release-tag single-URL exclude precedent), just manifesting
as `502`/`504` this time instead of `500`. The failures span unrelated repos and URL shapes
(issue, tree, blob, release, user-profile), ruling out a single bad link — narrower per-repo
excludes (like the existing `google-gemini/gemini-cli` and `gruntwork-io/terragrunt` ones) would
just be whack-a-mole against the same underlying gateway flakiness recurring on different random
URLs each run.

## Changes

- `lychee.toml`: added `"502"` and `"504"` to the `accept` list alongside the existing `500`,
  updating the explanatory comment to cover all three codes and cross-reference the narrower
  per-repo 502 excludes already present for repos that hit this before the code was accepted
  broadly.

## Validation

- `python3 -c "import tomllib; tomllib.load(open('lychee.toml','rb'))"` — valid TOML.
- `lychee --config lychee.toml README.md` — 0 errors (previously-failing
  `github.com/cloudposse/.github/blob/main/CONTRIBUTING.md` now returns 200).
- `lychee --config lychee.toml docs/prd/toolchain-implementation.md docs/fixes/*.md` — 0 errors,
  covering the other failing links from the CI log (`abiosoft/colima`, `lima-vm/lima`,
  `actions/runner-images`, various `cloudposse/atmos` issue/PR links).
- Manually curled all 9 URLs named in the CI failure summary outside CI — all return `200`.

## Follow-ups

None.
