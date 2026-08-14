# Replace a Homebrew Brewfile with the Atmos Toolchain

This reference explains how to replace the CLI-tool part of a [Homebrew Brewfile](https://github.com/Homebrew/homebrew-bundle)
(`brew bundle`) with the Atmos toolchain. For the skill decision guide, see
[../SKILL.md](../SKILL.md). For the full toolchain feature reference, see
[atmos-toolchain](../../atmos-toolchain/SKILL.md).

## Scope

This migration covers only part of the Brewfile. The Atmos toolchain manages only CLI binaries.
Atmos toolchain fetches these binaries from a GitHub release or a plain HTTP download. This is
the same model Aqua uses.

The Atmos toolchain does **not** replace these items:

- `cask` entries (GUI apps)
- `mas` entries (Mac App Store apps)
- Homebrew formulae that require a build from source or a complex dependency graph

Keep these items in the Brewfile. Continue to install them with `brew bundle`. Only plain
`brew "<cli-tool>"` lines that map to a standalone, prebuilt CLI binary are migration candidates.

## Before / After

**Before** (`Brewfile`):
```ruby
tap "hashicorp/tap"

brew "hashicorp/tap/terraform"
brew "kubernetes-cli"
brew "jq"
cask "docker"
mas "Xcode", id: 497799835
```

**After** (`atmos.yaml` and `.tool-versions`, with the Brewfile reduced to items that stay out of
scope):
```yaml
# atmos.yaml
toolchain:
  aliases:
    terraform: hashicorp/terraform
    kubectl: kubernetes/kubectl
  registries:
    - name: aqua
      type: aqua
      source: https://github.com/aquaproj/aqua-registry/pkgs
      ref: v4.550.0
      priority: 10
```
```text
# .tool-versions
terraform 1.9.8
kubectl 1.28.0
jq 1.7.1
```
```bash
atmos toolchain install
```
```ruby
# Brewfile (slimmed -- only what Atmos toolchain doesn't cover)
cask "docker"
mas "Xcode", id: 497799835
```

## Steps

1. **Classify each Brewfile line.** A plain `brew "<formula>"` entry for a standalone CLI tool is
  a candidate. `cask` entries, `mas` entries, and formulae that pull in a heavy dependency tree
  or require a source build stay out of scope. Leave these entries in the Brewfile.
2. **Resolve each candidate's GitHub `owner/repo`.** Homebrew formula names often differ from the
  upstream GitHub repository name. This differs from asdf or aqua. For example, `kubernetes-cli`
  maps to `kubernetes/kubectl`, and `hashicorp/tap/terraform` maps to `hashicorp/terraform`.
  You must do this step manually. If the mapping is not obvious, check the formula's `homepage` or
  `url` field with `brew info <formula>`.
3. **Pick a version to pin.** A Brewfile has no native per-project version pinning. `brew bundle`
  always installs the current version in the tap. Ask the user which version to pin. Or use the
  currently installed version as a starting point: run `brew list --versions <formula>`.
4. **Add each tool.** Run `atmos toolchain add owner/repo@version` for each resolved candidate.
5. **Add the `toolchain:` block to `atmos.yaml`** if the repository does not already have one.
  See the Before/After section above.
6. **Verify the result.** Run `atmos toolchain install`, then run `atmos toolchain list`.
7. **Reduce the Brewfile to the out-of-scope entries only**: casks, `mas` entries, and
  source-built formulae. Continue to run `brew bundle` for these entries. Do not delete the
  Brewfile unless every line in it was a migration candidate.

## Command Mapping

| Homebrew command | Atmos toolchain equivalent |
|---|---|
| `brew bundle` (installs everything in the Brewfile) | `atmos toolchain install` (installs everything in `.tool-versions`). This covers only the CLI-tool subset, not casks or `mas` entries. |
| `brew install terraform` | `atmos toolchain add hashicorp/terraform@<version>`, then `atmos toolchain install`. Or run `atmos toolchain install hashicorp/terraform@<version>` for a one-time install. |
| `brew list --versions terraform` | `atmos toolchain list` |
| `brew bundle dump` (generates a Brewfile from the installed state) | No direct equivalent. Build `.tool-versions` by running `atmos toolchain add` for each tool, or write the file by hand. |
| `brew uninstall terraform` | `atmos toolchain uninstall hashicorp/terraform@<version>` |
| `brew upgrade terraform` | `atmos toolchain add hashicorp/terraform@<new-version>`, then `atmos toolchain install`. Atmos pins exact versions. It does not track "latest" implicitly. |

## Shell Integration

Homebrew already uses this same pattern. Add `eval "$(/opt/homebrew/bin/brew shellenv)"` to
`~/.bashrc` or `~/.zshrc` (use `/usr/local/bin/brew shellenv` on Intel Macs). This command puts
Homebrew's `bin` directory on `PATH` once, permanently. Every formula's binary lives in that
directory. Homebrew installs are global, not per-project, so each binary is the same version in
every shell and every directory.

The Atmos toolchain uses the same eval-into-rc-file pattern. But the tools it resolves are
**project-scoped**, not global. The nearest `.tool-versions` file or `dependencies.tools` section
drives the resolution:

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

Verify the result with `which terraform` (use `Get-Command terraform` on PowerShell). The command
must resolve to the Atmos toolchain install directory, not Homebrew's `bin` directory.

Run `atmos` commands, including the shell integration commands above, from the directory that
contains `atmos.yaml`. If your shell is in a different directory, add `--chdir /path/to/project`
(short form `-C /path/to/project`) to the command.

**Important: this is a real behavior change from Homebrew's global installs, not only a syntax
change.** With Homebrew, `terraform` was the same version in every shell and every directory.
`atmos toolchain env` sets `PATH` from the current directory's resolved paths at eval time. If you
switch to a project with a different pinned version, you must re-run the `eval` line. Or use
`atmos terraform ...` instead of bare `terraform`. The `atmos terraform ...` command always
resolves the version per invocation, regardless of shell state. Tell the user about this
tradeoff. It is the cost of the per-project reproducibility that the Brewfile never provided.

## Functional Gaps

- **The Brewfile has no native versioning. This is a behavior change, not a bug.** When you move
  these tools under the Atmos toolchain, the team gets reproducible, pinned versions for the
  first time. Flag this as an improvement. But confirm the user wants pinning, not
  always-latest, before you migrate.
- **The formula-to-repository name mapping is manual.** No registry maps Homebrew formula names
  to GitHub repositories automatically. Each candidate needs a one-time lookup.
- **The Atmos toolchain does not support casks, `mas` entries, or source-built formulae.** Do not
  propose replacement of the whole Brewfile. Only the plain CLI-binary subset is in scope.

## Cross-Links

- [atmos-toolchain SKILL.md](../../atmos-toolchain/SKILL.md): `.tool-versions`,
  `dependencies.tools`, registries, aliases.
- [atmos-toolchain commands-reference.md](../../atmos-toolchain/references/commands-reference.md):
  full CLI command reference.
