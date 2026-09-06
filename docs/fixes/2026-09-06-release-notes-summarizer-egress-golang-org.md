# Fix: release-notes summarizer failed at "Set up Go" (golang.org egress blocked)

**Date:** 2026-09-06

## Summary

The `summarize-notes` job in `.github/workflows/release.yml` (the AI rewrite of
drafted release notes, added in #3045) failed on the first real release that
reached it, at the `Set up Go` step - before any summarization ran - with:

```
##[error]getaddrinfo EAI_AGAIN golang.org
```

The overall `Release` run therefore showed `failure`, even though the release
itself (binaries, SBOMs, draft notes) was fine.

## Root cause

The job runs `step-security/harden-runner` with `egress-policy: block` and a
fixed `allowed-endpoints` list. `actions/setup-go` resolves and downloads the Go
toolchain via `golang.org` (redirecting to `go.dev`), the `actions/go-versions`
manifest on `raw.githubusercontent.com`, and the toolchain archive on
`dl.google.com` - none of which were in the allow-list. harden-runner blocked the
`golang.org` DNS lookup, so `setup-go` died with `EAI_AGAIN`. The allow-list only
had `proxy.golang.org`/`sum.golang.org`/`google.golang.org` (module proxy +
checksum DB), which cover `go build` but not toolchain provisioning. The job's
comment claimed the set mirrored `test.yml`'s build job, but it did not.

## Fix

**`.github/workflows/release.yml`** - add the `setup-go` toolchain hosts to the
`summarize-notes` job's `allowed-endpoints`:

```
raw.githubusercontent.com:443
golang.org:443
go.dev:443
dl.google.com:443
```

## Notes

- This was never the 125k release-body overflow the summarizer protects against;
  the draft body was ~5 KB. When this job fails, release-drafter's categorized
  skeleton stays as the release body, which is small and safe.
- The fix only takes effect for `Release` runs built from a commit that contains
  it; re-running an old run re-checks-out the pre-fix workflow and fails again.
