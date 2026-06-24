# Toolchain Commands Reference

Complete reference for all `atmos toolchain` subcommands, flags, and usage patterns.

## Persistent Flags (All Commands)

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--github-token` | `ATMOS_GITHUB_TOKEN`, `GITHUB_TOKEN` | -- | GitHub authentication token (hidden) |
| `--tool-versions` | `ATMOS_TOOL_VERSIONS` | `.tool-versions` | Path to .tool-versions file |
| `--toolchain-path` | `ATMOS_TOOLCHAIN_PATH` | `.tools` | Tool installation directory |

---

## atmos toolchain install

Install tools from .tool-versions or by name.

```shell
atmos toolchain install [tool@version] [flags]
```

### Examples

```shell
atmos toolchain install                        # Install all from .tool-versions
atmos toolchain install terraform@1.9.8        # Install specific version
atmos toolchain install jq                     # Install latest from .tool-versions
```

---

## atmos toolchain uninstall

Remove an installed tool.

```shell
atmos toolchain uninstall <tool@version>
```

---

## atmos toolchain add

Add a tool to the .tool-versions file.

```shell
atmos toolchain add <tool[@version]>...
```

If no version is specified, the latest available version is used.

### Examples

```shell
atmos toolchain add terraform                  # Add latest version
atmos toolchain add terraform@1.9.8            # Add specific version
atmos toolchain add terraform kubectl helm     # Add multiple tools
```

---

## atmos toolchain remove

Remove a tool from the .tool-versions file.

```shell
atmos toolchain remove <tool[@version]>
```

---

## atmos toolchain set

Set the default version for a tool when multiple versions are installed.

```shell
atmos toolchain set <tool> <version>
```

---

## atmos toolchain get

Get the version of a tool from the .tool-versions file.

```shell
atmos toolchain get [tool]
```

---

## atmos toolchain list

Show installed tools with version, size, and installation date.

```shell
atmos toolchain list
```

---

## atmos toolchain info

Display tool configuration from the registry.

```shell
atmos toolchain info <tool>
```

Shows: package type, source, supported platforms, available versions.

---

## atmos toolchain which

Show the full filesystem path to a tool binary.

```shell
atmos toolchain which <tool>
```

---

## atmos toolchain search

Search for tools across all configured registries.

```shell
atmos toolchain search <query>
```

---

## atmos toolchain aliases

List all configured tool aliases.

```shell
atmos toolchain aliases
```

---

## atmos toolchain exec

Execute a tool with a specific version, installing it first if needed.

```shell
atmos toolchain exec <tool@version> [-- args...]
```

### Examples

```shell
atmos toolchain exec terraform@1.9.8 -- plan
atmos toolchain exec jq@1.7.1 -- '.foo' file.json
```

---

## atmos toolchain env

Output PATH environment variables for shell evaluation.

```shell
atmos toolchain env [--format=bash|json|fish|powershell|github] [--relative]
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--format` | `bash` | Output format (bash, json, fish, powershell, github) |
| `--relative` | false | Use relative paths instead of absolute |

### Examples

```shell
eval "$(atmos toolchain env)"                         # Bash/Zsh
eval "$(atmos toolchain env --format=bash)"           # Explicit bash
atmos toolchain env --format=fish | source            # Fish
atmos toolchain env --format=powershell | Invoke-Expression  # PowerShell
atmos toolchain env --format=github >> $GITHUB_PATH   # GitHub Actions
```

---

## atmos toolchain path

Print PATH entries for installed tools (one per line).

```shell
atmos toolchain path
```

---

## atmos toolchain du

Show disk usage of installed tools and cache.

```shell
atmos toolchain du
```

---

## atmos toolchain clean

Remove all installed tools and cached registry data.

```shell
atmos toolchain clean
```

---

## atmos toolchain registry list

List configured registries or tools in a specific registry.

```shell
atmos toolchain registry list [name]
```

### Examples

```shell
atmos toolchain registry list              # List all registries
atmos toolchain registry list aqua         # List tools in aqua registry
```

---

## atmos toolchain registry search

Search for tools across all configured registries.

```shell
atmos toolchain registry search <query>
```

---

## Registry Configuration Reference

### Aqua Registry

```yaml
registries:
  - name: aqua
    type: aqua
    source: https://github.com/aquaproj/aqua-registry/tree/main/pkgs
    ref: v4.260.0           # Optional: pin to specific registry version
    priority: 10
```

Source formats:
- GitHub tree URL: `https://github.com/owner/repo/tree/ref/path`
- File path: `file://./registry.yaml` (single file)
- File directory: `file://./pkgs/` (per-package structure)

### Inline (Atmos) Registry

```yaml
registries:
  - name: custom
    type: atmos
    priority: 150
    tools:
      owner/repo:
        type: github_release          # or http
        url: "asset_{{.Version}}_{{.OS}}_{{.Arch}}.tar.gz"
        format: tar.gz                # tar.gz, zip, gz, raw
        files:
          - name: binary-name
            src: path/in/archive
```

### Supported Package Types

| Type | Description |
|------|-------------|
| `github_release` | Download from GitHub release assets |
| `http` | Download from arbitrary HTTP URL |

### Supported Archive Formats

| Format | Description |
|--------|-------------|
| `tar.gz` | Gzip tarball |
| `zip` | ZIP archive |
| `gz` | Single gzip-compressed binary |
| `raw` | Uncompressed binary |
| `pkg` | macOS installer package |
