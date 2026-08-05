# Replace tofuenv with the Atmos Toolchain

This reference describes how to replace [tofuenv](https://github.com/tofuutils/tofuenv) with the
Atmos toolchain. For the decision guide, see [../SKILL.md](../SKILL.md). For the full toolchain
feature reference, see [atmos-toolchain](../../atmos-toolchain/SKILL.md).

The user can also manage Terraform versions with tfenv, or Terragrunt and TFLint versions with
tenv. In that case, see [from-tfenv.md](from-tfenv.md) and [from-tenv.md](from-tenv.md). The steps
are the same. Only the tool differs.

## Overview

tofuenv has one purpose. It pins an OpenTofu version. Use an `.opentofu-version` file (a single
bare version string), or use the `tofuenv use X.Y.Z` command.

The Atmos equivalent is one line in `.tool-versions`. For the common case, this migration is
mechanical. The one gap is tofuenv's partial or range-based version selectors. See Functional Gaps.

## Before / After

**Before:**
```text
# .opentofu-version
1.10.3
```
You can also set the version with the `tofuenv use 1.10.3` command. tofuenv records this version
in `~/.tofuenv/version` as the global default. Or tofuenv records the version in
`.opentofu-version` as a per-project pin.

**After:**
```text
# .tool-versions
opentofu 1.10.3
```
```yaml
# atmos.yaml
toolchain:
  aliases:
    opentofu: opentofu/opentofu
  registries:
    - name: aqua
      type: aqua
      source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
      priority: 10
components:
  terraform:
    command: tofu # tell Atmos to invoke the OpenTofu binary
```
```bash
atmos toolchain install
```

## Steps

1. **Read the current pin.** Run `cat .opentofu-version` if the file exists. Otherwise, run
   `tofuenv version-name` to find the active version.
2. **Add the version to `.tool-versions`.** Append `opentofu <version>`. Create the file if it
   does not exist.
3. **Add the `toolchain:` block to `atmos.yaml`** if the repository does not already have one.
   Set `components.terraform.command: tofu`. This makes `atmos terraform` invoke OpenTofu instead
   of Terraform (see Before/After above).
4. **Verify the setup.** Run `atmos toolchain install`, then run `atmos toolchain which opentofu`.
   Confirm that `atmos terraform plan <component> -s <stack>` uses the expected version.
5. **Promote the version to `dependencies.tools`** on the specific component. Do this if the
   version must apply only to that component's execution context, not to the whole repository as
   a default. See the "Default Rule" in [atmos-toolchain](../../atmos-toolchain/SKILL.md).
6. **Remove `.opentofu-version` and uninstall tofuenv last.** Do this only after the team
   verifies that `atmos terraform` commands resolve the correct version without it.

## Command Mapping

| tofuenv command | Atmos toolchain equivalent |
|---|---|
| `tofuenv install 1.10.3` | `atmos toolchain install opentofu@1.10.3` |
| `tofuenv use 1.10.3` | `atmos toolchain set opentofu 1.10.3` |
| `tofuenv version-name` | `atmos toolchain get opentofu` |
| `tofuenv list` (installed versions) | `atmos toolchain list` |
| `tofuenv list-remote` (available versions) | `atmos toolchain search opentofu` |
| `tofuenv uninstall 1.10.3` | `atmos toolchain uninstall opentofu@1.10.3` |

## Shell Integration

tofuenv works through shimming. Its installer adds `~/.tofuenv/bin` to `PATH`. It typically does
this with `export PATH="$HOME/.tofuenv/bin:$PATH"` in `~/.bashrc` or `~/.zshrc`. The `tofu` shim
reads the nearest `.opentofu-version` file on every invocation. Plain `tofu` always resolves to
the correct version in any shell. This needs no per-project setup.

**The Atmos toolchain does not do this by default.** Atmos resolves tools declared in
`dependencies.tools` and injects them into `PATH` only for the duration of one
`atmos <subcommand>` invocation. Plain `tofu` in your shell does not pick up the Atmos-managed
version unless you opt in to shell integration. This is a **supported mode**, not a limitation.
Use `atmos toolchain env` to export the resolved `PATH` into your interactive shell:

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

Verify with `which tofu` (`Get-Command tofu` on PowerShell). The result must point to the Atmos
toolchain install directory, not the tofuenv shim.

**Important: this is a static snapshot, not tofuenv's dynamic per-directory resolution.**
tofuenv's shim re-reads `.opentofu-version` on every command. Changing directory into a different
project automatically switches versions. `atmos toolchain env` sets the resolved paths for the
*current* directory into `PATH` one time, at eval time. It does not update automatically when you
change directory. Re-run the `eval` line after you switch projects. Or, when you work across
multiple projects, use `atmos terraform ...` instead of bare `tofu`. `atmos terraform ...` always
resolves the version per invocation, regardless of shell state.

## Functional Gaps

- **Partial or range-based version selectors have no equivalent today.** tofuenv supports
  selectors similar to tfenv's selectors (`latest:^1.9`, `latest:1.10.*`) through
  `tofuenv install` arguments. Atmos toolchain version specs support exact versions, `latest`,
  `pr:<n>`, `sha:<hex>`, and `ref:<branch/tag>`. SemVer range constraints (`~> 1.9.0`,
  `>= 1.8.0, < 2.0.0`) are documented as **planned, not yet implemented**. If the user relies on
  a floating constraint, pin an exact version for now. Tell the user that range support is
  planned.

## Cross-Links

- [atmos-toolchain SKILL.md](../../atmos-toolchain/SKILL.md): covers `.tool-versions`,
  `dependencies.tools`, and version spec syntax.
- [atmos-toolchain commands-reference.md](../../atmos-toolchain/references/commands-reference.md):
  full CLI command reference.
- [from-tfenv.md](from-tfenv.md): the same steps, for Terraform.
- [from-tenv.md](from-tenv.md): the same steps, for tenv's multi-tool version files.
