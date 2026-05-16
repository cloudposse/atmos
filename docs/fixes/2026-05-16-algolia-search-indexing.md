# Algolia Search Indexing: Crawler Config as Code

**Date:** 2026-05-16

## Problem

Site search on `atmos.tools` had several long-running quality issues that the
DocSearch frontend alone could not fix:

- Decorative card titles rendered as `<h2>` polluted the `hierarchy.lvl2`
  field, so command pages were buried under landing-page card text.
  (Initial fix: render decorative card titles as `div` — PR #2400.)
- Anchor and trailing-slash variants of the same URL produced multiple
  duplicate hits per page in the result list.
- The crawler had no taxonomy or page-rank signal, so reference and
  configuration pages outranked the command pages users were actually
  searching for (e.g., searching `atmos auth` returned configuration
  reference before the command docs).
- The crawler configuration lived in the Algolia dashboard UI: it was not
  versioned, not reviewable, and any deploy step had to be done by hand
  from a browser tab. Drift between intent and live config was effectively
  invisible.

Then, when we moved the config into the repo and started deploying it
automatically, the Algolia Crawler API rejected our payload with two
classes of error we had not anticipated.

## Root Cause

### Search ranking

The crawler's `recordExtractor` produced records with no `lvl0` taxonomy
or `pageRank` signal, and the index had no `customRanking` configured to
use those signals even if they existed. Anchor-level deduplication was
disabled in the index settings, so any page with multiple headings turned
into many indistinguishable hits.

### Closure references in serialized recordExtractor

The deploy script (`website/algolia/deploy-crawler-config.mjs`) sends the
crawler config to Algolia via `Function.prototype.toString()` on the
`recordExtractor` callback. Algolia runs the resulting source in an
**isolated sandbox** and ships it through a server-side linter. Closure
references to module-scope helpers (e.g., `getPathname`,
`getLvl0ForPath`, `getPageRankForPath`) are not visible to that sandbox,
so the linter rejected the config with:

```text
[no-undef] at line 1: 'getPathname' is not defined.
[no-undef] at line 7: 'getLvl0ForPath' is not defined.
[no-undef] at line 16: 'getPageRankForPath' is not defined.
```

### ES2020+ syntax in serialized recordExtractor

Algolia's server-side linter parses the function source at roughly
ECMAScript 2017. It rejects optional chaining (`?.`), nullish coalescing
(`??`), and binding-less `catch {}`. After inlining the helpers, the next
deploy failed with:

```text
[null] at line 38: Parsing error: Unexpected token .
```

This is not documented in Algolia's public API reference. The only way
to discover it without re-running a full deploy is to parse the
serialized function source against an ES2017 grammar locally.

## Fix

### Search ranking

`website/algolia/atmos-tools.crawler.js` now produces a `recordExtractor`
that:

- Derives `lvl0` from a URL-path taxonomy (CLI Commands, CLI Configuration,
  Tutorials, Changelog, etc.) instead of trusting page-level structure.
- Assigns a `pageRank` weight based on the same URL-path bucketing.
- Returns DocSearch v3 records via `helpers.docsearch(...)`.

The index settings configure `customRanking: [desc(weight.pageRank),
desc(weight.level), asc(weight.position)]` and use `distinct: true` with
`attributeForDistinct: url` so anchor/slash duplicates collapse.

### recordExtractor must be self-contained and ES2017-safe

The `recordExtractor` function in `atmos-tools.crawler.js` was rewritten so:

- All taxonomy/page-rank tables and all helper functions are defined
  **inside** the function body. Nothing leaks in from module scope.
- The function body uses only ES2017 syntax — explicit `x && x.prop`
  instead of `x?.prop`, `catch (_err)` instead of `catch {}`, indexed
  array access instead of array-destructuring patterns.

The module also still exports the same helpers (`getPathname`,
`getLvl0ForPath`, `getPageRankForPath`, `normalizePathname`) — these are
for unit-test coverage and are not what the runtime crawler executes.

### Deploy pipeline

`.github/workflows/algolia.yaml` validates and deploys the crawler config:

- `pull_request` runs `pnpm run test:algolia` and
  `pnpm run algolia:deploy:dry-run`. No secrets are required.
- `push` to `main` deploys crawler config + index settings from the
  `algolia` GitHub environment.
- `workflow_dispatch` lets an admin deploy from any branch (used once,
  before merging this PR, to validate the live deploy path).

The production content reindex is split into its own job
(`algolia-reindex-prod` in `.github/workflows/website-deploy-prod.yml`)
so that AWS production secrets stay in the `production` environment and
Algolia crawler secrets stay in the `algolia` environment.

### Pre-deploy CI guards

`website/algolia/atmos-tools.crawler.test.mjs` runs three secret-free
guards on every PR. These two would have caught every deploy-time
failure in this PR before a single Algolia request was made:

1. **JSON Schema (`crawler-config.schema.json` + Ajv2020)** — fails on
   structural drift: missing required fields, wrong types, empty arrays.
   Algolia does not publish a public schema for the Crawler API, so this
   codifies the contract we depend on.
2. **acorn parse at `ecmaVersion: 2017`** — fails on `?.`, `??`,
   binding-less `catch`, or any post-ES2017 syntax in the serialized
   `recordExtractor` source.
3. **eslint-scope reference analysis** — fails on any identifier
   referenced by `recordExtractor` that is not declared inside the
   function and not in an explicit allow-list of standard built-ins
   (`URL`, `String`, `Math`, `JSON`, `console`, ...). This catches
   closure references to module-scope helpers before they reach the
   Algolia linter.

## Expected Behavior

- `pnpm run test:algolia` passes locally and on every PR.
- `pnpm run algolia:deploy:dry-run` prints a redacted, serialized config
  without making any network call.
- Adding a new helper to the `recordExtractor` requires either inlining
  it inside the function body or adding the identifier to
  `ALLOWED_EXTRACTOR_GLOBALS` in `atmos-tools.crawler.test.mjs`. The CI
  guards will reject anything else.
- Using ES2018+ syntax (optional chaining, nullish coalescing,
  binding-less catch, etc.) inside the `recordExtractor` fails the
  parse test.
- Searching for `atmos auth` on `atmos.tools` returns the CLI command
  docs before configuration reference pages.

## References

- PR #2400 — render decorative card titles as `<div>` to fix Algolia
  search relevance.
- PR #2406 — this work (crawler config as code, deploy pipeline, CI
  guards).
- `docs/algolia-search-indexing.md` — operator-facing setup and
  troubleshooting guide.
- `website/algolia/README.md` — directory-level pointers for the
  crawler config, deploy script, and tests.
