# Research: Pact Consumer Contract Testing for Atmos Pro API

**Feature**: 001-pact-consumer-contracts
**Date**: 2026-06-09

---

## Decision 1: pact-go version and Pact specification level

**Decision**: Use `pact-go/v2` with the V2 Pact specification (not V3/V4 matchers).

**Rationale**: pact-go v2 is the current stable release. V2 spec provides all required
matchers (`Like`, `EachLike`, `Term`) for verifying JSON body shapes and header presence.
V3/V4 matchers add features (message pacts, plugin architecture) not needed for HTTP
consumer tests against a REST API. V2 keeps the dependency surface minimal.

**Alternatives considered**:
- pact-go v1 (deprecated, FFI-less) — rejected: EOL, missing V2 matchers.
- V3/V4 spec — rejected: unnecessary complexity; V2 covers all 8 interactions.

---

## Decision 2: Test file placement and build tag

**Decision**: Single file `pkg/pro/consumer_pact_test.go` with `//go:build pact` tag,
plus `pkg/pro/pact_helpers_test.go` for shared MockProvider lifecycle. Both use
`package pro` (same package as the code under test).

**Rationale**: Co-locating pact tests with the existing unit tests in `pkg/pro/` follows
the existing codebase convention (every `api_client_*.go` has a matching `*_test.go` in
the same package). A separate `contract/` sub-package would require exporting internal
fields. The `//go:build pact` tag guarantees these files are completely invisible to
`go test ./...` and `go build .`.

**Alternatives considered**:
- Separate `pkg/pro/contract/` sub-package — rejected: forces export of internals
  (`AtmosProAPIClient` fields), increasing coupling.
- Black-box test package (`package pro_test`) — rejected: `AtmosProAPIClient` needs
  direct field access to inject the mock server URL without going through constructors
  that resolve env vars.

---

## Decision 3: Mock server strategy for the GitHub OIDC endpoint

**Decision**: The GitHub OIDC token retrieval interaction (`GET ACTIONS_ID_TOKEN_REQUEST_URL`)
is mocked using a **second** pact mock provider configured with TLS (`NewV2Pact` with
`MockHTTPProviderConfig` + a custom `Host`/`Port`). The test sets `oidcHTTPClientOverride`
(the package-level test injection point already in `api_client.go`) to use the mock
server's TLS client.

**Rationale**: The existing codebase already exports `oidcHTTPClientOverride` specifically
for test injection. The pact mock server can serve TLS (matching the production
`https`-only validation in `buildOIDCRequestURL`). No new injection points needed.

**Alternatives considered**:
- Skip the OIDC GET interaction — rejected: spec requires all 8 interactions.
- Use `httptest.NewTLSServer` directly — rejected: defeats the purpose of pact (no
  contract generated). Using pact's TLS mock generates the pact file correctly.

---

## Decision 4: Pact file output location

**Decision**: `pacts/` directory at the repository root (pact-go default: `./pacts`).

**Rationale**: pact-go resolves the output directory relative to the test binary's working
directory. When `go test` runs from the repo root, `./pacts` maps to the repo root's
`pacts/` directory. This matches pact ecosystem conventions and makes the generated file
easy to find.

**Alternatives considered**:
- `pkg/pro/pacts/` — rejected: pact-go resolves paths relative to test binary, not the
  source file; output would land in a temp build dir during `go test`.
- Configurable via env var — rejected: over-engineering for a local-only workflow.

---

## Decision 5: pact-go native library installation in development

**Decision**: Native libraries are installed once per machine via `pact-go -l DEBUG install`.
The README documents this as a one-time setup step. No `Makefile` target is required
for this feature.

**Rationale**: The native lib is a pre-built shared library (~15 MB) downloaded from
the pact-go GitHub releases. It only needs to be installed once per developer machine.
Adding a `Makefile` target is a follow-up if the team wants to automate dev setup.

**Alternatives considered**:
- Embed native libs in the repo — rejected: large binary, licence implications.
- Docker-based test runner — rejected: adds complexity; spec says local-only.

---

## Decision 6: Pact consumer and provider names

**Decision**:
- Consumer: `"atmos"`
- Provider: `"AtmosPro"`

**Rationale**: These names appear in the generated pact file name (`atmos-AtmosPro.json`)
and in every interaction record. Using the canonical project names makes the contract
self-documenting and consistent with any future provider verification setup.

---

## Decision 7: Matcher strategy for request/response bodies

**Decision**: Use `matchers.Like()` for all optional/dynamic fields (IDs, timestamps,
SHA values). Use `matchers.Term()` for fields with known format constraints (e.g., ISO
timestamps, hex SHAs). Use exact literal values only for enum/constant fields (HTTP
methods, required string constants like `"key"`).

**Rationale**: `Like()` validates type and structure without pinning exact runtime values,
making contracts stable across different test runs. Pinning exact values for dynamic
fields (e.g., UUIDs, generated SHAs) would make tests brittle. `Term()` (regex) validates
format without requiring exact values for fields like `expiresAt`.

---

## Resolved NEEDS CLARIFICATION Items

All NEEDS CLARIFICATION markers from the spec were resolved during the clarification
session (2026-06-09). No open items remain:

| Item | Resolution |
|------|-----------|
| Endpoint scope (5 vs 8) | All 8 endpoints |
| Pact Broker publishing | Local/repo artifacts only |
| CI job requirement | Local-only; no CI job |
