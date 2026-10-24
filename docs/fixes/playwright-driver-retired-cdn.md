# Playwright Driver Download Fails: Retired CDN

**Date:** 2026-07-03

## Summary

`TestPlaywrightDriverDownload_Integration` (Acceptance Tests) failed with 404s, and real
`aws/saml` Browser-driver authentication could no longer download the Playwright driver on
fresh machines. playwright-go v0.5700.1 downloads its driver zip exclusively from the retired
`*.azureedge.net` CDN hosts, and the matching `playwright-1.57.0-*.zip` build has been purged
from the replacement CDN (`cdn.playwright.dev`).

Upgrading was not possible:

- `playwright-community/playwright-go@v0.6000.0` still uses the dead azureedge hosts, and
  saml2aws v2.36.19 (latest) does not compile against it (`BrowserContext.StorageState`
  signature change).
- The project moved to `github.com/mxschmitt/playwright-go` at v0.6100.0 (new module path);
  saml2aws still imports the old path, and a `replace` fails on the module's self-imports.

## Fix

Pre-seed playwright-go's driver directory from official registries before any Playwright code
runs (`pkg/auth/providers/aws/saml_driver_preseed.go`), mirroring what playwright-go v0.6100.0
does natively. The driver zip was only ever a bundle of:

- the platform-independent `playwright-core` npm package (from `registry.npmjs.org`, verified
  against the registry's published sha512 integrity), extracted to `<driverDir>/package/`; and
- a Node.js binary (from `nodejs.org/dist`, verified against `SHASUMS256.txt`), placed at
  `<driverDir>/node[.exe]`. Skipped when `PLAYWRIGHT_NODEJS_PATH` points at a preinstalled
  Node.js (also the escape hatch for platforms without prebuilt nodejs.org binaries).

With the directory seeded, playwright-go's `isUpToDateDriver()` passes and its dead download
path never executes — for Atmos's own `playwright.Install` call and for saml2aws's internal
one. Browser (Chromium) downloads were never affected: playwright-core 1.57's node-side
installer already uses `cdn.playwright.dev`.

The seeding runs:

- in `samlProvider.Authenticate` for the Browser driver (best-effort, warn on failure —
  an already-installed driver keeps working offline), and
- at the top of the integration test, which validates the full seed → install → detect chain
  (verified locally end-to-end: driver seed + Chromium download + detection, ~19s).

## Test Coverage Added

`pkg/auth/providers/aws/saml_driver_preseed_test.go` — offline unit tests against local
`httptest` registries: already-seeded short-circuit (proven offline via unreachable hosts),
full seed happy path (tarball + node archive extraction), npm integrity mismatch fails closed,
node checksum mismatch fails closed, archive path-escape rejection, and platform suffix
mapping.
