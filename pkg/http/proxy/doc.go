// Package proxy implements a generic, protocol-agnostic caching HTTP proxy.
//
// The proxy is the reusable core of the Atmos registry cache. It owns nothing
// Terraform-specific: protocol knowledge lives in Mirror adapters (the provider
// and module registry mirrors, and a future git mirror) that map an inbound
// request to a Route — a cache key, an upstream request, an artifact kind, and
// optional rewrite/verify hooks.
//
// Design tenets:
//
//   - The cache key is authoritative and is the backend object name; the
//     filesystem path IS the key. There is no secondary index.
//   - Fetch-once on a miss: no retries, no backoff. Retry/backoff is the caller's
//     (Atmos's) responsibility, keeping failures observable.
//   - One downloader, many readers per key, via three tiers of concurrency control
//     (see serveCacheable):
//     1. A lock-free hit fast path — safe because both the object and its sidecar
//     are written atomically (temp file + rename), so an unlocked reader never
//     sees a torn file.
//     2. In-process singleflight collapses concurrent cold-key fills in one process
//     (the Atmos bulk case: many terraform child processes share one proxy);
//     waiters block with no timeout and cancel cleanly when their client leaves.
//     3. A cross-process pkg/cache.FileLock (context-bounded) collapses fills across
//     processes that share a cache directory, held only around fetch + commit —
//     never while streaming to the client. On Windows the file lock degrades to a
//     no-op, so concurrent processes may redundantly download a cold key; the
//     atomic commit still guarantees no corruption.
//   - Atomic commit: stream to a temp file, hash, verify, then rename into place.
//   - Per-run, in-memory statistics only (hits + bytes saved) for the savings
//     report — no persistent hit/miss store.
//   - Credentials and headers are forwarded to upstream so private registries keep
//     working, and Terraform's User-Agent is forwarded verbatim.
package proxy
