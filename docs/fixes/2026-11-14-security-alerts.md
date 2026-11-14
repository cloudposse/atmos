# Fix: Dependabot and code-scanning alert remediation (November 2026)

**Date:** 2026-11-14

## Summary

One PR addresses the open Dependabot and code-scanning alerts that have a patched version or a
real code defect. Alerts with no patched version, alerts that are false positives, and alerts that
need a behavior change to a published image are listed below with reasoning; no alert state was
changed.

## Context

The snapshot on 2026-11-14 had 14 Dependabot alerts and 20 code-scanning alerts. This follows
[2026-10-25-security-alert-triage.md](2026-10-25-security-alert-triage.md), which deferred the nine
`go/path-injection` alerts pending a containment design.

## Changes

- **Path injection (CodeQL #6273–#6281).** `pkg/http/proxy` derives cache keys from request URL
  paths. `FileStore` now validates every key (`ValidateKey`: non-empty, relative, no `.`/`..`
  segments, no backslash or NUL) and checks containment against the absolute cache root for
  objects, sidecars, temp files and locks. The server answers `400` for an invalid key before any
  upstream fetch. A rejected key's lock never runs the guarded function.
- **Go toolchain.** Go 1.26.10 → 1.26.11 (GO-2026-6859, `net/http`). The version-pinned Windows
  exception-reserve workaround moves with it; `runtime/stack.go` and `stack_test.go` hash
  identically in 1.26.10 and 1.26.11, so the pinned SHA-256 is unchanged.
- **Go modules.** otel (`otel`, `sdk`, `trace`, `metric`, `sdk/metric`) 1.46.0 → 1.47.0
  (GHSA-q7gw-24fr-5p69); `brotli` 1.2.1 → 1.2.7 (GO-2026-6888); `klauspost/compress` 1.19.2 →
  1.20.2 (GO-2026-6865); `x/crypto` 0.58.0 → 0.59.0 (GO-2026-5932). `x/mod`, `x/sync`, `x/sys`,
  `x/term`, `x/text` and `x/tools` follow by minimum-version selection. `NOTICE` regenerated.
- **Website.** pnpm overrides: `follow-redirects` ^1.16.2, `joi` ^17.13.9, `minimatch@^9` →
  ^11.0.1 (resolves 11.0.5), `tinypool@^1` → ^2.1.2 (resolves 2.2.0). The minimatch and tinypool
  overrides are forced major upgrades: Docusaurus 3.10.2 still depends on `tinypool ^1.0.2`, and
  every minimatch release from 7.0.0 up to 11.0.0 is affected.
- **cdk-terraform example.** npm overrides `follow-redirects` 1.16.2 and `minimatch` 11.0.1.
- **Dockerfiles.** `.oss-scanner/Dockerfile` (`golang:1.26-bookworm`, Go 1.26.11) and the
  stack-lifecycle helm example (`nginx:1.27`, which is 1.27.5) are pinned by index digest.
  Dependabot docker entries keep both current.

## Validation

- `go build ./...`, `go test -race ./pkg/http/proxy/` (new tests prove traversal is rejected on
  `Stat`, `Open`, `Commit` and every `Lock` method, that nothing is written outside the root, that
  the proxy returns 400 without fetching upstream, and that normal keys and a relative root still
  work), and `go test -short` for `pkg/telemetry`, `pkg/auth/providers/aws`, `pkg/oci`,
  `pkg/http`, `pkg/terraform/registry` and `pkg/store`.
- `atmos lint --changed` reports zero issues. `go tool mage notice:generate` succeeds.
- The Windows workaround's unit tests pass with the new version.
- cdk-terraform: `npm ci` from the new lock plus `npm run synth` generates the Demo stack.
- Website: `pnpm install --frozen-lockfile` succeeds under the normal 7-day release-age policy, and
  `pnpm run build` exits 0 (119 prebuild tests pass; the build reports broken-anchor warnings on
  existing pages that this change does not touch).

## Follow-ups

Not fixed here; the remaining alerts are for the repository owner to dismiss or track:

- **No patched version:** `go-jose/v3` ≤ 3.0.5 (#341, #344, #345) is reached only through
  `saml2aws` → `playwright-community/playwright-go`; it can leave the module graph only when
  `saml2aws` stops depending on it. `deepmerge` 4.3.1 (#339, #343) comes from `@cdktn/provider-schema` in the
  example and from `@svgr/plugin-svgo` and `react-player` on the website.
- **Fix not applicable:** minimatch in `examples/cdk-cloudformation` (#353) is bundled inside
  `aws-cdk-lib`; npm overrides do not replace bundled dependencies, and the latest `aws-cdk-lib`
  (2.277.0) still bundles 10.2.6.
- **False positives:** weak hashing (#8217, #8581) hashes cache and state identities built from
  paths, step names and mtimes, not passwords; open redirect (#8255) is a loopback-only OIDC test
  fixture that already restricts `redirect_uri` to `http://127.0.0.1` without userinfo.
- **Needs a design decision:** Trivy DS-0002 on the root `Dockerfile` (#5365). Adding `USER`
  would change the published image's behavior for GitHub Actions workspaces and Docker socket
  access. Scorecard #5414 is an aggregate of the findings above.
