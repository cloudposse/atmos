# Fix: `website-deploy-preview` CI failure from a duplicate `nanoid` pnpm override

**Date:** 2026-08-10

## Summary

The `website-deploy-preview` GitHub Actions job failed at `pnpm install --frozen-lockfile` with
`ERR_PNPM_BROKEN_LOCKFILE: The lockfile at ".../website/pnpm-lock.yaml" is broken: duplicated
mapping key (29:3)`. `website/package.json`'s `pnpm.overrides` block contained the same key,
`"nanoid@^3.3.16": "^3.3.17"`, twice (lines 123 and 125) — JSON silently tolerates a duplicate
object key, but pnpm's lockfile writer emitted both into the `overrides:` section of
`pnpm-lock.yaml` as sibling YAML mapping entries, which is invalid YAML and breaks every
subsequent `pnpm install`.

## Context

Two independent commits each added the `nanoid@^3.3.16` override entry to fix separate
`nanoid` DoS advisories, and both landed on `main` via history merged into this branch
(`osterman/toolchain-update-pinning-field-test`, via the `origin/main` merge earlier in this
session): `f31c1ec998` added it once, and `ee9769f130` added it again without noticing the first
was already present. Since JSON silently accepts a duplicate key (only the last value wins when
parsed as a plain object) this didn't fail anything locally with a simple `pnpm install`, but
regenerating/reading the *lockfile*, which mirrors `overrides` into its own YAML mapping,
surfaces the duplicate as a hard parse error — exactly what `--frozen-lockfile` (the flag CI's
`website-deploy-preview` job uses) hit.

A different concurrent session (branch `osterman/make-migration-skill`, commit `47a247f8c5`) hit
and fixed the identical root cause independently around the same time; that branch's fix also
needed to fix 5 migration doc pages linking to a stale `/ai/agent-skills` route that only surfaced
once the lockfile was working again. Checked for that same follow-on issue here — this branch
doesn't have those particular migration doc pages yet, so no equivalent second issue applies.

## Changes

- `website/package.json` — removed the duplicate `"nanoid@^3.3.16": "^3.3.17"` entry from
  `pnpm.overrides` (kept the first occurrence; both were identical values, so there was no
  substantive conflict to resolve, just the redundant key).
- `website/pnpm-lock.yaml` — regenerated via `pnpm install --lockfile-only` to drop the
  now-single `nanoid@^3.3.16` mapping entry.

## Validation

- `pnpm install --lockfile-only` — regenerated cleanly (also flagged and recovered from the
  broken lockfile it replaced, confirming the prior state was genuinely broken, not a local
  fluke).
- `pnpm install --frozen-lockfile` — the exact command `website-deploy-preview` runs — now
  succeeds.
- `pnpm run build:site` — full Docusaurus build completes with `[SUCCESS] Generated static files
  in "build"`. One pre-existing, unrelated `[WARNING] Docusaurus found broken anchors!` for
  `/changelog/mcp-for-ai-coding-assistants` remains (a warning, not a build failure, and outside
  the scope of this fix).
- Not yet pushed.

## Follow-ups

None.
