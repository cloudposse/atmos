# Replace Aqua with the Atmos Toolchain

This reference explains how to replace the [Aqua](https://aquaproj.github.io/) CLI with the Atmos
toolchain. For the skill's decision guide, see [../SKILL.md](../SKILL.md). For the full toolchain
feature reference, see [atmos-toolchain](../../atmos-toolchain/SKILL.md).

## Overview

The Atmos toolchain reimplements the Aqua registry YAML schema with its own parser. It does not
use the Aqua Go SDK. The toolchain understands the same `pkgs/` registry format as Aqua. However,
the toolchain supports only a **subset** of the schema. It is **not** a full reimplementation.

Migration is mechanical for plain GitHub-release packages. Migration is not purely mechanical for
packages that use the schema features listed in Functional Gaps below. Check each package against
that table before you assume a 1:1 port.

## Before / After

**Before** (`aqua.yaml`):
```yaml
registries:
  - type: standard
    ref: v4.245.0 # renovate: depName=aquaproj/aqua-registry

packages:
  - name: hashicorp/terraform
    version: v1.9.8
  - name: kubernetes/kubectl
    version: v1.28.0
  - name: jqlang/jq
    version: v1.7.1
```
Aqua also generates `aqua-checksums.json`. This file records checksums for supply-chain
verification.

**After** (`atmos.yaml` + `.tool-versions`):
```yaml
# atmos.yaml
toolchain:
  use_lock_file: true
  registries:
    - name: aqua
      type: aqua
      source: https://github.com/aquaproj/aqua-registry/pkgs
      ref: v4.245.0
      priority: 10
  verification:
    checksums: when_available
    signatures: when_available
```
```text
# .tool-versions
hashicorp/terraform 1.9.8
kubernetes/kubectl 1.28.0
jqlang/jq 1.7.1
```
```bash
atmos toolchain install
```
Atmos also generates a lock file when you set `use_lock_file: true`. By default, Atmos writes it
as `toolchain.lock.yaml` under the toolchain install path. Set `toolchain.lock_file` to choose a
different path, for example a repo-relative path you can commit. This file records the resolved
version and checksum for each tool. It serves the same purpose as `aqua-checksums.json`.

## Steps

1. **Mirror the registry.** Map Aqua's `type: standard` registry with a `ref:` pin to a
  `toolchain.registries` entry of `type: aqua`. Set `source` to
  `https://github.com/aquaproj/aqua-registry/pkgs`. Set `ref` to the same tag Aqua pinned. Atmos
  resolves the pin through the separate `ref` field, not through the URL path. If `aqua.yaml` used
  a private or custom registry, add it as a second `type: aqua` registry entry. Use a `file://` or
  GitHub `source` for this entry. Set its `priority` higher than the public registry.
2. **Convert each plain package.** For a `packages:` entry with no unusual fields, run
  `atmos toolchain add owner/repo@version`. You can also hand-write the `.tool-versions` line.
  Both methods produce the same result.
3. **Flag nonstandard packages.** For any entry that uses a field or package type from the
  Functional Gaps table below, do not assume it ports automatically. Follow that row's
  workaround. Or leave the tool on Aqua temporarily until the team decides how to handle it.
4. **Replace `aqua-checksums.json` with `toolchain.verification` and `use_lock_file`.** Atmos
  verifies packages against checksum and signature metadata published by the registry itself.
  Set `checksums` and `signatures` to `when_available`, `required`, or `disabled`. Set
  `use_lock_file: true` to also record resolved versions and checksums in `toolchain.lock.yaml`,
  for the same reproducibility `aqua-checksums.json` provides.
5. **Verify the migration.** Run `atmos toolchain install`, then run `atmos toolchain list`.
  Confirm the resolved versions match what `aqua list` reported before.

## Command Mapping

| aqua command | Atmos toolchain equivalent |
|---|---|
| `aqua install` | `atmos toolchain install` |
| `aqua install github.com/hashicorp/terraform@v1.9.8` (ad hoc) | `atmos toolchain install hashicorp/terraform@1.9.8` |
| `aqua g -i` (generate + insert into `aqua.yaml`) | `atmos toolchain add owner/repo@version` (writes to `.tool-versions`) |
| `aqua list` | `atmos toolchain list` |
| `aqua which terraform` | `atmos toolchain which terraform` |
| `aqua exec -- terraform plan` | `atmos toolchain exec terraform@<version> -- plan` (direct third-party use only). `atmos terraform plan` already resolves the toolchain automatically. Do not wrap it in `atmos toolchain exec`. |
| `aqua update-checksum` | No separate command. `toolchain.verification` handles this automatically at install time. |

## Shell Integration

Aqua uses the same shim-based method as asdf. Add `export PATH="$(aqua root-dir)/bin:$PATH"` to
`~/.bashrc` or `~/.zshrc`. This puts Aqua's proxy directory on `PATH`. `aqua-proxy` resolves the
nearest `aqua.yaml` again on every invocation. As a result, plain `terraform` always resolves
correctly in any shell. This method needs no per-project setup.

**The Atmos toolchain does not do this by default.** Atmos resolves tools declared in
`.tool-versions` (the project-wide default) or `dependencies.tools` (a scoped override) and injects
them into `PATH` only for the duration of one `atmos <subcommand>` invocation. If you run plain
`terraform` in your shell, it will not use the Atmos-managed version unless you opt in to shell
integration. This is a **supported mode**, not a limitation. Use `atmos toolchain env` to export the
resolved `PATH` into your interactive shell:

**Bash** (add to `~/.bashrc`):
```bash
eval "$(atmos toolchain env --format=bash)"
```

**Zsh** (add to `~/.zshrc`):
```zsh
eval "$(atmos toolchain env --format=bash)"
```
Atmos has no separate `--format=zsh` option. The `bash` format emits plain POSIX
`export PATH=...`. Zsh evaluates this output the same way.

**Fish** (add to `~/.config/fish/config.fish`):
```fish
atmos toolchain env --format=fish | source
```

**PowerShell** (add to `$PROFILE`):
```powershell
Invoke-Expression (atmos toolchain env --format powershell | Out-String)
```

Verify with `which terraform` (use `Get-Command terraform` on PowerShell). It must resolve under
the Atmos toolchain install directory, not Aqua's proxy directory.

Run `atmos` commands, including the shell integration commands above, from the directory that
contains `atmos.yaml`. If your shell is in a different directory, add `--chdir /path/to/project`
(short form `-C /path/to/project`) to the command.

**Important: this method takes a static snapshot, not Aqua's dynamic per-directory resolution.**
`aqua-proxy` resolves the nearest `aqua.yaml` again on every command. As a result, when you `cd`
into a different project, Aqua automatically switches versions. `atmos toolchain env` bakes the
resolved paths of the current directory into `PATH` one time, at eval time. It does not update
automatically when you `cd`. Re-run the `eval` line after you switch projects. Or use
`atmos terraform ...` instead of bare `terraform` when you work across multiple projects.
`atmos terraform ...` always resolves per invocation, regardless of shell state.

**Atmos also has a direct equivalent to `aqua-proxy`: `toolchain.proxies`.** Configure a proxy to
expose a toolchain tool under a command name that differs from the tool's own binary name. Atmos
creates a link in `${toolchain.install_path}/bin/proxy`. That link re-invokes Atmos under the
configured command name. Atmos then resolves the tool, installs it if needed, and forwards the
arguments. `atmos toolchain env` puts this proxy directory on `PATH` when you configure at least
one proxy. Use this when a package name differs from the command name, for example a multicall
binary such as `coreutils` that must run under the name `ls`. You do not need a proxy for the
common case, where the tool's binary already has the name you want to type. In that case, the
direct `PATH` export above already provides it. The
[atmos-toolchain](../../atmos-toolchain/SKILL.md) skill does not yet cover `toolchain.proxies` in
depth; see [Toolchain Proxies](https://atmos.tools/cli/configuration/toolchain/proxies) for the
full reference.

## Functional Gaps

Atmos intentionally does not support the following Aqua schema features. Most of this list comes
from the "Unsupported Aqua Features" list in [atmos-toolchain](../../atmos-toolchain/SKILL.md).

| Aqua feature | Why it does not port directly | Workaround |
|---|---|---|
| `go_build` package type | Not implemented. Atmos does not build tools from source. | Use a pre-built release if the project publishes one. Add it via a `type: atmos` inline registry. |
| `cargo` package type | Not implemented. Atmos has no Cargo or crates.io installer. | No direct equivalent exists. Source the binary another way. |
| `version_filter` | Atmos does not support this Aqua registry field. Aqua uses it to filter candidate GitHub tags before version resolution. | Pin an exact, already-known-good version in `.tool-versions`. |
| `version_expr` / `version_expr_prefix` | Atmos does not support version-string manipulation through an expression. | Pin an exact version. |
| `go_version_file` | Atmos does not support reading a version from a Go source file. | Pin the version explicitly in `.tool-versions`. |
| `import` | Not needed. Atmos already lets you compose multiple registries by importing multiple `atmos.yaml` files through its own `import:` mechanism. A separate registry-level `import` field was not implemented. | Add multiple `toolchain.registries` entries directly. Or split them across `atmos.yaml` files and combine them with `import:`. |
| `command_aliases` | Atmos does not support this Aqua field. | Use `toolchain.proxies` in `atmos.yaml` instead. `toolchain.proxies` creates a command-name link, the same role `command_aliases` plays in Aqua. Do not use `toolchain.aliases` for this. `toolchain.aliases` only maps a short tool name to an `owner/repo` registry entry for lookup. It does not change the command name a tool runs under. See the Shell Integration section above. |
| `tags` | Not supported. | No equivalent exists. Atmos does not filter installs by tag. |
| `vars` | Atmos does not support Aqua's per-package template variables. | Hard-code the value into the `url` template of a `type: atmos` inline registry entry. |

**`version_prefix` is supported, not a gap.** Atmos reads `version_prefix` directly from an Aqua
registry package definition. It applies the prefix automatically when it builds the download URL
and compares versions. A package that already ships from an Aqua registry needs no workaround. Add
`version_prefix` to a custom `type: atmos` inline registry entry when you define a tool with no
upstream Aqua registry entry.

**`github_archive` and `github_content` are supported, not a gap.** Atmos added these two Aqua
package types in [PR #2416](https://github.com/cloudposse/atmos/pull/2416). Treat a package that
uses either type the same as any other Aqua package: convert it with
`atmos toolchain add owner/repo@version`.

## Example

This mixed `aqua.yaml` file shows both cases:
```yaml
packages:
  - name: hashicorp/terraform    # plain GitHub release -- migrates cleanly
    version: v1.9.8
  - name: some-org/custom-tool   # uses go_build -- needs manual porting
    version: v2.1.0
```

`hashicorp/terraform` becomes `atmos toolchain add hashicorp/terraform@1.9.8`. For
`some-org/custom-tool`, add a `type: atmos` inline registry entry that points at a pre-built
release asset, if one exists. Otherwise, leave it out of scope for this migration pass.

## Cross-Links

- [atmos-toolchain SKILL.md](../../atmos-toolchain/SKILL.md): the Registries section and the
  "Unsupported Aqua Features" list. The Functional Gaps table above comes from this list.
- [atmos-toolchain commands-reference.md](../../atmos-toolchain/references/commands-reference.md):
  covers `atmos toolchain add`, `registry list`, `registry search`.
