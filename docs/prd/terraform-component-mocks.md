# Terraform Component Mocks

## Problem

Atmos YAML functions such as `!terraform.state` and `!terraform.output` normally require the referenced component to have real Terraform output or remote state. This makes local configuration inspection and planning difficult when a dependency has not been deployed or credentials are unavailable.

## Goals

- Let a Terraform component declare literal output values under `mocks`.
- Resolve those values only when the caller explicitly passes `--use-mocks`.
- Treat mocks as fallbacks by default: use the real value when it exists and the mock only when the referenced component is not provisioned or the output is missing.
- Keep a hermetic `always` mode for runs that must never touch Terraform state, credentials, or a backend.
- Support the existing `!terraform.state` and `!terraform.output` argument and YQ-expression syntax.
- Keep normal commands unchanged and prevent mock values from reaching mutating Terraform operations.

## Non-goals

- Mocking Terraform providers, resources, data sources, stores, secrets, or other Atmos YAML functions.
- Named mock profiles, wildcard matching, consumer-local overrides, or synthetic output generation.
- Evaluating templates or YAML functions inside mock values.

## Configuration and CLI contract

Declare mocks on the Terraform component that normally produces the outputs:

```yaml
components:
  terraform:
    vpc:
      mocks:
        vpc_id: vpc-local
        private_subnet_ids: [subnet-a, subnet-b]
```

Use them explicitly:

```shell
atmos terraform plan app -s dev --use-mocks
atmos describe component app -s dev --use-mocks
```

The map participates in normal component inheritance and deep merging; the most-specific value wins.

`--use-mocks` requires YAML function processing and is supported only by `terraform plan` and `describe component`. Every other `atmos terraform` subcommand (for example apply, deploy, destroy) rejects the flag before stack resolution. `terraform plan --all` and `--affected` pass the flag through to each component. A bare `--use-mocks` takes no value, so `--use-mocks always` leaves `always` as a positional argument; Atmos reports this with an error and a hint, and the mode must be attached with `=`.

### `--use-mocks` values

| Invocation | Effect |
|---|---|
| (absent), empty, `--use-mocks=false` (also `0`, `f`) | Off. Lookups use real state and outputs. |
| `--use-mocks`, `--use-mocks=true` (also `1`, `t`), `ATMOS_USE_MOCKS=true` | On, using `components.terraform.mocks.mode`. |
| `--use-mocks=fallback`, `--use-mocks=always` | On, overriding `components.terraform.mocks.mode` for this run. |

Matching is case-insensitive and any other value is an error. `ATMOS_USE_MOCKS` is read only by `atmos terraform plan`; `atmos describe component` ignores it. If it is exported, every terraform subcommand other than `plan` fails with an error naming the variable.

### `components.terraform.mocks.mode`

```yaml
components:
  terraform:
    mocks:
      mode: fallback   # fallback (default) | always
```

The setting accepts `fallback` or `always` (case-insensitive in `atmos.yaml`) and can also be set with `ATMOS_COMPONENTS_TERRAFORM_MOCKS_MODE`. The effective mode resolves in this order (highest wins): `--use-mocks=<mode>`, the environment variable, `atmos.yaml`, then the edition-aware default. The default is `fallback` for unpinned projects and for projects pinned to a [config edition](editions.md) on or after 2026-10-01. Projects pinned to an earlier edition get `always`, which is the behavior bare `--use-mocks` had before this setting existed. An invalid value in `atmos.yaml` or the environment variable fails at configuration load for every command.

## Resolution and errors

### `fallback` mode

Atmos still runs the real lookup (authentication, backend read, caches) and uses the component's `mocks` only to fill gaps. A gap is recoverable: the referenced component's state is not provisioned, or the requested output is missing. This is the same classification YQ `//` defaults use.

1. The referenced component declares no `mocks`: the lookup behaves exactly as it does without `--use-mocks`.
2. The component declares `mocks`: Atmos loads them with template and YAML-function processing disabled, reads the component's full set of real outputs, and deep-merges the mocks under the real outputs. Every value present in real state wins; a mock fills keys missing from a real map output (real `config = { a = 1 }` with mock `config: { a: 0, b: 2 }` resolves to `{a: 1, b: 2}`). Lists and scalars are never merged element by element: a real list replaces the mock list wholesale. The requested expression, including any YQ `//` default, is evaluated against that merged map. A real value therefore always wins, a declared mock fills a missing output, and a `//` default applies only when neither exists. This holds for indexed and filtered expressions as well as plain output names.
3. Neither a real value, a mock, nor a `//` default exists: the result is the same as without mocks. A component that was never applied (with no matching mock) returns the not-provisioned error from `!terraform.state` and `null` from `!terraform.output`; an output missing from provisioned state resolves to `null` from both without an error. A missing `mocks` map or undeclared output is not itself an error in this mode.
4. Non-recoverable error (credentials, network, backend, initialization failures, or a missing Terraform binary): return the error with an added hint to use `--use-mocks=always` or `components.terraform.mocks.mode: always` for mocks-only resolution. Mocks never hide these.

Known limitation: Terraform does not record outputs whose value is `null` in state, so in fallback mode an output that is `null` in a provisioned component is indistinguishable from a missing output and resolves to the mock. Do not declare a mock for an output that can legitimately be `null`, or use `always`.

Precedence: real value, then mock, then YQ `//` default, then the normal not-provisioned error or `null`.

### `always` mode

Atmos loads the referenced component with template and YAML-function processing disabled, evaluates the requested output expression against its literal `mocks` map, and returns the result. It does not initialize Terraform, authenticate, read a backend, or use the Terraform state/output caches.

The `always` mode is fail-closed: an absent `mocks` map or an undeclared direct output is an error. Atmos does not fall back to real state, because doing so would make an explicit mock invocation non-hermetic. Explicit `null` mock values remain valid. Existing YQ defaults continue to work against the mock map and still rescue a missing mock map or output.

## Migration

The default meaning of bare `--use-mocks` changed:

- **Before:** bare `--use-mocks` always resolved lookups from `mocks` and never read real state, even when the referenced component had been applied.
- **Now:** bare `--use-mocks` uses real state when it is available and uses mocks only for components that are not provisioned or outputs that are missing, unless the project is pinned to an edition before 2026-10-01 or sets `components.terraform.mocks.mode: always`.

Handled through the edition journal as a `KindValue` entry on the brand-new key `components.terraform.mocks.mode` (dated 2026-10-01, `always` to `fallback`), following the `components.terraform.init.mode` precedent. Because the key is new, the journal entry is how an upgrading project keeps the previous fixed behavior: pinning `edition: "2026-09"` (or any earlier edition) restores `always` with no other config changes. Projects that want the hermetic behavior regardless of edition set `mode: always` or pass `--use-mocks=always`. Projects that want the new behavior while still pinned to an earlier edition set `mode: fallback` explicitly.

## Security and rollout

The feature is opt-in and excluded from mutating Terraform commands, so production apply behavior is unchanged. Literal mock values are still configuration data: users must not put secrets in them unless their normal repository controls permit it.

Fallback mode never weakens error handling: credential, network, and backend errors still fail rather than being replaced by mock values, and the plan-only restriction is unchanged.

Roll out with `describe component --use-mocks` and local `terraform plan --use-mocks` first. The provider-free example in `examples/terraform-component-mocks` demonstrates both the fallback flow and `--use-mocks=always`.

## Status and changelog

| Date | Changes |
|------|---------|
| 2026-07 | Initial implementation: `--use-mocks` always resolved from `mocks` (fail-closed). |
| 2026-10-01 | Mocks became fallbacks by default. Added `components.terraform.mocks.mode` (`fallback` / `always`), value-bearing `--use-mocks`, and the edition-gated default. |
