# Fix: CI cache excludes Atmos's own auth session caches by default

**Date:** 2026-07-08

## Problem

`ci.cache` restores and saves a well-known cache root (`~/.cache/atmos` by
default). When `ci.cache.paths` was unset — the common case — the entire
root was archived with no exclusions, including subdirectories where Atmos's
own auth flows persist session credentials: AWS SSO tokens/refresh
tokens/client secret (`aws-sso`), Azure device-code tokens (`azure-device-code`),
Atmos's browser-based AWS webflow refresh token (`aws-webflow`), and
provisioned-identity metadata (`auth`). Nothing stopped that credential
material from being uploaded to the CI provider's cache store (GitHub
Actions cache) alongside the toolchain and other regenerable data.

This was hardened proactively, not in response to a known exploit — access
to a repo's Actions cache is already scoped to that repo, and everything
above is short-lived, rotating session material rather than static secrets.
But the whole-root default meant the risk depended on every `ci.cache.paths`
configuration remembering to avoid these directories by hand.

OIDC-based auth (GitHub/GCP/Azure) was confirmed unaffected — those flows
mint credentials fresh in-memory every run and never write to any cache
directory.

## Fix

The four subdirectories above are now excluded from the CI cache
unconditionally — regardless of `ci.cache.paths` — in both Atmos's own
archiving backend (`pkg/ci/cache/archive.go`) and the `atmos ci cache paths`
passthrough used with the native `actions/cache` action
(`cmd/ci/cache/paths.go`, rendered as `!`-prefixed glob exclusions). A new
`ci.cache.allow_unsafe_auth_cache` config field (default `false`) opts back
in for anyone with a specific, trusted reason to cache their own credential
material. Each owning auth package carries a drift-guard test asserting its
subdir constant stays in sync with the exclusion list.

## Tests

```shell
go test ./pkg/ci/cache/... ./cmd/ci/cache/... \
  ./pkg/auth/providers/aws/... ./pkg/auth/providers/azure/... \
  ./pkg/auth/identities/aws/... ./pkg/auth/provisioning/... -count=1
```
