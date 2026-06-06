---
name: atmos-introspection
description: "Introspection & Querying: describe/list commands, config filtering, workspace introspection, dependency graphs, YQ integration"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - references/commands-reference.md
---

# Atmos Introspection

## Purpose

Atmos provides powerful introspection commands (`describe` and `list`) for querying the stack graph,
component configurations, dependencies, and change impact. AI agents operating in a terminal should
use these commands to understand the user's workspace instead of guessing at configurations.

## When to Use Introspection

- **Before generating configuration** -- Run `atmos describe component` to see the current resolved config
- **To find dependencies** -- Run `atmos describe dependents` to see what depends on a component
- **To understand project structure** -- Run `atmos list stacks` and `atmos list components`
- **For CI/CD** -- Run `atmos describe affected` or `atmos list affected` to detect changed stacks
- **To debug configuration** -- Use `--provenance` to trace where values originate

## Describe Commands

### atmos describe component

Display the complete, fully resolved configuration for a specific component in a stack.

```bash
atmos describe component vpc -s plat-ue2-prod
atmos describe component vpc -s plat-ue2-prod --format json
atmos describe component vpc -s plat-ue2-prod --provenance
atmos describe component vpc -s plat-ue2-prod -q '.vars.cidr_block'
```

Key flags:
- `-s, --stack` (required) -- Stack name
- `-f, --format` -- Output format (yaml, json). Default: yaml
- `--provenance` -- Show where each configuration value originated (file:line:column)
- `-q, --query` -- Filter output with yq expressions
- `--process-templates` / `--process-functions` -- Enable/disable template and YAML function processing
- `--skip` -- Skip specific YAML functions during processing

The output includes all resolved sections: `vars`, `settings`, `env`, `backend`, `metadata`,
`deps`, `inheritance`, and `remote_state_backend`.

### atmos describe stacks

Show fully deep-merged configuration for all stacks and their components.

```bash
atmos describe stacks
atmos describe stacks -s plat-ue2-prod
atmos describe stacks --components vpc,eks
atmos describe stacks --component-types terraform
atmos describe stacks --sections vars,settings
atmos describe stacks -q '.[] | select(.vars.environment == "prod")'
```

Key flags:
- `-s, --stack` -- Filter by specific stack
- `--components` -- Filter by component names (comma-separated)
- `--component-types` -- Filter by type (terraform, helmfile, packer)
- `--sections` -- Output specific sections (backend, deps, env, inheritance, metadata, remote_state_backend, settings, vars)
- `--include-empty-stacks` -- Include stacks with no components

### atmos describe affected

Identify components and stacks affected by Git changes between two refs.

```bash
atmos describe affected
atmos describe affected --ref main
atmos describe affected --sha abc123
atmos describe affected --include-dependents
atmos describe affected --format json --file affected.json
```

Key flags:
- `--ref` -- Git reference to compare (default: refs/remotes/origin/HEAD)
- `--sha` -- Git commit SHA (takes precedence over --ref)
- `--repo-path` -- Path to pre-cloned target repo (fastest, avoids cloning)
- `--clone-target-ref` -- Clone the target reference instead of checking out
- `--include-dependents` -- Include components that depend on changed components
- `--include-settings` -- Include the settings section for each affected component
- `--include-spacelift-admin-stacks` -- Include Spacelift admin stacks of affected stacks
- `--exclude-locked` -- Exclude components marked as locked
- `--upload` -- Upload results to an HTTP endpoint (for CI/CD integration)
- `--ssh-key` -- Path to PEM-encoded private key for SSH cloning
- `--ssh-key-password` -- Encryption password for the PEM-encoded private key
- `--process-templates` -- Enable/disable Go template processing (default: true)
- `--process-functions` -- Enable/disable YAML functions processing (default: true)
- `--skip` -- Skip executing specific YAML functions

### atmos describe dependents

List components that depend on a given component.

```bash
atmos describe dependents vpc --stack plat-ue2-prod
atmos describe dependents vpc --stack plat-ue2-prod --format json
```

Key flags:
- `--stack` (required) -- Stack name
- `-f, --format` -- Output format (json, yaml). Default: json

### atmos describe config

Display the final merged CLI configuration (atmos.yaml resolution result).

```bash
atmos describe config
atmos describe config --format json
atmos describe config -q '.stacks.name_pattern'
```

### atmos describe workflows

List all workflows and their associated files.

```bash
atmos describe workflows
atmos describe workflows --format json
atmos describe workflows --output map
```

### atmos describe locals

Display locals defined in stack manifests.

```bash
atmos describe locals --stack plat-ue2-prod
atmos describe locals vpc --stack plat-ue2-prod
```

## List Commands

### atmos list stacks

List all stacks with optional component filtering.

```bash
atmos list stacks
atmos list stacks --component vpc
atmos list stacks --format tree --provenance
atmos list stacks --format json
```

Key flags:
- `--component` -- Filter stacks that contain a specific component
- `--provenance` -- Show import provenance (tree format only)
- `-f, --format` -- Output format (table, json, yaml, csv, tsv, tree)

### atmos list components

List all unique component definitions.

```bash
atmos list components
atmos list components -s 'plat-*-prod'
atmos list components --type abstract
atmos list components --enabled true
atmos list components --format tree
```

Key flags:
- `-s, --stack` -- Filter by stack pattern (glob supported)
- `--type` -- Filter by component type (real, abstract, all). Default: real
- `--abstract` -- Include abstract components
- `--enabled` -- Filter by enabled status (true/false)
- `--locked` -- Filter by locked status (true/false)

### atmos list instances

List all component-stack combinations (instances).

```bash
atmos list instances
atmos list instances --stack 'plat-*-prod'
atmos list instances --format json
atmos list instances --columns component,stack,type
atmos list instances --sort 'stack:asc,component:asc'
```

Key flags:
- `--stack` -- Filter by stack pattern
- `--filter` -- YQ-based filter expressions
- `--columns` -- Custom column selection
- `--sort` -- Sort specification (e.g., `component:asc,stack:desc`)
- `--upload` -- Upload instances to Atmos Pro API

### atmos list affected

List affected components in table format (tabular version of `describe affected`).

```bash
atmos list affected
atmos list affected --ref main --include-dependents
atmos list affected --format csv
```

This is an **experimental** command.

### atmos list workflows

List all workflows with file and description information.

```bash
atmos list workflows
atmos list workflows --file deploy.yaml
```

### Other List Commands

```bash
atmos list values <component>       # Component values across stacks
atmos list vars <component>         # Component variables across stacks
atmos list settings <component>     # Component settings across stacks
atmos list sources <component>      # Component source information
atmos list vendor                   # Vendor configurations
atmos list aliases                  # Command aliases
atmos list themes                   # Available CLI themes
atmos list metadata                 # Metadata information
```

## Common Patterns

### Output Format Support

| Command Type | Formats |
|-------------|---------|
| Describe commands | yaml, json |
| List commands | table (default), json, yaml, csv, tsv, tree |

### Column Customization (List Commands)

```bash
# Simple field names
atmos list instances --columns component,stack,type

# Named columns with templates
atmos list instances --columns "Component={{ .component }},Stack={{ .stack }}"
```

### Sorting (List Commands)

```bash
atmos list instances --sort component:asc
atmos list instances --sort "stack:asc,component:desc"
```

### YQ Query Filtering

All commands support `-q, --query` for filtering with yq expressions:

```bash
atmos describe stacks -q '.[] | select(.vars.environment == "prod")'
atmos describe component vpc -s plat-ue2-prod -q '.vars'
atmos list instances --filter '.type == "terraform"'
```

### Provenance Tracking

Use `--provenance` to trace where configuration values originate:

```bash
atmos describe component vpc -s plat-ue2-prod --provenance
atmos list stacks --format tree --provenance
```

Shows file:line:column details and import hierarchy visualization.

### Template and Function Control

```bash
# Disable Go template processing
atmos describe component vpc -s prod --process-templates=false

# Disable YAML functions
atmos describe stacks --process-functions=false

# Skip specific YAML functions
atmos describe component vpc -s prod --skip '!terraform.output'
```

### Authentication for Remote Resources

```bash
# Use specific identity for YAML function resolution
atmos describe component vpc -s prod -i prod-admin

# Interactive identity selection
atmos describe component vpc -s prod -i
```

## Introspection Workflow for AI Agents

When assisting a user with Atmos configuration, follow this sequence:

1. **Understand the project**: `atmos describe config` to see atmos.yaml settings
2. **List what exists**: `atmos list stacks` and `atmos list components`
3. **Inspect specific configs**: `atmos describe component <name> -s <stack>`
4. **Check dependencies**: `atmos describe dependents <name> -s <stack>`
5. **Trace configuration origins**: Add `--provenance` to any describe command
6. **Validate changes**: `atmos describe affected` to see impact of modifications

Never guess at stack names, component names, or configuration values. Always query first.
