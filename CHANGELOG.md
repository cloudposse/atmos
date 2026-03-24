# Changelog

All notable changes to this project will be documented in this file.
See [Conventional Commits](https://www.conventionalcommits.org/) for commit message format.

---

## [Unreleased]

### Changed

- **`pkg/filesystem.GetGlobMatches`**: always returns a non-nil `[]string{}` (never `nil`).
  Callers must use `len(result) == 0` to detect no matches instead of `result == nil`.
  The cache is now bounded and configurable via three environment variables:
  - `ATMOS_FS_GLOB_CACHE_MAX_ENTRIES` (default `1024`, minimum `16`) — maximum number of cached glob patterns.
  - `ATMOS_FS_GLOB_CACHE_TTL` (default `5m`, minimum `1s`) — time-to-live for each cache entry.
    Values below the respective minimums are clamped up rather than rejected.
  - `ATMOS_FS_GLOB_CACHE_EMPTY` (default `1`) — set to `0` to skip caching patterns that match no files.
- **`pkg/http.normalizeHost`**: now strips default ports (`:443`, `:80`) in addition to
  lower-casing and removing trailing dots, so that `api.github.com:443` is treated
  identically to `api.github.com` for allowlist matching.

### Added

- **`pkg/filesystem`**: expvar observability counters (`atmos_glob_cache_hits`,
  `atmos_glob_cache_misses`, `atmos_glob_cache_evictions`, `atmos_glob_cache_len`) published
  via `RegisterGlobCacheExpvars()` and accessible at `/debug/vars` when the HTTP debug
  server is enabled.
- **`pkg/http`**: host-matcher three-level precedence documented and tested:
  1. `WithGitHubHostMatcher` — custom predicate always wins.
  2. `GITHUB_API_URL` — GHES hostname added to allowlist when set.
  3. Built-in allowlist — `api.github.com`, `raw.githubusercontent.com`, `uploads.github.com`.
  Authorization is only injected over HTTPS and stripped on cross-host redirects
  (301 / 302 / 303 / 307 / 308) to prevent token leakage.

### Security

- Cross-host HTTP redirects (all five status codes: 301, 302, 303, 307, 308) no longer
  forward the `Authorization` header to the redirect target, preventing accidental token
  leakage via open redirects.
