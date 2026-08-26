# PRD: FIPS 140-3 Mode for Atmos Builds

## Executive Summary

Every atmos binary (dev builds, official releases, CI test binaries, and the sharded
acceptance-test harness) is now built with `GOFIPS140=latest`, which links Go's native
FIPS 140-3 crypto module and defaults the resulting binary to FIPS-enforcing mode
(`GODEBUG=fips140=on`) at runtime, with no extra flag required by the user. This hardens
the TLS/crypto surface atmos uses to talk to cloud providers, git servers, and artifact
registries for operators in regulated environments. This establishes **FIPS 140-3 mode**
for that surface; it is **not**, by itself, a CMVP compliance certification for the atmos
binary, and it does **not** make Atmos's secrets subsystem FIPS 140-3 compliant — see
[Known Limitation](#known-limitation-age--nacl-secrets-encryption) below.

## Problem Statement

Operators in regulated environments (US federal, financial services, healthcare) are
often required to run only FIPS 140-validated cryptography. Go 1.24 added a native,
in-tree FIPS 140-3 crypto module (`crypto/internal/fips140/...`), selectable at build
time via `GOFIPS140` and enforced at runtime via `GODEBUG=fips140`. Atmos (Go 1.26.4)
previously built with no FIPS wiring at all, so no atmos binary could make a FIPS 140-3
mode claim.

## Design Goals

1. **On by default, everywhere a binary is built** — dev builds, official release binaries (goreleaser), and CI/test binaries (`atmos test`, sharded acceptance tests) all get `GOFIPS140=latest` so there's one behavior to reason about, not an opt-in variant most users never touch.
2. **Zero required runtime flags** — `GOFIPS140=latest` at build time makes the compiled binary default to `fips140=on` automatically; a user does not need to set `GODEBUG=fips140=on` themselves.
3. **Escape hatch preserved** — every build site sets `GOFIPS140` with a `${GOFIPS140:-latest}`-style default (or the YAML/Go equivalent), matching the existing `CGO_ENABLED` pattern, so it can still be overridden if ever needed.
4. **Honest about scope** — this is stdlib TLS/crypto hardening, not a certified FIPS 140-3 compliance claim for the whole binary. The gap below must stay documented, not quietly forgotten.

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

`GOFIPS140=latest` tracks whatever FIPS module snapshot ships with the Go toolchain used
for the build, not a version pinned to a specific CMVP validation certificate. That's the
right default for "always get Go's newest FIPS module," but it also means the claim this
change makes is **"this binary runs in FIPS 140-3 mode"**, not **"this binary is a
CMVP-certified FIPS 140-3 module."** A stricter compliance program that needs the latter
would pin `GOFIPS140` to a specific validated module version (e.g. `GOFIPS140=v1.0.0`)
instead of `latest`; that's a deliberate tradeoff for a future change, not something this
PR does.

## Platform Exclusion: windows/386

`crypto/internal/fips140.Supported()` rejects `windows/386` outright — that platform lacks
a CPU jitter entropy source good enough for FIPS mode — and `check.init()` panics at
process startup once `GODEBUG=fips140=on` is in effect (the default this change bakes in).
Building that combination with FIPS enabled would therefore produce a binary that crashes
immediately on launch rather than one that's merely non-FIPS. `magefiles/build.go`
(`Build.Binary`) rejects a resolved `windows/386` target whenever `GOFIPS140` will be
enabled, and `.goreleaser.yml` excludes `windows/386` from the release matrix entirely.

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
| `.github/workflows/website-deploy-prod.yml` / `website-preview-build.yml` | Throwaway `go run .` invocations that generate the website's schema JSON files (not a distributed artifact); set for consistency with every other build here, not because these processes make TLS calls |

Go does support a `godebug (fips140=on)` block in `go.mod` (see go.dev/doc/godebug), but
that only sets the *runtime default* for `GODEBUG=fips140`, not the build-time FIPS module
selection — it's the equivalent of what `GOFIPS140=latest` already gives every atmos binary
automatically (a FIPS-enforcing default with no user-set `GODEBUG` needed), and it still
requires `GOFIPS140` at build time to actually link the FIPS module in the first place. Since
every build site here already sets `GOFIPS140`, adding a `go.mod` `godebug` block would be
redundant, not a substitute.

## Verification

Build and short-test-suite runs under `GOFIPS140=latest` were compared against
unmodified baseline runs before this change landed: clean compile, a FIPS-built binary
successfully completing a real outbound HTTPS call (the `atmos version` update check
against GitHub), and an identical set of pre-existing local-environment test timeouts
in both the FIPS and non-FIPS runs (confirming this change introduces no new test
regressions). `go version -m` on a binary built via `atmos build` confirms
`GOFIPS140=latest` and `DefaultGODEBUG=fips140=on` are baked in.

## User-Facing Verification

`atmos version --format=json` (and `--format=yaml`) exposes a `fips` boolean field
(`internal/exec/version.go`, `isFIPSBuild`), backed by `crypto/fips140.Enabled()` —
Go's own runtime check for whether FIPS 140-3 mode is active in *this* process right
now. `GOFIPS140` at build time (linking the FIPS module in the first place) sets the
default, but `GODEBUG=fips140` can override that default at invocation in either
direction: `=on` activates FIPS mode even on a binary built without `GOFIPS140`, and
`=off` deactivates it on a `GOFIPS140`-built binary. `fips` reflects whichever wins for
the running process, not just the build-time default. This is intentionally a
live-status check rather than a static build-setting read: `go version -m` (or
`runtime/debug.ReadBuildInfo().Settings`) can only confirm the binary *includes* the
FIPS module and defaults to FIPS mode — it cannot tell you whether a runtime
`GODEBUG=fips140` override is in effect. `fips` gives users that runtime-accurate
answer without needing a Go toolchain installed.
