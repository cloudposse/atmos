# Introspection Commands Reference

Complete reference for all `atmos describe` and `atmos list` subcommands with flags and examples.

## Describe Commands

### atmos describe component

Display complete resolved configuration for a component in a stack.

```shell
atmos describe component <component> -s <stack> [options]
```

#### Flags

| Flag | Alias | Default | Description |
|------|-------|---------|-------------|
| `--stack` | `-s` | -- | Stack name (required) |
| `--format` | `-f` | `yaml` | Output format (yaml, json) |
| `--file` | | -- | Write result to file |
| `--process-templates` | | `true` | Enable Go template processing |
| `--process-functions` | | `true` | Enable YAML function processing |
| `--skip` | | -- | Skip specific YAML functions |
| `--query` | `-q` | -- | Filter output with yq expressions |
| `--provenance` | | `false` | Show configuration value origins |
| `--identity` | `-i` | -- | Authentication identity |

#### Path Resolution

Supports path-based component references:
- `atmos describe component .` -- Current directory component
- `atmos describe component ./vpc` -- Relative path
- `atmos describe component vpc` -- Component name lookup

---

### atmos describe stacks

Show fully deep-merged configuration for stacks.

```shell
atmos describe stacks [options]
```

#### Flags

| Flag | Alias | Default | Description |
|------|-------|---------|-------------|
| `--stack` | `-s` | -- | Filter by specific stack |
| `--components` | | -- | Filter by component names (comma-separated) |
| `--component-types` | | -- | Filter by type (terraform, helmfile, packer) |
| `--sections` | | -- | Specific sections: backend, deps, env, inheritance, metadata, remote_state_backend, settings, vars |
| `--format` | `-f` | `yaml` | Output format (yaml, json) |
| `--file` | | -- | Write result to file |
| `--include-empty-stacks` | | `false` | Include stacks with no components |
| `--process-templates` | | `true` | Enable Go template processing |
| `--process-functions` | | `true` | Enable YAML function processing |
| `--skip` | | -- | Skip specific YAML functions |
| `--query` | `-q` | -- | Filter with yq expressions |
| `--identity` | `-i` | -- | Authentication identity |

---

### atmos describe affected

Identify components and stacks affected by Git changes.

```shell
atmos describe affected [options]
```

#### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--ref` | `refs/remotes/origin/HEAD` | Git reference to compare |
| `--sha` | -- | Git commit SHA (takes precedence over --ref) |
| `--repo-path` | -- | Path to pre-cloned target repo |
| `--clone-target-ref` | `false` | Clone target ref instead of checkout |
| `--ssh-key` | -- | PEM-encoded private key for SSH cloning |
| `--ssh-key-password` | -- | Encryption password for PEM key |
| `--format` | `json` | Output format (json, yaml) |
| `--file` | -- | Write result to file |
| `--stack` | -- | Filter by stack |
| `--include-spacelift-admin-stacks` | `false` | Include Spacelift admin stacks |
| `--include-dependents` | `false` | Include dependent components |
| `--include-settings` | `false` | Include settings section |
| `--exclude-locked` | `false` | Exclude locked components |
| `--upload` | `false` | Upload to HTTP endpoint |
| `--process-templates` | `true` | Enable Go template processing |
| `--process-functions` | `true` | Enable YAML function processing |
| `--skip` | -- | Skip specific YAML functions |
| `--query` | -- | Filter with yq expressions |
| `--identity` | -- | Authentication identity |

#### Comparison Methods

1. **--repo-path** -- Use pre-cloned repo (fastest, no network)
2. **--clone-target-ref=true** -- Clone from remote (reliable, uses network)
3. **Default** -- Checkout from local .git directory (recommended for most cases)

---

### atmos describe dependents

List components that depend on a given component.

```shell
atmos describe dependents <component> --stack <stack> [options]
```

**Aliases:** `dependants`

#### Flags

| Flag | Alias | Default | Description |
|------|-------|---------|-------------|
| `--stack` | `-s` | -- | Stack name (required) |
| `--format` | `-f` | `json` | Output format (json, yaml) |
| `--file` | | -- | Write result to file |
| `--query` | `-q` | -- | Filter with yq expressions |
| `--process-templates` | | `true` | Enable Go template processing |
| `--process-functions` | | `true` | Enable YAML function processing |

---

### atmos describe config

Display the final merged CLI configuration.

```shell
atmos describe config [options]
```

#### Flags

| Flag | Alias | Default | Description |
|------|-------|---------|-------------|
| `--format` | `-f` | `json` | Output format (json, yaml) |
| `--query` | `-q` | -- | Filter with yq expressions |

---

### atmos describe workflows

List all workflows and their associated files.

```shell
atmos describe workflows [options]
```

#### Flags

| Flag | Alias | Default | Description |
|------|-------|---------|-------------|
| `--format` | `-f` | `yaml` | Output format (yaml, json) |
| `--output` | `-o` | `list` | Output type (list, map, all) |
| `--query` | `-q` | -- | Filter with yq expressions |

---

### atmos describe locals

Display locals defined in stack manifests.

```shell
atmos describe locals [component] --stack <stack> [options]
```

#### Flags

| Flag | Alias | Default | Description |
|------|-------|---------|-------------|
| `--stack` | `-s` | -- | Stack name (required) |
| `--format` | `-f` | `yaml` | Output format (yaml, json) |
| `--file` | | -- | Write result to file |
| `--query` | `-q` | -- | Filter with yq expressions |

---

## List Commands

### atmos list stacks

```shell
atmos list stacks [options]
```

#### Flags

| Flag | Alias | Default | Description |
|------|-------|---------|-------------|
| `--component` | | -- | Filter stacks by component |
| `--format` | `-f` | `table` | Output format (table, json, yaml, csv, tsv, tree) |
| `--columns` | | -- | Custom column selection |
| `--sort` | | -- | Sort specification |
| `--provenance` | | `false` | Show import provenance (tree format only) |

---

### atmos list components

```shell
atmos list components [options]
```

#### Flags

| Flag | Alias | Default | Description |
|------|-------|---------|-------------|
| `--stack` | `-s` | -- | Filter by stack pattern (glob supported) |
| `--type` | | `real` | Component type filter (real, abstract, all) |
| `--abstract` | | `false` | Include abstract components |
| `--enabled` | | -- | Filter by enabled status (true/false) |
| `--locked` | | -- | Filter by locked status (true/false) |
| `--format` | `-f` | `table` | Output format (table, json, yaml, csv, tsv, tree) |
| `--columns` | | -- | Custom column selection |
| `--sort` | | -- | Sort specification |

---

### atmos list instances

```shell
atmos list instances [options]
```

#### Flags

| Flag | Alias | Default | Description |
|------|-------|---------|-------------|
| `--stack` | | -- | Filter by stack pattern |
| `--filter` | | -- | YQ-based filter expressions |
| `--query` | `-q` | -- | YQ expression for value extraction |
| `--format` | `-f` | `table` | Output format (table, json, yaml, csv, tsv, tree) |
| `--columns` | | -- | Custom column selection |
| `--max-columns` | | -- | Maximum columns to display |
| `--delimiter` | | -- | CSV/TSV delimiter |
| `--sort` | | -- | Sort specification |
| `--upload` | | `false` | Upload instances to Atmos Pro API |
| `--provenance` | | `false` | Show import provenance (tree format only) |

---

### atmos list affected (Experimental)

```shell
atmos list affected [options]
```

#### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--ref` | -- | Git reference to compare |
| `--sha` | -- | Git commit SHA |
| `--repo-path` | -- | Pre-cloned repo path |
| `--include-dependents` | `false` | Include dependent components |
| `--exclude-locked` | `false` | Exclude locked components |
| `--format` | `table` | Output format (table, json, yaml, csv, tsv) |
| `--columns` | -- | Custom column selection |
| `--sort` | -- | Sort specification |

---

### atmos list workflows

```shell
atmos list workflows [options]
```

#### Flags

| Flag | Alias | Default | Description |
|------|-------|---------|-------------|
| `--file` | | -- | Filter by workflow file |
| `--format` | `-f` | `table` | Output format (table, json, yaml, csv, tsv, tree) |
| `--columns` | | -- | Custom column selection |
| `--sort` | | -- | Sort specification |

---

### atmos list values

```shell
atmos list values <component> [options]
```

#### Flags

| Flag | Alias | Default | Description |
|------|-------|---------|-------------|
| `--stack` | | -- | Filter by stack |
| `--abstract` | | `false` | Show abstract component values |
| `--vars` | | `false` | Show only vars section |
| `--format` | `-f` | -- | Output format (json, yaml, csv) |
| `--query` | `-q` | -- | YQ expression filter |

---

### atmos list vendor

```shell
atmos list vendor [options]
```

**Alias:** `atmos vendor list`

#### Flags

| Flag | Alias | Default | Description |
|------|-------|---------|-------------|
| `--format` | `-f` | `table` | Output format (table, json, yaml, csv, tsv) |
| `--columns` | | -- | Custom column selection |
| `--sort` | | -- | Sort specification |

---

### Other List Commands

```shell
atmos list aliases                     # Command aliases
atmos list settings <component>        # Component settings across stacks
atmos list sources <component>         # Component source information
atmos list themes                      # Available CLI themes
atmos list metadata                    # Metadata information
atmos list vars <component>            # Component variables across stacks
```

---

## Global Flags (All describe/list commands)

| Flag | Description |
|------|-------------|
| `--base-path` | Base path for Atmos configuration |
| `--config` | Configuration file name |
| `--config-path` | Configuration path |
| `--profile` | Configuration profile |
| `--logs-level` | Logging level (debug, info, warn, error) |
| `--identity` / `-i` | Authentication identity |
| `--query` / `-q` | YQ filter expression |
| `--pager` | Pager command (more, less) |
