# Migrating from mise

[mise](https://mise.jdx.dev/) is a tool-version manager. A mise config file has one of these
names: `mise.toml`, `.mise.toml`, `.mise/config.toml`. mise can also read a `.tool-versions` file.

This reference is a scenario-keyed decision guide. It covers only tool-version management: the
migration of `[tools]`, `[tasks]`, `[env]`, and `[settings]` to the Atmos toolchain. It does not
cover the migration of Terraform code itself. For that task, use the other references in this
skill.

## Identifying the User's Shape

Read the mise config file, or ask the user to show it to you.

| Shape | Recipe |
|---|---|
| Tool versions only: `[tools]`, or a `.tool-versions` file, with no `[tasks]` or `[env]` | [Shape A](#shape-a-tool-versions-only) |
| Tool versions plus tasks or environment variables: `[tasks]`, `[env]` | [Shape B](#shape-b-tool-versions-plus-tasks-and-env) |

## Shape A: Tool Versions Only

**Before:**
```text
project/
├── mise.toml
└── main.tf
```
```toml
# mise.toml
[tools]
terraform = "1.10.3"
jq = "1.7.1"
kubectl = "1.28.0"
```

**Recipe:**

1. Check for a `.tool-versions` file. If mise already reads this file, do not change it. If mise
    does not use this file, create it. Add one line for each tool in `[tools]`.
2. Find each tool in the Atmos toolchain. Run `atmos toolchain search <name>`. If the tool is in
    the Aqua registry, and the mise short name differs from the Aqua `owner/repo` name, add an
    alias in `toolchain.aliases`. If the tool is not in the Aqua registry, add an inline registry
    entry.
3. Add the `toolchain:` block to `atmos.yaml`:
    ```yaml
    toolchain:
      versions_file: .tool-versions
      aliases:
        terraform: hashicorp/terraform
        jq: jqlang/jq
        kubectl: kubernetes/kubectl
      registries:
        - name: aqua
          type: aqua
          source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
          priority: 10
    ```
    ```text
    # .tool-versions
    terraform 1.10.3
    jq 1.7.1
    kubectl 1.28.0
    ```
4. Check the migration. Run `atmos toolchain install`, then `atmos toolchain list`, then
    `atmos toolchain which <tool>` for each tool. Confirm each version matches what mise reported.

## Shape B: Tool Versions Plus Tasks and Env

This shape builds on Shape A. Do the Shape A recipe first, then add the steps below.

**Before:**
```text
project/
├── mise.toml
└── main.tf
```
```toml
# mise.toml
[tools]
terraform = "1.10.3"

[env]
AWS_REGION = "us-east-1"

[env.production]
AWS_PROFILE = "prod"

[tasks]
fmt = "terraform fmt -recursive"

[tasks.deploy]
description = "Deploy the app"
run = "terraform apply -auto-approve"
depends = ["fmt"]

[settings]
experimental = true
```

**Recipe:**

1. Migrate `[env]`. A mise env var maps to a stack `env:` block or a command `env:` block.
    ```yaml
    # atmos.yaml or stacks/_defaults.yaml -- env for the whole project
    env:
      AWS_REGION: us-east-1
    ```
    ```yaml
    # stacks/prod.yaml -- env for one environment
    env:
      AWS_PROFILE: prod
    ```
    A mise profile, for example `[env.production]`, maps to an Atmos stack. Each stack already
    has its own `env:` block. You do not need a separate profile feature. If only one command
    needs an env var, add the var to that command's own `env:` block instead of the global one.

    Precedence order, from lowest to highest: the system environment, then the global `env:` block
    in `atmos.yaml`, then the `env:` block in a stack file (stack root, then component type, then
    component). A command's own `env:` block is different. It is a list of key-value pairs, and it
    applies only when that command runs. It is not part of the order above. See the
    [atmos-settings](../../atmos-settings/SKILL.md) skill for full detail.
2. Migrate `[tasks]`. A simple mise task maps to an Atmos custom command.
    ```yaml
    # atmos.yaml
    commands:
      - name: fmt
        description: Format Terraform code
        steps:
          - terraform fmt -recursive

      - name: deploy
        description: Deploy the app
        steps:
          - atmos fmt
          - terraform apply -auto-approve
    ```
    mise `depends` has no direct equivalent inside one custom command. For a simple, linear
    dependency, add a step that runs the other command, as shown above. For a task graph with
    parallel steps or conditions, use an Atmos workflow instead. A workflow supports `depends_on`
    and `when:`. See the [atmos-workflows](../../atmos-workflows/SKILL.md) skill. Tasks in a
    `mise-tasks/` directory have no direct mapping. Convert each script to a step in a custom
    command.
3. Drop `[settings]`. `[settings].experimental` has no equivalent. Most other `[settings]`
    entries have no equivalent either. See "Common Gotchas" below.
4. Check the migration the same way as Shape A.

## CLI Command Mapping

| mise command | Atmos equivalent | Notes |
|---|---|---|
| `mise install` | `atmos toolchain install` | Installs all tools listed in `.tool-versions` |
| `mise install <tool>@<version>` | `atmos toolchain install <tool>@<version>` | Installs one tool |
| `mise use <tool>@<version>` | `atmos toolchain set <tool> <version>` | Sets the default version in `.tool-versions` |
| `mise ls` / `mise ls --current` | `atmos toolchain list` | Lists installed tools |
| `mise ls-remote <tool>` | `atmos toolchain info <tool>` | Shows available versions |
| `mise current` | `atmos toolchain get [tool]` | Shows the version set in `.tool-versions` |
| `mise which <tool>` | `atmos toolchain which <tool>` | Shows the path to the tool binary |
| `mise uninstall <tool>@<version>` | `atmos toolchain uninstall <tool>@<version>` | Removes one installed version |
| `mise prune` | `atmos toolchain clean` | Atmos removes all installed tools and the cache, not just unused versions. |
| `mise exec <tool>@<version> -- <command>` | `atmos toolchain exec <tool>@<version> -- <command>` | Runs one command with a pinned tool version |
| `mise run <task>` | `atmos <command>` | Runs the migrated custom command |
| `mise env` | `atmos toolchain env` | Prints PATH and env settings for the shell |
| `mise activate` | `eval "$(atmos toolchain env --format=bash)"` in the shell startup file | Atmos has no activate daemon. Other formats: `fish`, `powershell`, `github`. |
| `mise search <tool>` | `atmos toolchain search <tool>` | Searches all registries |
| `mise registry` | `atmos toolchain registry list` or `registry search` | Lists or searches one registry |
| `mise unuse <tool>` | `atmos toolchain remove <tool>` | Removes a tool from `.tool-versions` |

These mise commands have no Atmos equivalent. Do not look for a match. Drop them during the
migration: `mise doctor`, `mise plugins`, `mise trust`/`untrust`, `mise settings`, `mise where`
(use `atmos toolchain which` for the binary path instead), `mise outdated`/`upgrade` (change the
version in `.tool-versions` and run `atmos toolchain install` instead), `mise implode` (closest
match is `atmos toolchain clean`), `mise tasks` (run `atmos --help` to list custom commands
instead).

## Common Gotchas

### No plugin system

mise and asdf use plugins. A plugin is a script that installs a tool. Atmos does not run install
scripts. Atmos gets tools from the Aqua registry, or from an inline registry in `atmos.yaml`. Do
not look for a plugin equivalent. Find the tool in the Aqua registry instead, or add an inline
registry entry for it. See the [atmos-toolchain](../../atmos-toolchain/SKILL.md) skill for the
full registry reference.

### `.tool-versions` format is shared

Atmos and mise read the same asdf-compatible `.tool-versions` format. In most cases, this file
does not need to change. Only the config that wraps it changes: mise finds the file on its own,
but Atmos needs a `toolchain:` block in `atmos.yaml`.

### `[settings]` mostly has no equivalent

Settings such as `experimental`, `idiomatic_version_file_enable_tools`, and `jobs` are specific to
mise. Do not try to find an Atmos setting for each one. Drop them during the migration.

### `atmos toolchain` is an experimental command

Tell the user this before the migration starts. Do not let the user find this out partway
through the work.

### Shims vs. `dependencies.tools`

mise adds tools to the shell PATH with shims. Atmos does not do this by default. Atmos adds a
tool to PATH only for the command, workflow, or component that declares it, through
`dependencies.tools`. For an interactive shell, run `atmos toolchain env` or
`atmos toolchain path`.

## What to NOT Do

- Do not try to run mise install scripts through Atmos. Atmos has no plugin system.
- Do not leave tool versions only in `.tool-versions` when a specific component, workflow, or
  command needs a pinned version. Use `dependencies.tools` for that case. See the
  [atmos-toolchain](../../atmos-toolchain/SKILL.md) skill.
- Do not put `[tasks]` content inside the `toolchain:` block. Custom commands and workflows are
  separate features. See the [atmos-custom-commands](../../atmos-custom-commands/SKILL.md) and
  [atmos-workflows](../../atmos-workflows/SKILL.md) skills.
- Do not put `[env]` content inside the `toolchain:` block. Use stack or command `env:` instead.
  See the [atmos-config](../../atmos-config/SKILL.md) and [atmos-stacks](../../atmos-stacks/SKILL.md)
  skills.
- Do not introduce Gomplate datasources for things YAML functions can express. See the Core
  Principles in the [SKILL.md](../SKILL.md).
