# Fix: retry `go install go-licenses` in `tools/noticegen` on transient sum.golang.org failures

**Date:** 2026-08-31

## Summary

`Review Dependency Licenses` failed with:

```text
go: github.com/google/go-licenses@v1.6.0: version constraints conflict:
  github.com/google/go-licenses@v1.6.0 indirectly requires github.com/googleapis/enterprise-certificate-proxy@v0.0.0-20220520183353-fd19c99a87aa: verifying go.mod: github.com/googleapis/enterprise-certificate-proxy@v0.0.0-20220520183353-fd19c99a87aa/go.mod: reading https://sum.golang.org/tile/8/0/x042/147: stream error: stream ID 1155; INTERNAL_ERROR; received from peer
##[error]Process completed with exit code 1.
```

A transient mid-stream HTTP/2 reset talking to `sum.golang.org` (the Go checksum database) during
`go install`'s go.sum verification, unrelated to any actual dependency problem -- the exact same
failure class already fixed once for `go mod download` in
`docs/fixes/2026-08-25-build-atmos-go-mod-download-retry.md` (later ported to
`magefiles/build.go`'s `runGoModDownload` per
`docs/fixes/2026-08-26-merge-main-go-mod-download-retry-port.md`), just hitting a different
network call (`go install`'s dependency-graph resolution, not `go mod download`) in a different
script.

## Context

The original NOTICE generator, `scripts/generate-notice.sh`, called
`go install "github.com/google/go-licenses@${GO_LICENSES_VERSION}"` with no retry logic, so a
single mid-stream `sum.golang.org` hiccup failed the whole NOTICE regeneration outright, before it
ever got to actually scanning dependencies. Confirmed via `gh api
repos/cloudposse/atmos/actions/jobs/99522359137` that this ran against `head_sha: 5ac5d9739f` --
the immediately preceding commit on this branch, which itself only touched the `REPO_OVERRIDES`
list and `NOTICE` for an unrelated `cuelang.org/go` URL fix
(`docs/fixes/2026-08-31-required-check-gates-fail-on-cancelled-run.md`'s sibling commit) -- ruling
out a regression from that change.

`scripts/generate-notice.sh` has since been replaced by `tools/noticegen`, a Go tool invoked via
`go tool mage notice:generate` (see the commit that rewrote the NOTICE generator as a Go tool + mage
target). The retry fix below was ported into `tools/noticegen/report.go`'s `ensureGoLicenses` as
part of that rewrite, so it survives in the current implementation even though the original shell
script it was written against no longer exists.

## Changes

- `tools/noticegen/report.go`: `ensureGoLicenses` wraps its `go install` call (via the
  package-level `runGoInstall` var) in a 3-attempt/15s-backoff retry loop (`goInstallMaxAttempts`,
  `goInstallRetryDelay`, `goInstallSleep`), matching the established convention
  (`.github/actions/download-artifact-retry`, `magefiles/build.go`'s `runGoModDownload`).
- `tools/noticegen/report_test.go`: `TestEnsureGoLicensesRetriesOnTransientInstallFailure` and
  `TestEnsureGoLicensesFailsAfterExhaustingRetries` cover the retry loop by faking `runGoInstall`
  and `goInstallSleep`, so the tests run without real subprocesses, network access, or sleeps.

## Validation

- `cd tools/noticegen && go test ./... -run 'TestEnsureGoLicenses' -v` -- both new tests pass.
- `cd tools/noticegen && go test ./...` -- full package suite passes.
- Not exercised end-to-end against a real `sum.golang.org` outage (not reproducible on demand);
  the fix is a direct, narrow port of an already-validated pattern (see the two referenced prior
  fix docs) to a second call site hitting the same failure class.

## Follow-ups

None.
