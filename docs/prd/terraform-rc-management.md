# Terraform RC Management

> Related: [Terraform Registry Cache](terraform-registry-cache.md)

## Overview

Atmos should manage the Terraform/OpenTofu **CLI configuration** (`.terraformrc` / `terraform.tfrc`) on
behalf of the user. Today there is no way to declare CLI-level configuration in `atmos.yaml`; users must
hand-author a `.terraformrc` and export `TF_CLI_CONFIG_FILE` themselves, and Atmos has no hook to inject
CLI-config directives (such as a provider network mirror) before invoking Terraform.

This PRD introduces a `components.terraform.rc` section that Atmos renders, verbatim, into Terraform's native
CLI configuration format and exposes to the subprocess via `TF_CLI_CONFIG_FILE`. It is a small, standalone,
immediately useful feature **and** the foundation that the [Terraform Registry Cache](terraform-registry-cache.md)
builds on — the cache injects its `provider_installation { network_mirror { ... } }` and module `host` blocks
into the same generated CLI config.

## Problem Statement

### Current State

- Atmos already injects some Terraform behavior via environment variables (e.g. `TF_PLUGIN_CACHE_DIR`,
  `TF_IN_AUTOMATION`) in `internal/exec`, but there is **no `TF_CLI_CONFIG_FILE` / `.terraformrc` handling
  anywhere** in the codebase.
- There is no schema for Terraform CLI configuration. Users who need `provider_installation`,
  `credentials`, `host`, or `plugin_cache_dir` directives must manage a `.terraformrc` out of band.
- Without a generated CLI config, Atmos cannot transparently point Terraform at a provider network mirror —
  which the registry cache requires.

### Desired State

- Users declare CLI configuration once, per stack/component, under `components.terraform.rc`.
- Atmos renders it to a temporary CLI config file and sets `TF_CLI_CONFIG_FILE` before launching
  Terraform/OpenTofu, cleaning the file up afterward.
- The section is **a passthrough**, not a new abstraction: future Terraform/OpenTofu CLI-config directives
  work without Atmos schema changes.

## Goals

- **Native passthrough.** Render `components.terraform.rc` directly into Terraform's CLI config format. Atmos
  invents no abstraction over Terraform's own configuration language.
- **Transparent.** Users never manage `.terraformrc` or `TF_CLI_CONFIG_FILE` manually.
- **Forward-compatible.** New Terraform/OpenTofu CLI-config features are usable immediately, without waiting
  for an Atmos release, because the section is near-opaque.
- **Foundation for caching.** Provide a stable injection point the registry cache extends.

## Non-Goals

- Caching of providers or modules — owned by [Terraform Registry Cache](terraform-registry-cache.md).
- A bespoke Atmos enforcement/governance engine (see Governance note below).
- Validating the semantic correctness of arbitrary CLI-config directives — Terraform validates these at run
  time; Atmos only renders them.

## User Experience

```yaml
components:
  terraform:
    rc:
      enabled: true
      # Rendered verbatim into Terraform CLI configuration.
      provider_installation:
        - network_mirror:
            url: "https://terraform-mirror.example.com/"
        - direct:
            exclude:
              - "registry.terraform.io/hashicorp/*"
      host:
        "registry.terraform.io":
          services:
            "modules.v1": "https://modules.example.com/v1/modules/"
```

No Terraform code, provider declarations, or module sources change. Atmos renders the above into a temporary
CLI config and sets `TF_CLI_CONFIG_FILE` for the `terraform`/`tofu` subprocess.

## Configuration Reference

<dl>
  <dt><code>components.terraform.rc.enabled</code></dt>
  <dd>Enable rendering of the CLI configuration. Defaults to <code>false</code>.</dd>

  <dt><code>components.terraform.rc</code> (remaining keys)</dt>
  <dd>A near-opaque map rendered into Terraform's native CLI configuration (HCL). Keys correspond directly to
  Terraform CLI-config blocks (<code>provider_installation</code>, <code>host</code>, <code>credentials</code>,
  <code>plugin_cache_dir</code>, etc.). Because the map is passed through, future directives require no Atmos
  schema change.</dd>
</dl>

## Behavior

1. During `terraform`/`tofu` execution, if `components.terraform.rc.enabled` is set, Atmos renders the section
   to a temporary CLI config file using an **atomic write** (`pkg/filesystem.WriteFileAtomic`).
2. Atmos sets `TF_CLI_CONFIG_FILE=<temp path>` in the subprocess environment.
3. After the run completes, Atmos removes the temporary file.
4. **Single integration point.** All of this is driven from one hook in `ExecuteTerraform`
   (`internal/exec/terraform.go`) that calls a `Setup(...) (env, closer, error)` facade in the new
   `pkg/terraform/rc` package; the logic itself lives entirely in that package.

### Precedence and coexistence

- **User-supplied CLI config.** If the user already exports a CLI-config env var — `TF_CLI_CONFIG_FILE`,
  `TOFU_CLI_CONFIG_FILE`, or the legacy `TERRAFORM_CONFIG` (via OS env or the `env:` section) — Atmos defers
  to it and renders nothing, mirroring how `TF_PLUGIN_CACHE_DIR` is treated today.
- **`TF_PLUGIN_CACHE_DIR`.** RC management is independent of and coexists with the existing plugin-cache
  configuration.

### Terraform and OpenTofu (both env vars, no heuristics)

Atmos does not guess which binary runs. Because Terraform reads `TF_CLI_CONFIG_FILE` and OpenTofu prefers
`TOFU_CLI_CONFIG_FILE` (falling back to `TF_CLI_CONFIG_FILE`), Atmos sets **both** environment variables to
the same generated file, so the configuration is honored whether the resolved binary is `terraform` or
`tofu`. The CLI-config grammar is identical for both tools, so a single rendered file serves either.

## Governance Note

Provider-source trust does **not** require a new Atmos abstraction. Terraform's native
`provider_installation { network_mirror { include/exclude } direct { exclude } }` — expressed through
`components.terraform.rc` — already restricts which providers may load and from where. The registry cache adds
a single proxy egress on top of this, but the policy primitives are Terraform's own.

## Risks / Open Questions

- **OpenTofu parity.** *Resolved:* OpenTofu reads the same CLI-config grammar and prefers
  `TOFU_CLI_CONFIG_FILE`, falling back to `TF_CLI_CONFIG_FILE`. Atmos sets **both** to the generated file
  (no binary heuristic), and treats either — plus the legacy `TERRAFORM_CONFIG` — being user-set as a
  defer signal.
- **Existing user RC.** Decide and document precedence/merge semantics when a user already provides a
  `.terraformrc` or `TF_CLI_CONFIG_FILE`.
- **Rendering fidelity.** Ensure the YAML→HCL rendering of nested blocks (repeated `provider_installation`
  methods, `host` blocks) matches Terraform's expected CLI-config grammar.

## Implementation Notes

- New package `pkg/terraform/rc` (`rc.go`, `generate.go`, `setup.go`) — render + temp-file lifecycle facade.
- One ~5-line hook in `internal/exec/terraform.go` (`ExecuteTerraform`); no changes to overloaded packages
  (`pkg/utils`) and no other `internal/exec` edits.
- Schema: add `RC *TerraformRCConfig` to the `Terraform` struct in `pkg/schema/schema.go` and the `rc` block
  to the JSON schemas under `pkg/datafetcher/schema/`.
- Documentation: `website/docs/cli/commands/terraform/` configuration reference for
  `components.terraform.rc`.
