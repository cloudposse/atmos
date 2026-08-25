# PRD: FIPS 140-3 Mode for Atmos Builds

## Executive Summary

Every atmos binary (dev builds, official releases, CI test binaries, and the sharded
acceptance-test harness) is now built with `GOFIPS140=latest`, which links Go's native
FIPS 140-3 validated cryptographic module and defaults the resulting binary to
FIPS-enforcing mode (`GODEBUG=fips140=on`) at runtime, with no extra flag required by
the user. This hardens the TLS/crypto surface atmos uses to talk to cloud providers,
git servers, and artifact registries for operators in regulated environments. It does
**not**, by itself, make Atmos's secrets subsystem FIPS 140-3 compliant — see
[Known Limitation](#known-limitation-age--nacl-secrets-encryption) below.

## Problem Statement

Operators in regulated environments (US federal, financial services, healthcare) are
often required to run only FIPS 140-validated cryptography. Go 1.24 added a native,
in-tree FIPS 140-3 validated crypto module (`crypto/internal/fips140/...`), selectable
at build time via `GOFIPS140` and enforced at runtime via `GODEBUG=fips140`. Atmos
(Go 1.26.4) previously built with no FIPS wiring at all, so no atmos binary could make
a FIPS 140-3 claim.

## Design Goals

1. **On by default, everywhere a binary is built** — dev builds, official release
   binaries (goreleaser), and CI/test binaries (`atmos test`, sharded acceptance tests)
   all get `GOFIPS140=latest` so there's one behavior to reason about, not an opt-in
   variant most users never touch.
2. **Zero required runtime flags** — `GOFIPS140=latest` at build time makes the
   compiled binary default to `fips140=on` automatically; a user does not need to set
   `GODEBUG=fips140=on` themselves.
3. **Escape hatch preserved** — every build site sets `GOFIPS140` with a
   `${GOFIPS140:-latest}`-style default (or the YAML/Go equivalent), matching the
   existing `CGO_ENABLED` pattern, so it can still be overridden if ever needed.
4. **Honest about scope** — this is stdlib TLS/crypto hardening, not a certified FIPS
   140-3 compliance claim for the whole binary. The gap below must stay documented, not
   quietly forgotten.

## What's Covered

Go's FIPS 140-3 module intercepts the stdlib surface: `crypto/tls` (restricts to
FIPS-approved protocol versions, cipher suites, signature algorithms, key exchange),
`crypto/rsa`, `crypto/ecdsa`/`crypto/ecdh` (curve restrictions), `crypto/rand`
(NIST SP 800-90A DRBG), `crypto/aes`, `crypto/sha256`/`sha3`/`sha512`, HMAC, HKDF,
PBKDF2. Atmos's own use of this surface was already FIPS-compatible before this change
(TLS configs already pin `MinVersion: tls.VersionTLS12` with no custom cipher suites,
all production EC key generation uses the FIPS-approved P-256 curve, all real
`crypto/rand` usage is correct) — this change makes that compatibility enforced by
default rather than incidental.

## Known Limitation: age / NaCl Secrets Encryption

Two security-relevant code paths use cryptography that sits **outside** Go's FIPS
module boundary. `GODEBUG=fips140=on` does not intercept, restrict, or flag these —
they keep working exactly as before, just without FIPS validation:

- **`filippo.io/age`** (X25519 + ChaCha20-Poly1305) — used by `atmos secret keygen`
  and the SOPS/age secrets backend (`pkg/secrets/providers/sops/sops_keygen.go`).
- **`golang.org/x/crypto/nacl/box`** (Curve25519 + XSalsa20-Poly1305) — used to seal
  secret values pushed to the GitHub Actions Secrets API
  (`pkg/store/providers/github_actions_client.go`), per GitHub's own sealed-box API
  contract. This one can't be swapped for a FIPS primitive without breaking the
  integration — it's GitHub's requirement, not an Atmos design choice.

MD5 and SHA-1 usage elsewhere in the codebase (S3 SSE-C header, AWS SSO cache key,
Windows certificate thumbprints, toolchain checksum verification) is likewise outside
the module boundary — but unlike the two paths above, none of that usage is for
authentication or encryption, so it carries no compliance implication either way.

Closing this gap for the secrets subsystem is a larger, separate effort (a SOPS-backend
redesign for age, and no viable replacement for the GitHub sealed-box requirement) and
is out of scope for this change.

## Where It's Wired In

Every distinct Go-toolchain build invocation in the repo sets `GOFIPS140`:

| Location | Purpose |
|---|---|
| `magefiles/build.go` (`Build.Binary`) | `atmos build` — dev builds and the CI `build` job |
| `.goreleaser.yml` / `.goreleaser.draft.yml` | Official release binaries |
| `.github/workflows/screengrabs.yaml` | The one CI spot with a raw `go build`, bypassing `atmos build` |
| `.atmos.d/test.yaml` (`go_auto_env` anchor, plus `short-cover`/`race`) | `atmos test short/coverage/race/magefiles` |
| `internal/ci/acceptance/command.go` (`goCommandEnvironment()`) | Sharded acceptance-test binaries built via `mage acceptance:*` |

`go.mod` has no `godebug`-directive equivalent for `fips140` (confirmed against
go.dev/doc/godebug), so there's nothing to set there.

## Verification

Build and short-test-suite runs under `GOFIPS140=latest` were compared against
unmodified baseline runs before this change landed: clean compile, a FIPS-built binary
successfully completing a real outbound HTTPS call (the `atmos version` update check
against GitHub), and an identical set of pre-existing local-environment test timeouts
in both the FIPS and non-FIPS runs (confirming this change introduces no new test
regressions). `go version -m` on a binary built via `atmos build` confirms
`GOFIPS140=latest` and `DefaultGODEBUG=fips140=on` are baked in.
