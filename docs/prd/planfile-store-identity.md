# Per-Store Identity for Planfile Storage

**Version:** 1.0
**Last Updated:** 2026-10-16
**Status:** Implemented (PR #3401, closes #3400)

---

## Executive Summary

Atmos can store a Terraform planfile after `plan` and verify it before `deploy`, uploading it to a backend
configured under `components.terraform.planfiles.stores`. Until now, that store always authenticated with the
component's active identity - the same identity the Terraform run used. In a single-cloud project that is
correct, but it breaks the common multi-cloud case: a component that authenticates with a non-AWS identity
cannot produce AWS credentials for an `aws/s3` planfile store, so the upload fails.

This PRD introduces an optional `identity` field on a planfile store. When set, the store authenticates with
that named [Atmos Auth identity](/cli/configuration/auth), independent of the component's identity. When
omitted, the store inherits the component's identity exactly as before, so existing projects are unchanged.
This mirrors the `identity` field already supported by general [Atmos stores](/cli/configuration/stores).

---

## Problem Statement

### Current Behavior

When Atmos resolves the planfile store from configuration, it unconditionally propagates the component's
active identity (`info.Identity`) to the store. The store-creation path reads only `type` and `options` from
the store spec; there is no per-store identity.

For an identity-aware backend (currently `aws/s3`), a non-empty identity defers client initialization until an
auth resolver is injected, then resolves cloud credentials for that identity. For a non-AWS identity the
resolver authenticates the identity successfully but produces no AWS auth context, so initialization fails.

### User Impact

A typical multi-cloud CI setup authenticates a component with, for example, an `azure/subscription` identity
while storing planfiles in an `aws/s3` bucket the platform team already runs. The plan succeeds, but the
planfile upload fails:

```text
failed to upload planfile: failed to load AWS configuration:
failed to resolve AWS auth context for identity "azure-plan":
auth context not available for identity
```

The same mismatch would occur in reverse (an AWS component with a planfile store in another cloud) once
non-AWS planfile backends exist; today only the `aws/s3` identity-aware backend ships, so the reverse case is
a future scenario rather than a current failure. Either way, there was no way to authenticate the store
separately from the component, so the plan-then-deploy workflow could not cross a credential boundary.

### Root Cause

The planfile-store factory always attached the component identity, and the identity-aware backend had no
signal to use a different one. The underlying auth plumbing was already capable of carrying a per-store
identity - the store options already had `Identity` and `Resolver` fields, and the shared resolver already
supported AWS, Azure, and GCP - but the configuration surface and the wiring to select a store-level identity
did not exist.

---

## Goals

- Let a planfile store authenticate with an explicit Atmos Auth identity, independent of the component's
  identity.
- Preserve existing behavior when no store identity is set (inherit the component identity).
- Support any identity that can authenticate the selected backend, not just `ambient` - for the `aws/s3`
  backend, any AWS-capable identity kind or chain (`aws/assume-role`, `aws/permission-set`, …) must be
  resolved through the full Atmos Auth chain, not merely passed through from the runner environment.
- Stay consistent with the `identity` field already supported by general Atmos stores.

## Non-Goals

- New planfile backends. The feature targets the existing identity-aware `aws/s3` backend. Azure Blob and GCS
  planfile backends do not exist today; the shared resolver is already multi-cloud, so they can adopt the same
  field when implemented, but shipping them is out of scope here.
- Full Go-template support in planfile store config. Not required for this feature.

---

## Proposed Solution

### Configuration

Add an optional `identity` field to a planfile store:

```yaml
auth:
  identities:
    azure-plan:                 # the component's default identity (non-AWS)
      kind: azure/subscription
      default: true
      # ...
    aws-backend:                # AWS identity used only by the planfile store
      kind: aws/assume-role     # or ambient, aws/permission-set, etc.
      # ...

components:
  terraform:
    planfiles:
      default: s3
      stores:
        s3:
          type: aws/s3
          identity: aws-backend # store authenticates with this identity
          options:
            bucket: my-terraform-planfiles
            region: us-east-1
```

The component keeps using its own identity (`azure-plan`) for Terraform operations, while the S3 planfile
store uses `aws-backend` for upload and download.

### Implementation

1. Add `Identity string` to the planfile store spec in the configuration schema (with the `identity` YAML/JSON key), and regenerate the embedded atmos.yaml JSON Schema.
2. In the planfile-store factory, validate that a configured store identity names an entry in `auth.identities`; reject an unknown identity at store creation with an error naming the store and the identity, so a typo fails immediately rather than during a later upload.
3. Select the effective identity: the store-level identity takes precedence over the component's active identity; when the store identity is empty, fall back to the component identity.
4. Attach both the identity name **and** the auth resolver for the chosen identity.

The last point is load-bearing. The artifact registry only injects the resolver into an identity-aware
backend when the resolver is set, and such backends fall back to the default credential chain when no resolver
is present. Setting the identity name alone would therefore silently ignore a non-`ambient` identity - it
would "work" for ambient credentials but not for `aws/assume-role`, contradicting the goal above. The resolver
is attached whenever an identity is in scope, including when the component itself declares no identity but the
store does.

---

## Design Details

### Identity precedence

| Store `identity` | Component identity | Identity the store uses |
| ---------------- | ------------------ | ----------------------- |
| set              | any                | store identity          |
| empty            | set                | component identity (prior behavior) |
| empty            | empty              | none (default credential chain) |

### Resolver behavior

The resolver authenticates the identity name passed to it and returns cloud-specific credentials. For the
`aws/s3` backend it resolves an AWS auth context; the same resolver also supports Azure and GCP for future
backends. When the auth manager is unavailable or of an unexpected type, the identity name is recorded but no
resolver is attached, and the backend falls back to the default credential chain - a safe degradation that
avoids a panic.

### Edge cases

- Component with no identity but a store identity: the store identity and its resolver are attached (the naive
  "set the name only" approach would have dropped this to the default chain).
- `nil` command info: no-op, no panic.
- Non-identity-aware backends (`local/dir`, `github/artifacts`): the identity is carried in options but only
  identity-aware backends consume it, so setting it is harmless.

---

## Backward Compatibility and Editions

The `identity` field is new and defaults to empty. An empty value preserves the exact prior behavior
(inherit the component identity), so no existing project changes behavior on upgrade. Per the editions
decision tree, a brand-new key whose default does not change what Atmos already did needs no journal entry;
the default-configuration snapshot is unaffected because there are no default planfile stores.

---

## Testing

- Unit tests for identity selection: store-over-component precedence; store-only identity (component has
  none); fall back to component identity when the store identity is empty; resolver attachment; and nil-safety.
- A store-factory test that builds an `aws/s3` store with a store-level identity and a non-AWS component
  identity, confirming the identity-aware path is taken without a live cloud call (a non-empty identity defers
  client initialization).
- A validation test confirming a store identity that names no `auth.identities` entry is rejected at store
  creation (wrapping the identity-not-in-configuration error).
- An end-to-end example validated in CI: `atmos validate stacks` passes and `atmos describe config` shows the
  store-level identity round-tripping into the loaded planfile store spec.

---

## Future Work

- Azure Blob and GCS planfile backends, which can adopt the same `identity` field through the already
  multi-cloud resolver.

---

## References

- Issue: cloudposse/atmos#3400
- Implementation: cloudposse/atmos#3401
- Related docs: [Planfile Storage](https://atmos.tools/ci/planfile-storage),
  [Stores Configuration](/cli/configuration/stores), [Authentication](/cli/configuration/auth)
