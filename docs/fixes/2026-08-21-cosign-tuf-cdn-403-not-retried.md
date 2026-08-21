# Toolchain Verification: Cosign's TUF Trust-Root CDN Fetch Errors Weren't Classified as Retryable

**Date:** 2026-08-21
**Severity:** Medium — spurious `atmos toolchain install` / CI failures on transient Sigstore TUF CDN blips
**Reproducer:** `pkg/toolchain/verification/signature_rekor_test.go` —
`TestClassifySignatureVerificationError/TUF_CDN_root.json_fetch_403_is_retryable`,
`TestClassifySignatureVerificationError/TUF_CDN_timestamp.json_fetch_403_is_retryable`,
`TestRunCosignWithRetry_RecoversFromTUFCDNFetch403`

---

## Symptom

CI job `[mock-linux] examples/demo-context` on PR #2974 failed installing `opentofu/opentofu`:

```text
✗ Install failed opentofu/opentofu@1.12.2: failed to verify downloaded asset: signature verification failed:
  .../cosign [verify-blob --certificate ... --signature ... SHA256SUMS]: exit status 1
  WARNING: Could not fetch trusted_root.json from the TUF repository. Continuing with individual targets.
  Error from TUF: error getting live trusted root: failed to create TUF client failed to load metadata:
  tuf refresh failed: failed to download https://tuf-repo-cdn.sigstore.dev/16.root.json, http status code: 403
  Error: getting Rekor public keys: updating local metadata and targets: error updating to TUF remote
  mirror: tuf: failed to download timestamp.json: GET "https://tuf-repo-cdn.sigstore.dev/timestamp.json":
  unexpected HTTP status 403
```

Same as the `2026-07-07` cosign-direct-fetch-5xx fix, the install failed on the first attempt —
no retry occurred — even though `runCosignWithRetry` exists specifically to survive transient
upstream flakes.

---

## Root Cause

Before every `verify-blob` invocation, cosign refreshes its Sigstore TUF trust-root/timestamp
metadata by fetching `16.root.json` and `timestamp.json` from `tuf-repo-cdn.sigstore.dev`. This
is a third distinct network call, separate from both:

- the Rekor transparency-log `/api/v1/log/entries/retrieve` endpoint (`rekorTlogEndpointMarker`,
  already retried), and
- a direct `--certificate`/`--signature` asset fetch (`cosignHTTPFetchMarker`, added in the
  2026-07-07 fix).

`isRetryableSignatureError` had no marker at all for this TUF-CDN fetch shape, so a transient
`403` from the CDN — an upstream blip unrelated to the actual signature being verified — was left
unclassified, `isRetryableCosignError` returned `false`, and the retry loop gave up after one
attempt.

---

## Fix

Added a fourth classification rule to `pkg/toolchain/verification/signature_rekor.go`, scoped
tightly to the TUF CDN host rather than a bare `"403"` match anywhere in cosign output — 403 is a
terminal authorization failure on most endpoints (see `pkg/toolchain/registry/githubratelimit.go`,
which only treats 403 as retryable when GitHub's rate-limit headers are present), so an unscoped
match would risk silently retrying away a real permissions problem on an unrelated request:

```go
const tufCDNHostMarker = "tuf-repo-cdn.sigstore.dev"

var tufCDNRetryableStatusMarkers = []string{"http status code: 403", "unexpected HTTP status 403"}

func hasTUFCDNRetryableStatus(msg string) bool {
    if !strings.Contains(msg, tufCDNHostMarker) {
        return false
    }
    return matchesAnyMarker(msg, tufCDNRetryableStatusMarkers)
}
```

Both message phrasings observed in the CI log are matched (cosign's TUF client reports a non-2xx
CDN response two different ways depending on which metadata file it was fetching). This is safe
to broaden because:

- The error occurs while cosign is still refreshing trust metadata — before any signature verdict
  is rendered. It can never mask a real verification failure (tampering, expired cert, identity
  mismatch).
- The match requires both the TUF CDN hostname *and* one of the two observed status phrasings, so
  a 403 anywhere else in cosign's output (e.g. a genuine auth failure fetching a private asset)
  stays unclassified and surfaces immediately — covered by the
  `403_outside_the_TUF_CDN_host_is_NOT_retryable` test case.

No changes were needed to `runCosignWithRetry`, `cosignRetryConfig`, or `isRetryableCosignError` —
the only gap was the classifier not wrapping this particular error shape.

---

## Tests

`pkg/toolchain/verification/signature_rekor_test.go`:

- `TestClassifySignatureVerificationError` — added cases for a `16.root.json` fetch 403, a
  `timestamp.json` fetch 403, and a 403 outside the TUF CDN host (must NOT be retried), using the
  verbatim error text observed in CI.
- `TestRunCosignWithRetry_RecoversFromTUFCDNFetch403` — end-to-end: a flaky runner that fails
  twice with the CI's exact `16.root.json ... http status code: 403` error, then succeeds; asserts
  `runCosignWithRetry` retries and ultimately succeeds after 3 calls.

---

## Verification

1. `go test ./pkg/toolchain/verification/... -run 'TestClassifySignatureVerificationError|TestRunCosignWithRetry'`
   — new cases pass; all existing Rekor/transport-flake/cosign-HTTP-fetch and non-retryable cases
   (tampering, identity mismatch, unscoped 403) remain correctly classified.
2. `go test ./pkg/toolchain/verification/...` — full package suite passes, no regressions.
3. `go build ./...` — clean.

---

## Related

- `pkg/toolchain/verification/signature.go`: `runCosignWithRetry`, cosign invocation.
- `pkg/toolchain/verification/signature_rekor.go`: `classifySignatureVerificationError`,
  `isRetryableSignatureError`, `hasTUFCDNRetryableStatus`.
- `docs/fixes/2026-07-07-cosign-direct-fetch-http-5xx-not-retried.md` — the prior fix for cosign's
  *direct* asset-fetch errors; this fix covers a distinct network call (TUF trust-root refresh)
  that neither that fix nor the Rekor-tlog handling covered.
- PR #2974 CI run: `[mock-linux] examples/demo-context`, job ID `96885492588`.

---

## Known unrelated flake in the same CI run (not fixed here)

The same PR's `Acceptance Tests (windows, shard 7/10)` job failed independently with:

```text
Head "https://github.com/helmfile/helmfile/releases/latest": context deadline exceeded
--- FAIL: TestExecuteHelmfile_Version (26.47s)
```

This is the real `helmfile` binary's own internal update-check HTTP call (made by the
toolchain-installed `helmfile` CLI itself when running `helmfile version`), not a call atmos makes
— `internal/exec/helmfile.go`'s `version` handler shells out to `helmfile version` with no extra
flags or env vars. No suppression flag/env var for helmfile's own update check is currently wired
into atmos's invocation, and no other `_Version` test in the package (`TestExecuteTerraform_Version`,
`TestExecutePacker_Version`) guards against this class of external check either, so there's no
existing convention to extend. This is upstream/network flakiness outside atmos's code; re-run the
job rather than patch code for it.
