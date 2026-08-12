# Replace asdf with the Atmos Toolchain

This reference describes how to replace [asdf](https://asdf-vm.com/) with the Atmos toolchain. For
the skill's decision guide, see [../SKILL.md](../SKILL.md). For the full toolchain feature
reference, see [atmos-toolchain](../../atmos-toolchain/SKILL.md).

## Overview

asdf and the Atmos toolchain read the same file format. Both use `.tool-versions`, with one
`<tool> <version>` pair per line.

When you migrate from asdf, you do not need to rewrite this file. The existing `.tool-versions`
file usually needs no changes.

Two things change: the tool download source and the install method. Atmos downloads tools from an
Atmos or Aqua registry. asdf downloads tools from an asdf plugin. asdf installs tools with the
`asdf install` command. Atmos installs tools automatically when you run an `atmos` command.

## Before / After

**Before** (asdf-managed):
```text
# .tool-versions
terraform 1.9.8
kubectl 1.28.0
helm 3.13.0
```
```bash
asdf plugin add terraform
asdf plugin add kubectl
asdf plugin add helm
asdf install
```

**After** (Atmos-managed, same `.tool-versions` file, unchanged):
```yaml
# atmos.yaml
toolchain:
  aliases:
    terraform: hashicorp/terraform
    kubectl: kubernetes/kubectl
    helm: helm/helm
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

1. **Leave `.tool-versions` in its current location.** If the file is not at the repo root, set
  `toolchain.versions_file` (or `ATMOS_TOOL_VERSIONS`) to its path. Do not move the file.
2. **Add a minimal `toolchain:` block to `atmos.yaml`.** Add an `aliases:` entry for each tool
  whose asdf plugin name does not match an `owner/repo` value in the public Aqua registry. Also
  add the public Aqua registry itself as a fallback. See Before/After above for an example.
3. **Verify tool resolution.** Run `atmos toolchain install` to install every tool declared in
  `.tool-versions`. Then run `atmos toolchain list`. Run `atmos toolchain which terraform` to
  confirm the resolved binary path.
4. **Move frequently used tools to `dependencies.tools`.** `.tool-versions` is a good default for
  the whole repository's developer shell. But if a specific component, workflow, or custom
  command needs a pinned version as part of its contract, declare the tool there instead. See the
  "Default Rule" and "Where to Declare a Tool" table in
  [atmos-toolchain](../../atmos-toolchain/SKILL.md). This step is optional. It is not required
  for the migration to work.
5. **Remove the asdf shims last.** First confirm that `atmos <command>` resolves tools correctly
  without the shims on `PATH`. Uninstall asdf plugins (`asdf plugin remove <name>`) or asdf
  itself only after the whole team has switched to the Atmos toolchain.

## Command Mapping

| asdf command | Atmos toolchain equivalent |
|---|---|
| `asdf plugin add terraform` | No plugin step needed. Add the tool to `toolchain.aliases`/`registries` only if the public Aqua registry does not already resolve it. |
| `asdf install` | `atmos toolchain install` |
| `asdf install terraform 1.9.8` | `atmos toolchain install terraform@1.9.8` |
| `asdf global terraform 1.9.8` | `atmos toolchain set terraform 1.9.8` (writes the default version to `.tool-versions`) |
| `asdf local terraform 1.9.8` | `atmos toolchain add terraform@1.9.8` (adds/updates the `.tool-versions` entry) |
| `asdf current` | `atmos toolchain list` |
| `asdf current terraform` | `atmos toolchain get terraform` |
| `asdf which terraform` | `atmos toolchain which terraform` |
| `asdf uninstall terraform 1.9.8` | `atmos toolchain uninstall terraform@1.9.8` |
| `asdf plugin list all` | `atmos toolchain registry search <name>` |
| `asdf shell terraform 1.9.8` (session-only override) | `atmos toolchain exec terraform@1.9.8 -- <args>` (one-off pinned run of the raw binary) |
| `. ~/.asdf/asdf.sh` in shell rc (shim-based, automatic) | `eval "$(atmos toolchain env --format=bash)"` in shell rc |

Note: `atmos terraform`, `atmos helmfile`, and other Atmos commands do not need any of these
steps. These commands resolve and inject declared tool versions automatically. The table above
maps commands for direct, asdf-style interactive use: installing, listing, pinning, and running a
raw third-party binary.

## Shell Integration

asdf uses shims to work. The line `. "$HOME/.asdf/asdf.sh"` in `~/.bashrc` or `~/.zshrc` adds
`~/.asdf/shims` to `PATH`. Each shim reads the nearest `.tool-versions` file again each time it
runs. As a result, plain `terraform` always resolves correctly in any shell, IDE terminal, or
script. This works with no per-project setup.

**The Atmos toolchain does not do this by default.** Atmos resolves and injects tools declared in
`.tool-versions` (the project-wide default) or `dependencies.tools` (a scoped override) into `PATH`
only for the duration of an `atmos <subcommand>` invocation. If you run plain `terraform` in your
shell, it will not use the Atmos-managed version. To use the Atmos-managed version in your shell,
you must opt in to shell integration. This is a supported mode, not a limitation. Use
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
`export PATH=...` syntax. Zsh evaluates this syntax identically to bash.

**Fish** (add to `~/.config/fish/config.fish`):
```fish
atmos toolchain env --format=fish | source
```

**PowerShell** (add to `$PROFILE`):
```powershell
Invoke-Expression (atmos toolchain env --format powershell | Out-String)
```

Verify the result with `which terraform` (`Get-Command terraform` on PowerShell). The command must
resolve to the Atmos toolchain install directory, not the system `terraform` or an asdf shim.

**Important: `atmos toolchain env` creates a static snapshot. It does not match asdf's dynamic
per-directory resolution.** asdf shims read `.tool-versions` again on every command. So when you
change directory (`cd`) into a different project, asdf automatically switches versions. `atmos
toolchain env` sets the resolved paths for the current directory into `PATH` one time, at eval
time. It does not update automatically when you change directory. Run the `eval` line again after
you switch projects. Alternatively, use `atmos terraform ...` instead of bare `terraform` when you
work across multiple projects. `atmos terraform ...` always resolves tools per invocation,
regardless of shell state.

## Functional Gaps

- **Atmos has no equivalent for plugin-specific behavior.** asdf plugins can run arbitrary shell
  commands during install. These commands can include custom build steps, post-install hooks, and
  non-standard download URLs. An Atmos tool is either an Aqua registry entry (GitHub-release or
  HTTP download) or a `type: atmos` inline registry entry. Atmos has no hook mechanism. Most
  common CLI tools work through the public Aqua registry without changes. A tool that depends on a
  custom asdf plugin script can need a custom `type: atmos` registry entry. Some such tools cannot
  migrate to Atmos at all.
- **The Atmos toolchain has no equivalent to asdf's `.tool-versions` `system` sentinel.** The
  `system` sentinel tells asdf to use the tool already on `PATH`. Atmos resolves and installs
  every declared tool instead.

## Cross-Links

- [atmos-toolchain SKILL.md](../../atmos-toolchain/SKILL.md): full `toolchain:` config reference,
  `dependencies.tools`, registries, aliases.
- [atmos-toolchain commands-reference.md](../../atmos-toolchain/references/commands-reference.md):
  full CLI command reference.
