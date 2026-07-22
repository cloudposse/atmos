# PRD: Default Auth Behavior

## Problem

Component stack configuration can use `auth.identities.<name>.default` to choose
or demote the identity used for that component. Atmos currently deep-merges the
entire component identity map into the active global auth configuration. As a
result, an entry containing only `default: false` for an identity absent from
the active profile becomes a new identity with no `kind`. Auth-manager
initialization then fails because every merged identity must have a `kind`.

This makes a harmless default-selection marker behave as an incomplete identity
declaration. Shared component mixins consequently force every profile to add
dummy identities solely to make the merged configuration valid.

## Goals

- Let a component demote an identity it does not define without requiring a
  placeholder identity in every active profile.
- Preserve field-by-field merging for real component identity definitions and
  for identities supplied by the active global auth configuration.
- Fail early and clearly when a component selects an identity that does not
  exist.

## Non-Goals

- Change the schema or syntax of `auth.identities`.
- Change identity-selection precedence, profile loading, or stack import
  semantics.
- Treat entries with identity configuration beyond `default: false` as markers.

## Desired Behavior

Before merging component auth into global auth, Atmos must classify each entry
in the component's resolved `auth.identities` map.

| Component entry | Global identity exists | Result |
| --- | --- | --- |
| Only `default: false` | Yes | Merge it; the global identity remains complete and is not default. |
| Only `default: false` | No | Drop it. Do not create an identity and do not return an error. |
| Only `default: true` | Yes | Merge it and make the existing identity the component default. |
| Only `default: true` | No | Return `ErrInvalidIdentityConfig` explaining that the component default references an undefined identity. |
| Contains `kind` or any identity field besides `default` | Either | Treat it as an identity declaration and retain the existing deep-merge and validation behavior. |

An empty identity map (`name: {}`), `required: true` without a `kind`, or any
other incomplete declaration is **not** a marker and must retain the existing
validation failure. The special treatment applies only when the entry has
exactly one field, `default`, whose value is `false` or `true`.

### Example

Given an active profile with one real identity:

```yaml
auth:
  identities:
    deploy:
      kind: aws/assume-role
      default: true
```

and a shared component mixin:

```yaml
auth:
  identities:
    deploy:
      default: true
    audit:
      default: false
```

the resulting component auth configuration contains only the complete
`deploy` identity. `audit` is ignored because it is a false-only marker for an
identity not defined by the active profile.

If the mixin instead sets `audit.default: true`, Atmos must fail before
authentication with an error that names `audit` and tells the user to define it
in global/profile auth or select an existing identity.

## Implementation

Apply this normalization in `MergeComponentAuthConfig`, after the component
configuration has been fully resolved from its stack imports and before global
defaults are cleared or the maps are deep-merged.

- Work on a copy of the component auth section so callers' component
  configuration is not mutated.
- Determine whether an identity exists from the copied global auth config using
  the same name representation currently used by component auth merging; this
  change must not alter existing identity-name or case-handling behavior.
- Remove false-only entries that have no matching global identity.
- Detect true-only entries that have no matching global identity and return an
  `ErrInvalidIdentityConfig` error from the merge helper. Include the identity
  name and an actionable hint to define it in active global/profile auth.
- Run the existing `componentAuthHasDefault` check only after this validation,
  so an invalid undefined `default: true` marker cannot clear an otherwise
  valid global default.
- Deep-merge all remaining entries exactly as today. Existing identity and
  factory validation remains responsible for invalid real declarations.

No public API or configuration-schema changes are required.

## Compatibility

Existing fully defined identities, including intentionally configured
`aws/ambient` identities, retain their current behavior. Placeholder identities
used only to support absent `default: false` entries become unnecessary but
remain valid configuration. An undefined `default: true` currently leads to a
later unusable or incomplete auth configuration; it will instead receive an
earlier, targeted validation error.

## Acceptance Tests

- A false-only component entry for an undefined identity is absent from the
  merged auth config and auth-manager initialization succeeds.
- A false-only component entry for an existing identity updates its default
  state without losing that identity's fields.
- A true-only component entry for an existing identity selects it as default.
- A true-only component entry for an undefined identity returns
  `ErrInvalidIdentityConfig`, names the identity, and does not mutate or clear
  the input global defaults.
- Entries with any fields besides `default` continue through normal deep merge
  and existing identity validation.
- Existing tests continue to verify that merge inputs are not mutated.

Regression coverage asserts that a false-only undefined marker is removed and
initialization succeeds, rather than preserving the prior failure.
