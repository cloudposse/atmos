# Replace tenv with the Atmos Toolchain

This reference explains how to replace [tenv](https://github.com/tofuutils/tenv) with the Atmos
toolchain. For the skill's decision guide, see [../SKILL.md](../SKILL.md). For the full toolchain
feature reference, see [atmos-toolchain](../../atmos-toolchain/SKILL.md).

If the user manages only 1 tool, see [from-tfenv.md](from-tfenv.md) or
[from-tofuenv.md](from-tofuenv.md) instead. The steps are the same. This file covers tenv's
multi-tool scope only.

## Overview

tenv replaces tfenv and tofuenv. It is one binary that manages version pinning for Terraform,
OpenTofu, Terragrunt, and TFLint. Each tool uses its own version file, or a `tenv <tool> use`
command.

The Atmos toolchain treats each tool as an independent tool declaration. To migrate, apply the
same one-line-per-tool process to each tool that tenv currently manages.

## Version Files and Tool Names

| tenv-managed file      | Tool         | `.tool-versions` line     |
|-------------------------|--------------|----------------------------|
| `.terraform-version`    | Terraform    | `terraform <version>`      |
| `.opentofu-version`     | OpenTofu     | `opentofu <version>`       |
| `.terragrunt-version`   | Terragrunt   | `terragrunt <version>`     |
| `.tflint-version`       | TFLint       | `tflint <version>`         |

Convert only the files that exist in the repo. Most projects use 1 or 2 of these files. Few
projects use all 4.

## Before / After

**Before:**
```text
# .terraform-version
1.9.8
# .tflint-version
0.54.0
```
You can also set versions with commands: `tenv terraform use 1.9.8` or `tenv tflint use 0.54.0`.

**After:**
```text
# .tool-versions
terraform 1.9.8
tflint 0.54.0
```
```yaml
# atmos.yaml
toolchain:
  aliases:
    terraform: hashicorp/terraform
    tflint: terraform-linters/tflint
  registries:
    - name: aqua
      type: aqua
      source: https://github.com/aquaproj/aqua-registry/pkgs
      ref: v4.550.0
      priority: 10
```
```bash
atmos toolchain install
```

## Steps

1. **Identify which version files exist** in the repo. See the mapping table above. If a file is
  missing but a version is pinned another way, run `tenv <tool> detect` to find the current
  resolved version.
2. **Add one `.tool-versions` line for each tool.** Use the mapping table.
3. **Add the `toolchain:` block** to `atmos.yaml` if the repo does not already have one. See
  Before/After above. If OpenTofu is the primary binary for `atmos terraform` commands, add
  `components.terraform.command: tofu` too.
4. **Verify the setup.** Run `atmos toolchain install`, then run `atmos toolchain list`. Confirm
  each tool resolves to the expected version.
5. **Promote versions to `dependencies.tools`** on the specific component or workflow when a
  version is part of that unit's contract. Use this instead of a repo-wide default. See the
  "Default Rule" in [atmos-toolchain](../../atmos-toolchain/SKILL.md).
6. **Remove the version files and uninstall tenv last.** Do this only after the team verifies
  that `atmos` commands resolve every tool correctly without tenv.

## Command Mapping

| tenv command | Atmos toolchain equivalent |
|---|---|
| `tenv terraform install 1.9.8` | `atmos toolchain install terraform@1.9.8` |
| `tenv terraform use 1.9.8` | `atmos toolchain set terraform 1.9.8` |
| `tenv tofu install 1.10.3` | `atmos toolchain install opentofu@1.10.3` |
| `tenv tofu use 1.10.3` | `atmos toolchain set opentofu 1.10.3` |
| `tenv terragrunt install 0.67.0` | `atmos toolchain install terragrunt@0.67.0` |
| `tenv tflint install 0.54.0` | `atmos toolchain install tflint@0.54.0` |
| `tenv terraform list` (installed) | `atmos toolchain list` |
| `tenv terraform list --remote` (available) | `atmos toolchain search terraform` |
| `tenv terraform detect` | `atmos toolchain get terraform` |
| `tenv terraform uninstall 1.9.8` | `atmos toolchain uninstall terraform@1.9.8` |

For the last 4 rows, replace `terraform` with `tofu`, `terragrunt`, or `tflint` as needed.

## Shell Integration

tenv works through shims. Its installer adds `~/.tenv/bin` to `PATH`, typically with
`export PATH="$HOME/.tenv/bin:$PATH"` in `~/.bashrc` or `~/.zshrc`.

Each shim (`terraform`, `tofu`, `terragrunt`, `tflint`) reads its matching version file again on
every invocation. Plain commands such as `terraform` or `tofu` always resolve correctly in any
shell. This setup needs no per-project configuration.

**The Atmos toolchain does not do this by default.** Atmos resolves tools declared in
`.tool-versions` (the project-wide default) or `dependencies.tools` (a scoped override) and injects
them into `PATH` only for the duration of an `atmos <subcommand>` invocation. If you run one of
these tools plain in your shell, it will not use the Atmos-managed version. To use the
Atmos-managed version in your shell, opt in to shell integration.

Shell integration is a **supported mode**. It is not a limitation. Use `atmos toolchain env` to
export the resolved `PATH` into your interactive shell:

**Bash** (add to `~/.bashrc`):
```bash
eval "$(atmos toolchain env --format=bash)"
```

**Zsh** (add to `~/.zshrc`):
```zsh
eval "$(atmos toolchain env --format=bash)"
```
Atmos has no separate `--format=zsh` option. The `bash` format emits plain POSIX
`export PATH=...` output. Zsh evaluates this output the same way.

**Fish** (add to `~/.config/fish/config.fish`):
```fish
atmos toolchain env --format=fish | source
```

**PowerShell** (add to `$PROFILE`):
```powershell
Invoke-Expression (atmos toolchain env --format powershell | Out-String)
```

Verify the setup with `which terraform` (use `Get-Command terraform` on PowerShell). The command
must resolve under the Atmos toolchain install directory, not a tenv shim. Repeat this check for
`tofu`, `terragrunt`, and `tflint` as needed.

Run `atmos` commands, including the shell integration commands above, from the directory that
contains `atmos.yaml`. If your shell is in a different directory, add `--chdir /path/to/project`
(short form `-C /path/to/project`) to the command.

**Important: `atmos toolchain env` creates a static snapshot. It does not repeat tenv's
per-directory resolution.** tenv's shims read their version files again on every command. Because
of this, changing directories (`cd`) into a different project automatically switches versions for
every tool tenv manages.

`atmos toolchain env` writes the current directory's resolved paths into `PATH` one time, at eval
time. It does not update automatically when you change directories. Re-run the `eval` line after
you switch projects. Alternatively, when you work across multiple projects, use
`atmos terraform ...` instead of bare tool commands. `atmos terraform ...` always resolves the
version for each invocation, regardless of shell state.

## Functional Gaps

- **Atmos toolchain has no equivalent for partial or range-based version selectors yet.** Like
  tfenv and tofuenv, tenv supports these selectors (for example, `latest:^1.9`, `latest-stable`)
  for `tenv <tool> install`. Atmos toolchain version specs support exact versions, `latest`,
  `pr:<n>`, `sha:<hex>`, and `ref:<branch/tag>`. SemVer range constraints are documented as
  **planned, not yet implemented**. Pin exact versions for now.
- **Atmos does not natively orchestrate Terragrunt** the way it orchestrates Terraform and
  OpenTofu. Atmos toolchain can still install and pin the `terragrunt` binary through
  `.tool-versions` or `dependencies.tools`. Running Terragrunt is the user's own workflow or
  custom command. It is not a built-in Atmos component type.

## Cross-Links

- [atmos-toolchain SKILL.md](../../atmos-toolchain/SKILL.md): `.tool-versions`,
  `dependencies.tools`, version spec syntax.
- [atmos-toolchain commands-reference.md](../../atmos-toolchain/references/commands-reference.md):
  full CLI command reference.
- [from-tfenv.md](from-tfenv.md) / [from-tofuenv.md](from-tofuenv.md): single-tool version of
  these same steps.
