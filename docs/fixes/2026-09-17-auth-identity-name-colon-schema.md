# Fix: Manifest schema accepts auth/secret provider and identity names containing `:`

**Date:** 2026-09-17

## Summary

Relaxed the stack-manifest JSON Schema so `auth.identities`, `auth.providers`, and `secrets.providers`
keys may contain a colon (`:`). Namespaced identity conventions such as `example/prod:terraform_applier`
are accepted by the `atmos.yaml` (config) schema and by the runtime identity model, but stack-manifest
validation rejected them before identity resolution, making an otherwise valid global identity unusable
as a component-level default. Fixes cloudposse/atmos#3185.

`secrets.providers` shared the identical inconsistency (config schema accepts any key; manifest schema
rejected `:`; provider names are opaque map keys at runtime), so it was relaxed in the same pass for
consistency.

## Context

The manifest schema defined `auth_identities`, `auth_providers`, and `secret_providers` with
`patternProperties: { "^[a-zA-Z0-9/_-]+$": ... }` and `additionalProperties: false`. The character
class omitted `:`, so any namespaced key produced:

```text
components.terraform.vpc.auth.identities: additionalProperties
'example/prod:terraform_applier' not allowed
```

This was purely a schema over-restriction:

- The **config schema** (`pkg/datafetcher/schema/atmos/config/1.0.json`) validates root-level `auth`
  with plain `additionalProperties` and no key pattern, so `:` was already accepted there.
- The **runtime** treats identity/provider names as opaque map keys. `manager.resolveIdentityName`
  (`pkg/auth/manager.go`) does exact + case-insensitive map lookup only; there is no character
  validation of identity names anywhere in `pkg/auth/`. (The stricter `^[a-z0-9_-]+$` pattern in
  `pkg/auth/realm/realm.go` applies to *realm* names, which is a separate, deliberately stricter
  concept.)
- No selection code path splits identity names on `:`, so widening the pattern introduces no
  delimiter collision.

Two other restrictive key patterns were reviewed and intentionally left unchanged: component instance
names (`^[/a-zA-Z0-9-_{}. ]+$`) are unrelated to this issue, and Terraform `required_providers` local
names (`^[a-zA-Z0-9-_]+$`) follow Terraform's own rule (`[a-z0-9]`, no colons).

## Changes

- `pkg/datafetcher/schema/atmos/manifest/1.0.json` — changed the `auth_identities`, `auth_providers`,
  and `secret_providers` `patternProperties` key pattern from `^[a-zA-Z0-9/_-]+$` to `^[a-zA-Z0-9/_:-]+$`.
- `tests/fixtures/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json` — applied the same pattern
  relaxation (the hand-synced test copy). While there, restored parity for the `auth_identity` and
  `auth_provider` definitions, which had drifted from the embedded schema (the fixture still required
  `kind` and was missing the `required`/`tags` fields).
- `pkg/datafetcher/schema_auth_identity_name_test.go` (new) — schema-level regression tests asserting
  colon-containing identity, provider, and secret-provider names validate against every on-disk schema
  copy (embedded/website/fixture) at the component level, plus a negative test that a name with a space
  is still rejected (so `additionalProperties: false` keeps catching genuinely malformed keys).
- `pkg/auth/manager_case_test.go` — extended the identity-resolution table with namespaced
  (colon-containing) identity names for both exact and case-insensitive lookup, proving the runtime
  contract behind the schema relaxation.
- `pkg/secrets/providers/sops/construct_test.go` — added a namespaced (colon-containing) SOPS provider
  name case, proving secret-provider names resolve as opaque map keys at runtime.

## Validation

```bash
# Reproduction tests failed before the schema change, pass after.
go test ./pkg/datafetcher/ -run 'TestManifestSchema_AuthIdentityNames|TestManifestSchema_SecretProviderNames' -v

# Full affected packages.
go test ./pkg/datafetcher/ ./pkg/validator/ ./pkg/auth/ ./pkg/secrets/...
go test ./internal/exec/ -run 'Schema|Validate|Manifest'
```

All listed tests pass. The embedded schema loaded by `loadEmbeddedSchemaBytes`
(`atmos://schema/atmos/manifest/1.0`) is the same schema enforced at runtime, so the schema-level
tests exercise the real validation path.

## Follow-ups

None.
