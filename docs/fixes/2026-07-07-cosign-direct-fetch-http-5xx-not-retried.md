# Toolchain Verification: Cosign's Own URL Fetch Errors Weren't Classified as Retryable

**Date:** 2026-07-07
**Severity:** Medium — spurious `atmos toolchain install` / CI failures on transient CDN blips
**Reproducer:** `pkg/toolchain/verification/signature_rekor_test.go` —
`TestClassifyCosignError/cosign_--certificate_fetch_504_is_retryable`,
`TestRunCosignWithRetry_RecoversFromCertificateFetch504`

---

## Symptom

CI job `[mock-linux] examples/demo-component-versions` on PR #2700 failed installing
`opentofu/opentofu`:

```text
✗ Install failed : failed to verify downloaded asset: signature verification failed:
.tools/bin/sigstore/cosign/v3.1.1/cosign [verify-blob --certificate
https://github.com/opentofu/opentofu/releases/download/v1.12.2/tofu_1.12.2_SHA256SUMS.pem
--certificate-identity-regexp ... --certificate-oidc-issuer
https://token.actions.githubusercontent.com --signature
https://github.com/opentofu/opentofu/releases/download/v1.12.2/tofu_1.12.2_SHA256SUMS.sig
/tmp/atmos-checksum-1127573497.2_SHA256SUMS]: exit status 1
  Error: loading verifier from key opts: loading cert: loading URL
  https://github.com/opentofu/opentofu/releases/download/v1.12.2/tofu_1.12.2_SHA256SUMS.pem:
  server returned HTTP 504
```

The install failed on the first attempt — no retry occurred — even though the toolchain
verification layer has retry logic (`runCosignWithRetry` in
`pkg/toolchain/verification/signature.go`) specifically built to survive transient upstream
flakes.

---

## Root Cause

`atmos toolchain install` shells out to the `cosign` binary to verify a downloaded asset's
signature. When invoked with `--certificate <URL>` / `--signature <URL>` (as opposed to local
file paths), **cosign fetches those URLs itself**, using its own internal HTTP client — atmos
never sees or makes that request. This means:

- The toolchain's asset-download retry logic (`pkg/toolchain/installer/download.go`,
  `downloadToCache` / `isRetryableHTTPStatus`) never applies, because atmos didn't make the
  request — cosign did, as a side effect of running `verify-blob`.
- The cosign subprocess invocation itself *is* wrapped in retry logic
  (`runCosignWithRetry` → `classifyCosignError` → `isRetryableCosignError`, in
  `pkg/toolchain/verification/signature.go` and `signature_rekor.go`), but that classifier only
  recognized two narrow categories of transient failure:
  1. Known Sigstore Rekor transparency-log API flakes (`rekorFlakeMarkers`, and 5xx status codes
     scoped specifically to the `/api/v1/log/entries/retrieve` endpoint via
     `rekorTlogEndpointMarker`).
  2. Generic network transport failures (`transportFlakeMarkers`: connection reset, TLS handshake
     timeout, i/o timeout, unexpected EOF, HTTP/2 stream errors).

A `504` from GitHub's release-asset CDN while cosign loads the `--certificate` PEM file matches
neither category: it isn't the Rekor tlog endpoint, and the error text (`server returned HTTP
504`) isn't one of the transport-error substrings. So `classifyCosignError` returned the error
unchanged, `isRetryableCosignError` returned `false`, and the retry loop gave up after the first
attempt.

In short: **the retry plumbing existed and was wired in correctly, but the error classifier's
allowlist was too narrow** — it covered Rekor-specific and low-level transport flakes, but not
generic upstream HTTP 5xx/429 responses from a direct cosign URL fetch, even though this is
exactly the same class of transient failure the toolchain's own downloader already treats as
retryable (`isRetryableHTTPStatus` in `download.go` covers 429, 500, 502, 503, 504).

---

## Fix

Added a third classification rule to `classifyCosignError`
(`pkg/toolchain/verification/signature_rekor.go`) that matches cosign's generic HTTP-fetch error
format — `server returned HTTP <code>` — against the same retryable status set the toolchain
downloader already uses (429, 500, 502, 503, 504):

```go
const cosignHTTPFetchMarker = "server returned HTTP "

var cosignHTTPFetchRetryableStatuses = []string{"429", "500", "502", "503", "504"}

// ...inside classifyCosignError:
if strings.Contains(msg, cosignHTTPFetchMarker) {
    for _, status := range cosignHTTPFetchRetryableStatuses {
        if strings.Contains(msg, cosignHTTPFetchMarker+status) {
            return errors.Join(errUtils.ErrSignatureRetryable, err)
        }
    }
}
```

This is safe to broaden (unlike, say, matching bare `"5xx"` anywhere in cosign output) because:

- The error occurs while cosign is still *loading* the certificate/signature — before any
  signature verdict is rendered. It can never mask a real verification failure (tampering,
  expired cert, identity mismatch), since those errors have a different, already-excluded shape.
- The status codes matched are the exact same set the toolchain's own asset downloader already
  treats as transient, keeping the two retry policies consistent.
- A non-retryable status via the same code path (e.g. a genuine `404` for a missing asset) is
  correctly left unclassified and surfaces immediately — covered by the
  `cosign_--certificate_fetch_404_is_NOT_retryable` test case.

No changes were needed to `runCosignWithRetry`, `cosignRetryConfig`, or
`isRetryableCosignError` — they already correctly retry any error wrapped in
`ErrSignatureRetryable`; the only gap was the classifier not wrapping this particular error shape.

---

## Tests

`pkg/toolchain/verification/signature_rekor_test.go`:

- `TestClassifyCosignError` — added cases for a `--certificate` fetch 504, a `--signature` fetch
  503, and a `--certificate` fetch 404 (must NOT be retried), using the verbatim error text
  observed in CI.
- `TestRunCosignWithRetry_RecoversFromCertificateFetch504` — end-to-end: a flaky runner that
  fails twice with the CI's exact `SHA256SUMS.pem ... server returned HTTP 504` error, then
  succeeds; asserts `runCosignWithRetry` retries and ultimately succeeds after 3 calls.

---

## Verification

1. `go test ./pkg/toolchain/verification/... -run 'TestClassifyCosignError|TestRunCosignWithRetry'`
   — new cases pass; all existing Rekor/transport-flake and non-retryable cases (tampering,
   identity mismatch) remain correctly classified.
2. `go test ./pkg/toolchain/...` — full package suite passes, no regressions.
3. `go build ./...` — clean.

---

## Related

- `pkg/toolchain/verification/signature.go`: `runCosignWithRetry`, cosign invocation.
- `pkg/toolchain/verification/signature_rekor.go`: `classifyCosignError`,
  `isRetryableCosignError`, `cosignRetryConfig`.
- `pkg/toolchain/installer/download.go`: `isRetryableHTTPStatus` — the toolchain downloader's
  equivalent retryable-status set, which this fix now mirrors for cosign's direct fetches.
- PR #2700 CI run: `[mock-linux] examples/demo-component-versions`,
  `https://github.com/cloudposse/atmos/actions/runs/28899952842/job/85737775862`.
