# PRD: Provider-Config Contributor

> Related: [Terraform RC Management](terraform-rc-management.md), [Emulators](emulators.md)

## Summary

Add a generic **provider-config contributor** mechanism to Atmos provider generation: a registry that
lets assist processes dynamically inject provider-configuration fragments that are deep-merged into
the generated `providers_override.tf.json` before Terraform/OpenTofu runs.

This is the direct analog of [Terraform RC Management](terraform-rc-management.md): there, Atmos
assembles a `.terraformrc` from contributions and the registry cache contributes a
`provider_installation { network_mirror { … } }` block (`pkg/terraform/cache/cache.go`). Provider
generation has no equivalent hook today — `pkg/generator/providers/generator.go` wraps the stack's
`providers:` section verbatim. This PRD adds the missing seam.

The first consumer is the [Emulators](emulators.md) feature: an emulator-bound component needs
Terraform provider **behavior** flags (`skip_requesting_account_id`, `s3_use_path_style`,
`skip_credentials_validation`, `skip_metadata_api_check`) plus `endpoints {}` and dummy creds — and
these are provider *arguments*, not environment variables, so env injection alone cannot set them.

## Goals

- A pluggable contributor registry so assist processes can inject provider-config fragments.
- Deep-merge contributions into `ProvidersSection`, with **explicit stack `providers:` winning**.
- A clean, cycle-free seam (contributors implement a narrow interface; the generator owns it).
- First consumer: emulator. Designed to be reused (auth, registry cache, future features).

## Non-Goals

- Replacing the user-facing `providers:` stack section — explicit config always wins.
- A schema for arbitrary provider config — contributors return opaque fragments deep-merged in.
- Re-implementing the providers generator output format — contributions feed the existing one.

## Design

### §A The contributor interface

```go
// ProviderContributor injects a provider-config fragment for a component being generated.
// The returned fragment is keyed by provider name (e.g. "aws") and deep-merged into the
// component's ProvidersSection. Returning (nil, nil) contributes nothing.
type ProviderContributor interface {
    Name() string
    Contribute(ctx context.Context, genCtx *generator.GeneratorContext) (map[string]any, error)
}
```

Contributors register at init (mirroring `pkg/store/registry.go` / the RC contributions model). The
providers generator (`pkg/generator/providers/generator.go`) collects all contributions in
deterministic lexical `Name()` order, **deep-merges them under** the explicit
`genCtx.ProvidersSection` (explicit wins), and emits the result as `{"provider": merged}` →
`providers_override.tf.json`.

`GeneratorContext` (`pkg/generator/generator.go`) already carries `AtmosConfig`, `StackInfo`,
`Component`, `Stack`, and `ProvidersSection`, so a contributor has everything it needs to decide
whether and what to contribute.

### §B Merge semantics

- Deep-merge per provider key; explicit stack `providers:` values **override** contributed values.
- Multiple contributors merge in deterministic lexical `Name()` order; conflicts resolve to the
  earliest contributor in that order, and explicit config always wins over all contributors.
- No contribution when no contributor applies → output is identical to today (back-compatible).

### §C First consumer — the emulator contributor

When `genCtx`'s component is bound to an emulator (its identity is `<cloud>/emulator`, or it carries
an `!emulator`/emulator reference), the emulator contributor resolves the emulator profile
(`pkg/emulator.Manager.Resolve`) and contributes `Profile.Provider`, e.g. for AWS:

```json
{ "aws": {
    "endpoints": [{ "s3": "http://localhost:54321", "sqs": "http://localhost:54321", "dynamodb": "http://localhost:54321" }],
    "skip_requesting_account_id": true,
    "skip_credentials_validation": true,
    "skip_metadata_api_check": true,
    "s3_use_path_style": true,
    "access_key": "test",
    "secret_key": "test"
} }
```

The contributor lives in `pkg/emulator` and implements the generator's narrow interface (no import
cycle: `pkg/generator` defines the interface; `pkg/emulator` implements and registers it). The
GCP/Azure provider fragments are emitted where those providers support endpoint overrides; otherwise
those targets rely on env injection (their Terraform provider support is limited).

### §D Relationship to env injection

Provider generation and env injection are complementary, not duplicative:

- **Env vars** (the auth identity path) drive SDKs, the shell, and non-Terraform tools, and the
  Terraform provider endpoint/creds where env-honored.
- **The provider contributor** drives Terraform provider *behavior* flags that env cannot set.

Both read the same emulator profile, resolved once.

## Public Interface

- `ProviderContributor` interface + a registration function in `pkg/generator`.
- No user-facing config — contributors are internal; users continue to author the `providers:`
  section, which always wins.

## Implementation Notes

- Add the interface + registry to `pkg/generator`; wire the deep-merge into
  `pkg/generator/providers/generator.go` (`ShouldGenerate` must account for contributions, not just a
  non-empty `ProvidersSection`).
- Reuse the deep-merge utility used elsewhere in stack processing; ensure explicit-wins precedence.
- Emulator contributor in `pkg/emulator`, registered at init; gated on the component being
  emulator-bound.
- Precedent to follow: `pkg/terraform/rc` (RC assembly from contributions) and
  `pkg/terraform/cache/cache.go` (the cache contributing `provider_installation`).

## Test Plan

- **Merge precedence:** explicit stack `providers:` overrides a contributed fragment; contributions
  merge into an empty `ProvidersSection`; multiple contributors merge deterministically.
- **No-op:** with no applicable contributor, output is byte-identical to today.
- **Emulator contributor:** emulator-bound component → `endpoints` + skip-flags + creds present;
  non-emulator component → no contribution; not-running emulator → actionable error.
- **Generator integration:** `ShouldGenerate` true when only a contribution exists; emitted JSON is
  valid `{"provider": …}`.
- Follow repo testing mandates (table-driven; isolation tests both directions for merges).

## Related

- [Terraform RC Management](terraform-rc-management.md) — the contribution pattern this mirrors.
- [Emulators](emulators.md) — the first consumer.
