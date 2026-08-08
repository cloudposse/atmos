# Migrating from Aqua CLI

[Aqua CLI](https://aquaproj.github.io/) is a tool-version manager. It reads a config file named
`aqua.yaml`.

Read this distinction before you start: Aqua CLI is not the same thing as the Aqua registry.
Atmos does not run the Aqua CLI tool. Atmos does reuse the Aqua **registry** data format --
the same package listings that Aqua CLI reads. Atmos reimplemented the registry parser itself,
because the Aqua project states that its own Go modules are for internal use, not for external
tools to depend on. This means the registry ecosystem carries over. The `aqua.yaml` file, the
checksum lockfile, and the policy file do not carry over as-is. See "Common Gotchas" below.

This reference is a scenario-keyed decision guide. It covers only tool-version management: the
migration of `aqua.yaml` to the Atmos toolchain. It does not cover the migration of Terraform code
itself. For that task, use the other references in this skill.

## Identifying the User's Shape

Read the `aqua.yaml` file, or ask the user to show it to you.

| Shape | Recipe |
|---|---|
| `aqua.yaml` uses the standard registry only | [Shape A](#shape-a-standard-registry-only) |
| `aqua.yaml` uses a custom registry, a checksum block, or a policy file | [Shape B](#shape-b-custom-registry-checksums-or-policy) |

## Shape A: Standard Registry Only

**Before:**
```text
project/
├── aqua.yaml
└── main.tf
```
```yaml
# aqua.yaml
registries:
  - type: standard
    ref: v4.0.0

packages:
  - name: hashicorp/terraform@v1.10.3
  - name: jqlang/jq@jq-1.7.1
  - name: kubernetes/kubectl@v1.28.0
```

**Recipe:**

1. Add the `toolchain:` block to `atmos.yaml`. A `registries:` entry with `type: standard` maps
    to a `toolchain.registries[]` entry with `type: aqua`. Carry over the `ref:` pin from
    `aqua.yaml` -- an unpinned registry can change under you the same way an unpinned `ref: main`
    would.
    ```yaml
    toolchain:
      versions_file: .tool-versions
      registries:
        - name: aqua
          type: aqua
          source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
          ref: v4.0.0
          priority: 10
    ```
2. Convert each `packages:` entry to a `.tool-versions` line. The common `name: owner/repo@version`
    form splits into a short tool name and a version:
    ```text
    # .tool-versions
    terraform 1.10.3
    jq 1.7.1
    kubectl 1.28.0
    ```
    If the short name in `.tool-versions` does not match the Aqua `owner/repo` name, add an alias:
    ```yaml
    toolchain:
      aliases:
        terraform: hashicorp/terraform
        jq: jqlang/jq
        kubectl: kubernetes/kubectl
    ```
3. Check the migration. Run `atmos toolchain install`, then `atmos toolchain list`, then
    `atmos toolchain which <tool>` for each tool. Confirm each version matches what Aqua CLI
    reported.

## Shape B: Custom Registry, Checksums, or Policy

This shape builds on Shape A. Do the Shape A recipe first, then add the steps below.

**Before:**
```yaml
# aqua.yaml
registries:
  - type: standard
    ref: v4.0.0
  - type: github_content
    repo_owner: myorg
    repo_name: my-registry
    ref: main
    path: registry.yaml

packages:
  - name: hashicorp/terraform@v1.10.3
  - name: myorg/internal-tool@v2.0.0

checksum:
  enabled: true
  require_checksum: true
```
```yaml
# aqua-policy.yaml
registries:
  - type: standard
policies:
  - myorg/my-registry/registry.yaml
```

**Recipe:**

1. Add the custom registry as a second `toolchain.registries[]` entry. Use `source` for the
    registry location, and `ref` to pin a version. Atmos accepts `ref` only when `source` is a
    `github.com` URL. Pin `ref` to a tag or commit SHA, not a branch -- a branch like `main` is
    mutable and can change what gets installed without any change to `atmos.yaml`.
    ```yaml
    toolchain:
      registries:
        - name: internal
          type: aqua
          source: https://github.com/myorg/my-registry/tree/main
          ref: v1.2.0
          priority: 100
        - name: aqua
          type: aqua
          source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
          ref: v4.0.0
          priority: 10
    ```
2. Replace the `checksum:` block with `toolchain.verification`:
    ```yaml
    toolchain:
      verification:
        checksums: required
        signatures: when_available
        verifier_install: auto
    ```
3. Drop `aqua-policy.yaml`. Atmos has no trust or policy step. See "Common Gotchas" below.
4. Do not migrate `aqua-checksums.json`. Atmos manages its own lockfile, `toolchain.lock.yaml`.
    Whether it writes that lockfile automatically depends on the project's edition: unpinned or
    newer projects get it by default, with no configuration needed. A project whose `atmos.yaml`
    pins an edition dated before `2026-08-05` keeps the old opt-in behavior and must set
    `toolchain.use_lock_file: true` explicitly to get the same automatic lockfile.
5. Check the migration the same way as Shape A.

## CLI Command Mapping

| Aqua CLI command | Atmos equivalent | Notes |
|---|---|---|
| `aqua init` | Create the `toolchain:` block in `atmos.yaml` by hand | There is no init command. |
| `aqua install` | `atmos toolchain install` | Installs all tools from `.tool-versions` |
| `aqua g` / `aqua generate` | `atmos toolchain search <name>`, then add a `.tool-versions` line | Atmos has no interactive picker. |
| `aqua cp` | No direct equivalent | `toolchain.install_path` gives a predictable local directory, for caching, not for vendoring binaries into another image. |
| `aqua exec -- <command>` | `atmos toolchain exec <tool>@<version> -- <command>` | Runs one command with a pinned tool version |
| `aqua which <tool>` | `atmos toolchain which <tool>` | Shows the path to the tool binary |
| `aqua list` | `atmos toolchain list` | Lists installed tools |
| `aqua update-checksum` | No action needed | Atmos manages `toolchain.lock.yaml` on its own. |
| `aqua policy allow [file]` | No equivalent | Atmos has no trust or policy step. |
| `aqua info` | `atmos toolchain info <tool>` | Shows registry metadata for one tool, not full environment diagnostics |

## Common Gotchas

### The registry format is shared. The CLI tool is not.

Atmos reads Aqua registry package listings. Atmos does not read `aqua.yaml` directly, and Atmos
does not run the Aqua CLI. Translate `aqua.yaml` by hand, using the recipes above. Do not expect
Atmos to parse `aqua.yaml` as config.

### No policy or trust-gating step

Aqua CLI uses `aqua-policy.yaml` and the `AQUA_POLICY_CONFIG` variable to control which configs
and registries a user trusts, because some Aqua package types run arbitrary build steps or
scripts. Atmos supports only the two safest package types, `github_release` and `http`. Atmos
never runs an install script. This removes the need for a trust step. Do not look for a policy
equivalent.

### No layered global config

Aqua CLI supports `AQUA_GLOBAL_CONFIG`, a list of config file paths that apply everywhere, not
just in one project. Atmos has no matching variable. Use the project `.tool-versions` file for
tools every developer needs, or `toolchain.aliases`/`toolchain.registries` in `atmos.yaml` or in
`.atmos.d/`.

### No lazy, shim-based install

Aqua CLI installs a shim for each declared tool, then downloads the real binary the first time
the shim runs. Atmos does not use shims. Atmos installs a tool when a component, workflow, or
command that declares it runs, or when the user runs `atmos toolchain install` directly.

### Unsupported package types

Atmos does not support these Aqua package types: `github_content`, `github_archive`, `go_build`,
`cargo`, `go_install`. If a package in `aqua.yaml` uses one of these types, find another source
for it, or define an inline `type: atmos` registry entry with a `github_release` or `http`
package type instead. See the [atmos-toolchain](../../atmos-toolchain/SKILL.md) skill for the
full list of supported and unsupported registry features.

## What to NOT Do

- Do not point Atmos at `aqua.yaml` directly. Atmos does not read this file. Translate it by
  hand, using the recipes above.
- Do not look for an Atmos equivalent of `aqua-policy.yaml` or `AQUA_POLICY_CONFIG`. Atmos has no
  trust step, by design.
- Do not try to migrate `aqua-checksums.json`. Atmos writes its own lockfile automatically.
- Do not use a naive `grep` on `name:` lines to convert `packages:` entries. This breaks on the
  common single-line `name: owner/repo@version` form. Use the recipe above instead.
- Do not introduce Gomplate datasources for things YAML functions can express. See the Core
  Principles in the [SKILL.md](../SKILL.md).
