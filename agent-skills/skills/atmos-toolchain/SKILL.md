---
name: atmos-toolchain
description: "Toolchain management: declarative dependencies, automatic installs, install/exec/search/env commands, Aqua registry integration, version pinning, package verification"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - references/commands-reference.md
---

# Atmos Toolchain

## Purpose

The Atmos toolchain manages CLI tool installation and versioning natively, using the Aqua package registry
ecosystem. It replaces external version managers (asdf, mise, aqua CLI) with a built-in system that
integrates directly with atmos.yaml configuration.

## Default Rule

Declare tool requirements where they are used, then run the owning Atmos command. Do not add
`atmos toolchain install` before component, workflow, custom-command, or hook execution when the
tool can be declared with `dependencies.tools`. Atmos installs missing declared tools and injects
them into `PATH` for that execution context.

Use explicit `atmos toolchain install` only for shell bootstrap, cache warming, ad-hoc
troubleshooting, or job-level tools that are not owned by a component, workflow, custom command, or hook.

Agents repeatedly get two things wrong here: putting an operation-specific tool version only in
`.tool-versions`, and wrapping `atmos` itself in `atmos toolchain exec`. Both are covered below.

❌ **Wrong** — a component needs a pinned Terraform version, declared only in `.tool-versions`:
```text
# .tool-versions
terraform 1.10.3
```
This is a repo-wide default. Nothing ties it to the `vpc` component, so a different component (or a
different environment's `.tool-versions`) can silently drift the version this component actually needs.

✅ **Right** — pin it on the component that requires it:
```yaml
components:
  terraform:
    vpc:
      dependencies:
        tools:
          terraform: "1.10.3"
```
`.tool-versions` still sets the repo-wide developer-shell default; `dependencies.tools` overrides it for
this component's execution context, and travels with the component if it's vendored or reused elsewhere.

## Core Concepts

### .tool-versions File

The `.tool-versions` file (asdf-compatible format) declares which tools and versions a project needs:

```text
terraform 1.9.8
opentofu 1.10.3
kubectl 1.28.0
helm 3.13.0
jq 1.7.1
```

- One tool per line, format: `toolname version [version2 version3...]`
- Multiple versions per tool are supported; first version is the default
- File location: project root (default), overridable via `toolchain.versions_file` in atmos.yaml,
  `--tool-versions`, or `ATMOS_TOOL_VERSIONS`

Use `.tool-versions` for repo-wide developer shell defaults and general tools that every developer,
agent, or interactive shell should see. Atmos can auto-install these defaults for custom commands and
shell integration, but `.tool-versions` is not the preferred home for operation-specific requirements.

Use `dependencies.tools` when the tool is part of a stack/component/workflow/command contract. Those
dependencies are installed and injected into that execution context and can override `.tool-versions`
defaults. Do not rely on `.tool-versions` alone when CI or a stack requires a specific Terraform,
OpenTofu, scanner, or generator version.

### Where to Declare a Tool

| Need | Preferred declaration |
|------|-----------------------|
| Component needs Terraform/OpenTofu, scanner, or generator | Stack/component `dependencies.tools` |
| Workflow step needs a CLI | Workflow `dependencies.tools` |
| Custom command invokes a CLI such as `bat`, `tree`, `jq`, or `checkov` | Command `dependencies.tools` |
| Hook invokes a scanner or custom binary | Component `dependencies.tools` for the hooked component |
| One-off local shell, manual verification, or cache warm | `atmos toolchain install` plus `atmos toolchain env` |

### Tool Installation Path

By default, tools are installed in the XDG data directory for Atmos, such as
`~/.local/share/atmos/toolchain` on Linux/macOS. If the XDG data directory cannot be created, Atmos
falls back to `.tools` in the current project.

Installed tools use this layout under the selected install path:

```text
<install-path>/bin/{os}/{tool}/{version}/{binary}
```

Override the install path with `toolchain.install_path` in `atmos.yaml` or `ATMOS_TOOLCHAIN_PATH`
when tools must be stored in a project-local, shared, or cached directory.

### Registries

Atmos supports three registry types for discovering and downloading tools:

**Aqua Registry** -- The primary source, providing 1,000+ tools:
```yaml
registries:
  - name: aqua
    type: aqua
    source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
    priority: 10
```

**Inline (Atmos) Registry** -- Define tools directly in atmos.yaml:
```yaml
registries:
  - name: custom
    type: atmos
    priority: 150
    tools:
      owner/repo:
        type: github_release
        url: "asset_{{.Version}}_{{.OS}}_{{.Arch}}"
        format: tar.gz
```

**File-Based Registry** -- Local or remote Aqua-format files:
```yaml
registries:
  - name: corporate
    type: aqua
    source: file://./custom-registry.yaml
    priority: 100
```

**Priority System:** Higher numbers are checked first. First match wins.
Typical ordering: inline (150) > corporate (100) > public aqua (10).

### Aliases

Map short names to fully qualified tool identifiers:

```yaml
toolchain:
  aliases:
    terraform: hashicorp/terraform
    tf: hashicorp/terraform
    kubectl: kubernetes-sigs/kubectl
```

### Package Verification

Atmos verifies downloaded packages before extraction when Aqua or inline registry metadata provides checksums,
signatures, or attestations. Defaults are non-breaking: verify when metadata exists, allow packages without metadata.

```yaml
toolchain:
  verification:
    checksums: when_available   # when_available | required | disabled
    signatures: when_available  # when_available | required | disabled
    verifier_install: auto      # auto | path_only
```

Supported methods: checksums (`sha256`, `sha512`, `sha1`, `md5`), `cosign`, `slsa-verifier`,
GitHub artifact attestations via `gh`, and `minisign`.

## Configuration in atmos.yaml

```yaml
toolchain:
  install_path: .tools              # Optional override; default is XDG data dir
  versions_file: .tool-versions     # Path to version file

  aliases:
    terraform: hashicorp/terraform
    tf: hashicorp/terraform

  registries:
    # Inline tools (highest priority)
    - name: my-tools
      type: atmos
      priority: 150
      tools:
        jqlang/jq:
          type: github_release
          url: "jq-{{.OS}}-{{.Arch}}"

    # Aqua registry (fallback)
    - name: aqua
      type: aqua
      source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
      priority: 10

  verification:
    checksums: when_available
    signatures: when_available
    verifier_install: auto
```

## Key Commands

### Manual Installation and Cleanup

```bash
atmos toolchain install                    # Bootstrap/cache warm tools from .tool-versions
atmos toolchain install terraform@1.9.8    # Ad-hoc install for shell use or troubleshooting
atmos toolchain uninstall terraform@1.9.8  # Remove installed tool
atmos toolchain clean                      # Remove all installed tools and cache
```

Prefer `dependencies.tools` for normal Atmos execution. Component, workflow, custom-command, and hook
runs install missing declared tools automatically.

### Version Management

```bash
atmos toolchain add terraform              # Add tool to .tool-versions (latest)
atmos toolchain add terraform@1.9.8        # Add with specific version
atmos toolchain remove terraform           # Remove from .tool-versions
atmos toolchain set terraform 1.9.8        # Set default version
atmos toolchain get terraform              # Get version from .tool-versions
```

### Discovery

```bash
atmos toolchain search terraform           # Search across registries
atmos toolchain info hashicorp/terraform   # Display tool configuration
atmos toolchain list                       # Show installed tools
atmos toolchain which terraform            # Show full path to binary
atmos toolchain du                         # Show disk usage
```

### Execution

```bash
atmos toolchain exec terraform@1.9.8 -- plan    # Run specific version
atmos toolchain env --format=bash                # Export PATH for shell
atmos toolchain path                             # Print PATH entries
```

**Anti-pattern: never run `atmos toolchain exec -- atmos ...`.** Every `atmos` command is already
toolchain-aware — it resolves and injects declared tool paths for its own execution automatically (see
"Default Rule" above). Wrapping `atmos` in `atmos toolchain exec` is redundant at best and a sign the
tool declaration is missing at worst. `toolchain exec` exists only to run a **third-party** binary
(`terraform`, `kubectl`, a scanner CLI) directly, pinned to a specific managed version, outside of an
atmos-orchestrated command — never to invoke `atmos` itself.

If a workflow/custom-command/hook step needs to confirm a tool is present on `PATH` **without**
installing it, use the `require` step type (alias `assert`) instead of `dependencies.tools` (which
auto-installs) or a hand-rolled `command -v` shell check — see `atmos-workflows` for the `tools:`/`files:`/
`dirs:` config.

### Registry Management

```bash
atmos toolchain registry list              # List all registries
atmos toolchain registry list aqua         # List tools in specific registry
atmos toolchain registry search jq         # Search across registries
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `ATMOS_GITHUB_TOKEN` / `GITHUB_TOKEN` | GitHub token for higher API rate limits |
| `ATMOS_TOOL_VERSIONS` | Override .tool-versions file path |
| `ATMOS_TOOLCHAIN_PATH` | Override tool installation directory |
| `ATMOS_TOOLCHAIN_ENV_FORMAT` | Default format for `env` command |

## Precedence Order

1. CLI flags (highest)
2. Environment variables
3. atmos.yaml configuration
4. XDG data directory
5. `.tools` fallback

## Shell Integration

If you only ever run `atmos <subcommand>`, you do not need shell integration — Atmos resolves and
injects declared tool paths for its own execution automatically. Shell integration is only for getting
the raw tool binary directly into an interactive shell or a non-Atmos script (e.g. running `terraform`
by hand outside of `atmos terraform ...`).

Add to `~/.bashrc` or `~/.zshrc` for automatic PATH setup:

```bash
eval "$(atmos toolchain env --format=bash)"
```

Other shell formats: `--format=fish`, `--format=powershell`, `--format=github` (for CI).

For shells where evaluation is not desirable, prepend the generated toolchain path directly:

```bash
export PATH="$(atmos toolchain path):$PATH"
```

Set `ATMOS_TOOLCHAIN_PATH` when the installed-tools directory must be shared, cached, or moved outside
the repo checkout:

```bash
export ATMOS_TOOLCHAIN_PATH="$HOME/.cache/atmos/tools"
eval "$(atmos toolchain env --format=bash)"
```

In GitHub Actions, use the GitHub format only when later job-level steps need direct shell access to
toolchain binaries. When `$GITHUB_PATH` is available, Atmos appends toolchain paths to it automatically:

```yaml
- name: Add toolchain paths for later job-level scripts
  run: atmos toolchain env --format=github
```

## Tool Dependencies in Atmos Config

Declare tool dependencies close to the thing that needs them. Atmos installs missing tools from the
toolchain registry and injects them into the command environment. Use these dependencies for tools
required by stacks, components, workflows, and custom commands; use `.tool-versions` for general
project tools.

```yaml
# Top-level defaults
dependencies:
  tools:
    aws-cli: "^2.0.0"

# Component-type defaults
terraform:
  dependencies:
    tools:
      terraform: "~> 1.10.0"
      tflint: "^0.54.0"

# Component-specific tools
components:
  terraform:
    vpc:
      dependencies:
        tools:
          terraform: "1.10.3"
          checkov: "latest"

# Workflow tools
workflows:
  validate:
    dependencies:
      tools:
        tflint: "^0.54.0"

# Custom command tools
commands:
  - name: scan
    dependencies:
      tools:
        checkov: "3.0.0"
```

Use exact versions for reproducible CI, SemVer ranges for managed upgrade windows, and `latest`
only for non-production workflows where drift is acceptable.

In Atmos CI, prefer `dependencies.tools` over GitHub setup actions such as
`hashicorp/setup-terraform` or `opentofu/setup-opentofu`. Setup actions install a runner-level binary,
while Atmos tool dependencies travel with the stack, component, workflow, or command that requires
the tool and are injected into that execution context. If an agent is about to add
`atmos toolchain install <tool>` to CI, first check whether the tool belongs on the component,
workflow, or custom command that invokes it.

## Custom Registries in atmos.yaml

Add your own tools under `toolchain.registries` when the public Aqua registry does not define them.
Use a high-priority `type: atmos` registry for inline definitions, and keep the public Aqua registry
as a fallback:

```yaml
toolchain:
  aliases:
    policyctl: company/policyctl

  registries:
    - name: company
      type: atmos
      priority: 150
      tools:
        company/policyctl:
          type: github_release
          url: "policyctl_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz"
          format: tar.gz
          binary_name: policyctl

    - name: aqua
      type: aqua
      source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
      priority: 10
```

For shared registry files, use `type: aqua` with a local or remote `source` and a higher priority than
the public registry:

```yaml
toolchain:
  registries:
    - name: company-registry
      type: aqua
      source: file://./toolchain/company-registry.yaml
      priority: 100
```

## Template Variables in Registries

Aqua and inline registries support Go templates in asset URLs:

| Variable | Description |
|----------|-------------|
| `{{.Version}}` | Full version string |
| `{{trimV .Version}}` | Version without 'v' prefix |
| `{{.OS}}` | Operating system (linux, darwin, windows) |
| `{{.Arch}}` | Architecture (amd64, arm64) |

## Common Patterns

### Project Setup

```yaml
# atmos.yaml
toolchain:
  aliases:
    terraform: hashicorp/terraform
    kubectl: kubernetes-sigs/kubectl
  registries:
    - name: aqua
      type: aqua
      source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
      priority: 10
```

```text
# .tool-versions
hashicorp/terraform 1.9.8
kubernetes-sigs/kubectl 1.28.0
helmfile/helmfile 0.168.0
```

Tools declared on components, workflows, and commands are installed when those operations run. Use
`atmos toolchain list` to inspect local installs; use `atmos toolchain install` only to pre-warm a
developer shell or CI cache.

### CI/CD Integration

```yaml
# GitHub Actions: normal Atmos operation, no preinstall step needed
- run: atmos terraform plan vpc -s prod

# Optional: expose already-managed tool paths to later non-Atmos shell steps
- run: atmos toolchain env --format=github
```

### Custom Tool Registry

```yaml
toolchain:
  registries:
    - name: internal
      type: atmos
      priority: 150
      tools:
        company/internal-tool:
          type: github_release
          url: "internal-tool_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz"
          format: tar.gz
    - name: aqua
      type: aqua
      source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
      priority: 10
```

## Unsupported Aqua Features

These Aqua features are intentionally not supported to keep Atmos focused:

- `github_content`, `github_archive`, `go_build`, `cargo` package types
- `version_filter`, `version_expr` version manipulation
- `import` (use multiple registries instead)
- `command_aliases` (use `toolchain.aliases` in atmos.yaml)
