# Fix: unify TTL caching across all remote import forms

**Date:** 2026-08-12

## Summary

The `imports.ttl` setting added in PR #2923 only governed `git::` imports that use a subdirectory
(`resolveGitSubdir`/`ensureSourceDir`). Plain remote URLs and `git::` imports without a subdirectory went
through a separate code path (`RemoteImporter.Download`, backed by `pkg/cache.FileCache`) that cached
content indefinitely with no expiry at all, and never read `imports.ttl`. Extended `Download` to also
honor `imports.ttl`, so the same setting now controls caching for every remote import form uniformly.

## Context

Raised by the user while reviewing the PR #2923 diff for the "Caching Remote Imports" doc section: "This
caching should apply to all remote imports, no?" Verified the doc's scoping to `git::` subdirectory
imports was accurate for the code as it stood, but confirmed the underlying asymmetry was real — two
distinct caching mechanisms with different semantics for what is conceptually one feature.

`pkg/cache.FileCache` is a shared package used well beyond remote imports (auth credential caching,
container runtime discovery, Helm repo cache, provisioner lockfiles, HTTP proxy store). Changing its core
`Get`/`Set`/`GetOrFetch` semantics to add expiry was ruled out — that would risk affecting every other
consumer of the package, none of which were audited for TTL-safety. The fix instead adds freshness
tracking as a second cache entry local to `pkg/stack/imports/remote.go`, the same way the git-subdir path
already tracks freshness via a `sourceMetadata` JSON sidecar, without touching `FileCache` itself.

## Changes

- `pkg/stack/imports/remote.go`:
  - `Download(uri)` now reads `r.atmosConfig.Imports.TTL`, matching `Resolve(uri)`'s existing pattern.
  - Added `downloadMetaCacheKey`, `downloadCacheFresh`, and `writeDownloadMeta`, mirroring
    `ensureSourceDir`'s freshness-check pattern but storing the `sourceMetadata` sidecar as a second
    `FileCache` entry (key-prefixed `meta:`) rather than a file next to a cloned directory, since
    `Download`'s cache is a single content blob per URL, not a directory.
  - When `ttl` is set and the cached content is stale (or was never tracked), the entry is evicted before
    `GetOrFetch` runs, so a real fetch happens. The freshness timestamp is written only when a real fetch
    occurs (not on every cache hit), matching `fetchSourceDir`'s write-only-on-fetch semantics rather than
    sliding expiration.
  - Default (`ttl` unset) behavior is byte-for-byte unchanged for both import forms: `git::` subdirectory
    imports still re-clone every invocation, and plain URL/no-subdirectory imports are still cached
    forever. Only setting `imports.ttl` changes anything, and it now changes both uniformly.
- Docs updated to remove the now-inaccurate "plain remote URLs are not affected by this setting" claim,
  scoped only to `git::` subdirectory imports:
  - `website/docs/cli/configuration/imports.mdx` — "Caching Remote Imports" section rewritten to describe
    both defaults and the unified `ttl` behavior.
  - `website/blog/2026-08-11-remote-import-github-auth-and-caching.mdx` — "The Fix" and "How to Use It"
    sections updated (kept in ASD-STE100 style, matching the rest of that post).
  - `website/src/data/roadmap.js` — milestone description and benefits updated for the auth initiative's
    PR #2923 entry.

## Validation

- `go build ./pkg/stack/imports/...` — clean.
- New test `TestRemoteImporter_Download_HonorsConfigTTL` in `pkg/stack/imports/remote_test.go`: cold
  cache fetches; a second call within `ttl` reuses with no new request; backdating the freshness metadata
  past `ttl` forces a refresh; `ttl` unset preserves the historical cache-forever behavior. Assertions use
  request-count deltas rather than absolute counts, since the real downloader issues more than one HTTP
  request per logical fetch (confirmed unrelated to this change).
- `go test ./pkg/stack/imports/...` (full package) and `go test ./pkg/config/... ./pkg/cache/...` (to
  confirm the shared `FileCache` package's own tests are unaffected, since it was audited but not
  modified) — all pass.
- `cd website && npm run build` — clean, no new broken-anchor warnings.
- Verified via `node --input-type=module` import of `roadmap.js` that the milestone edit parses correctly
  and no other fields were disturbed.

## Follow-ups

None. The fix is scoped to the one asymmetry raised; `pkg/cache.FileCache` itself was intentionally left
unmodified to avoid any risk to its other consumers.
