# Fix: Upgrade Node.js to 24.22.1 and Go to 1.26.10 for October 2026 CVEs

**Date:** 2026-10-23

## Summary

Move every exposed Node.js pin to 24.22.1, Go to 1.26.10, and three website
pnpm overrides to patched releases.

## Context

The [October 2026 Node.js security releases](https://nodejs.org/en/blog/vulnerability/october-2026-security-releases)
fix four High-severity CVEs:

- CVE-2026-111497: Brotli out-of-bounds write that allows remote code execution.
- CVE-2026-105483 and CVE-2026-106069: TLS hostname verification bypasses.
- CVE-2026-105482: `dns.resolve` truncates names at a NUL character.

Atmos downloads a Node runtime for the AWS SAML Playwright driver, and that pin
was 24.18.0. The website pinned Node 25.1.0. Node 25 is end-of-life and will not
get a patch.

Go 1.26.10 fixes GO-2026-6809 (CVE-2026-111943). That release makes
`crypto/x509` distrust compromised intermediate certificates. govulncheck
reports that Atmos calls the affected package.

Dependabot reported vulnerable website transitive dependencies:
`dompurify` 3.4.16, `launch-editor` 2.14.1, and `follow-redirects` 1.16.0.

## Changes

- Set `playwrightNodeVersion` to 24.22.1. Checksums still come from that release's
  `SHASUMS256.txt` at download time.
- Set `website/.nvmrc` to 24.22.1, the Active LTS line. This matches the `node24`
  action runtimes.
- Set the `go.mod` Go version to 1.26.10.
- Re-pin the Windows exception-reserve CI workaround from `go1.26.9` to `go1.26.10`.
  The upstream issue golang/go#81238 remains open. The only changes between the
  two releases are the `crypto/x509` distrust list and restored `moddeps` tests.
  `src/runtime/stack.go` is byte-identical, so the verified SHA256 and the
  one-expression replacement stay unchanged.
- Raise the website `pnpm.overrides` for `dompurify` to ^3.4.17, `launch-editor`
  to ^2.14.2, and `follow-redirects` to ^1.16.1.

## Validation

- The `pkg/auth/providers/aws` tests pass under Go 1.26.10.
- `go build ./...` passes with `GOTOOLCHAIN=go1.26.10`.
- The eight Windows exception-reserve safeguard tests pass.
- The `go1.26.9` and `go1.26.10` copies of `src/runtime/stack.go` from
  go.googlesource.com share SHA256 `0225942a…71d3d`.
- The 24.22.1 `SHASUMS256.txt` and the darwin-arm64, linux-x64, and win-x64
  archives on nodejs.org all return HTTP 200.
- `pnpm run build` for the website passes under Node 24.22.1, both before and
  after the override changes. Both builds report the same 129 pre-existing
  broken-anchor warnings.
- No Windows host was available locally. The Windows CI jobs on this PR exercise
  the overlay on hosted runners.

## Follow-ups

- Dependabot alerts #312 and #313 (`tinypool`) need a 1.x to 2.x major bump, which
  the repository's Dependabot policy blocks.
- Alert #311 (`http-cache-semantics`) has no patched release.
- Alert #310 (`braces`) is a withdrawn advisory.
- Remove the Windows overlay when a supported Go release fixes golang/go#81238.
