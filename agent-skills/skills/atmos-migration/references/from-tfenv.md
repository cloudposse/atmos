# Replace tfenv with the Atmos Toolchain

This reference explains how to replace [tfenv](https://github.com/tfutils/tfenv) with the Atmos
toolchain. For the skill's decision guide, see [../SKILL.md](../SKILL.md). For the full toolchain
feature reference, see [atmos-toolchain](../../atmos-toolchain/SKILL.md). Some users also manage
OpenTofu versions with tofuenv, or Terragrunt and TFLint versions with tenv. For these tools, see
[from-tofuenv.md](from-tofuenv.md) and [from-tenv.md](from-tenv.md). The steps are the same. Only
the tool changes.

## Overview

tfenv has one purpose. It pins a Terraform version. It uses a `.terraform-version` file, which
holds a single version string. It also uses the `tfenv use X.Y.Z` command. The Atmos equivalent is
one line in `.tool-versions`. For most cases, this migration is mechanical. One gap remains:
tfenv's partial or range-based version selectors. See Functional Gaps.

## Before / After

**Before:**
```text
# .terraform-version
1.9.8
```
You can also set the version with the `tfenv use 1.9.8` command. This command records the version
in `~/.tfenv/version` as the global default. Or it records the version in `.terraform-version` for
a per-project pin.

**After:**
```text
# .tool-versions
terraform 1.9.8
```
```yaml
# atmos.yaml
toolchain:
  aliases:
    terraform: hashicorp/terraform
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

1. **Read the current pin.** Run `cat .terraform-version` if the file exists. Otherwise, run
  `tfenv version-name` for the current active version.
2. **Add it to `.tool-versions`.** Append `terraform <version>`. Create the file if it does not
  exist.
3. **Add the `toolchain:` block** to `atmos.yaml` if the repository does not already have one. See
  Before/After above.
4. **Verify** the setup. Run `atmos toolchain install`. Then run `atmos toolchain which terraform`.
  Confirm that `atmos terraform plan <component> -s <stack>` uses the expected version.
5. **Promote to `dependencies.tools`** on the specific component. Do this if the version must apply
  only to that component, not to the whole repository. See the "Default Rule" in
  [atmos-toolchain](../../atmos-toolchain/SKILL.md).
6. **Remove `.terraform-version` and uninstall tfenv last.** Do this only after the team verifies
  that `atmos terraform` commands resolve the correct version without it.

## Command Mapping

| tfenv command | Atmos toolchain equivalent |
|---|---|
| `tfenv install 1.9.8` | `atmos toolchain install terraform@1.9.8` |
| `tfenv use 1.9.8` | `atmos toolchain set terraform 1.9.8` |
| `tfenv version-name` | `atmos toolchain get terraform` |
| `tfenv list` (installed versions) | `atmos toolchain list` |
| `tfenv list-remote` (available versions) | `atmos toolchain search terraform` |
| `tfenv uninstall 1.9.8` | `atmos toolchain uninstall terraform@1.9.8` |

## Shell Integration

tfenv works through shimming. Its installer adds `~/.tfenv/bin` to `PATH`. It typically does this
with `export PATH="$HOME/.tfenv/bin:$PATH"` in `~/.bashrc` or `~/.zshrc`. The `terraform` shim
re-reads the nearest `.terraform-version` file on every command. As a result, plain `terraform`
always resolves to the correct version in any shell. This setup needs no per-project configuration.

**The Atmos toolchain does not do this by default.** Atmos resolves tools declared in
`.tool-versions` (the project-wide default) or `dependencies.tools` (a scoped override) and injects
them into `PATH`. It does this only for the duration of one `atmos <subcommand>` command. If you
run plain `terraform` in your shell, it will not use the Atmos-managed version. To use it, you must
opt in to shell integration. This is a **supported mode**, not a limitation. Use
`atmos toolchain env` to export the resolved `PATH` into your interactive shell:

**Bash** (add to `~/.bashrc`):
```bash
eval "$(atmos toolchain env --format=bash)"
```

**Zsh** (add to `~/.zshrc`):
```zsh
eval "$(atmos toolchain env --format=bash)"
```
Atmos has no separate `--format=zsh` option. The `bash` format emits plain POSIX
`export PATH=...` syntax. Zsh evaluates this syntax the same way.

**Fish** (add to `~/.config/fish/config.fish`):
```fish
atmos toolchain env --format=fish | source
```

**PowerShell** (add to `$PROFILE`):
```powershell
Invoke-Expression (atmos toolchain env --format powershell | Out-String)
```

Verify the result with `which terraform`. On PowerShell, use `Get-Command terraform`. The command
must resolve to the Atmos toolchain install directory, not the tfenv shim.

**Important: this is a static snapshot, not tfenv's dynamic per-directory resolution.** tfenv's
shim re-reads `.terraform-version` on every command. So when you `cd` into a different project,
tfenv switches versions automatically. `atmos toolchain env` sets `PATH` once, at eval time, using
the resolved paths of the *current* directory. It does not update automatically when you `cd`.
Re-run the `eval` line after you switch projects. Or use `atmos terraform ...` instead of bare
`terraform` when you work across multiple projects. The `atmos terraform ...` command always
resolves the version per invocation, regardless of shell state.

## Functional Gaps

- **The Atmos toolchain has no equivalent for partial or range-based version selectors yet.**
  tfenv supports selectors like `latest:^1.5` or `latest:1.9.*` through `tfenv install` arguments.
  These selectors use a partial constraint and select the latest matching release. Atmos toolchain
  version specs support exact versions, `latest`, `pr:<n>`, `sha:<hex>`, and `ref:<branch/tag>`.
  SemVer range constraints, such as `~> 1.9.0` or `>= 1.8.0, < 2.0.0`, are documented as
  **planned, not yet implemented**. If the user relies on a floating constraint, pin an exact
  version for now. Note that range support is coming.

## Cross-Links

- [atmos-toolchain SKILL.md](../../atmos-toolchain/SKILL.md): covers `.tool-versions`,
  `dependencies.tools`, and version spec syntax.
- [atmos-toolchain commands-reference.md](../../atmos-toolchain/references/commands-reference.md):
  full CLI command reference.
- [from-tofuenv.md](from-tofuenv.md): same steps, for OpenTofu.
- [from-tenv.md](from-tenv.md): same steps, for tenv's multi-tool version files.
