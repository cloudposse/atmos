---
name: atmos-templates
description: "Go templates: Sprig/Gomplate functions, atmos.Component, atmos.GomplateDatasource, atmos.Store, template configuration, evaluations"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - references/go-templates.md
---

# Atmos Go Templates

## Overview

Go templates use `{{ }}` delimiters and are processed **before** YAML parsing. They support the full
Go `text/template` syntax plus Sprig and Gomplate function libraries.

Go templates are an escape hatch for complex conditional logic, loops, dynamic key generation, or
advanced string manipulation that YAML functions cannot handle. For most use cases, prefer YAML
functions (see the `atmos-yaml-functions` skill).

## Enabling Go Templates

```yaml
# atmos.yaml
templates:
  settings:
    enabled: true
    evaluations: 1      # Number of processing passes
    delimiters: ["{{", "}}"]  # Default delimiters
    sprig:
      enabled: true     # Enable Sprig functions
    gomplate:
      enabled: true     # Enable Gomplate functions and datasources
      timeout: 5        # Datasource timeout in seconds
```

## Template Context Variables

In Go templates, you can reference any value from the component's configuration (as returned by
`atmos describe component`):

| Variable | Description |
|----------|-------------|
| `.atmos_component` | The Atmos component name |
| `.atmos_stack` | The Atmos stack name |
| `.stack` | Alias for `.atmos_stack` |
| `.atmos_stack_file` | The stack manifest file path |
| `.workspace` | The Terraform workspace name |
| `.vars.*` | Component variables |
| `.settings.*` | Component settings |
| `.env.*` | Environment variables |
| `.metadata.*` | Component metadata |
| `.providers.*` | Provider configuration |
| `.backend.*` | Backend configuration |
| `.backend_type` | Backend type string |

## Template Examples

```yaml
components:
  terraform:
    vpc:
      vars:
        tags:
          atmos_component: "{{ .atmos_component }}"
          atmos_stack: "{{ .atmos_stack }}"
          terraform_workspace: "{{ .workspace }}"
          # Sprig function
          provisioned_by: '{{ env "USER" }}'
          # Gomplate function
          description: "{{ strings.Title .atmos_component }} in {{ .atmos_stack }}"
```

## `atmos.Component` Template Function

Reads any section or attribute from another component's configuration, including Terraform outputs:

```yaml
vars:
  # Read outputs (remote state)
  vpc_id: '{{ (atmos.Component "vpc" .stack).outputs.vpc_id }}'

  # Read variables from another component
  vpc_name: '{{ (atmos.Component "vpc" .stack).vars.name }}'

  # Read settings
  test_setting: '{{ (atmos.Component "test" .stack).settings.test }}'

  # Read metadata
  component_name: '{{ (atmos.Component "test" .stack).metadata.component }}'

  # Complex outputs require !template + toJson
  subnet_ids: !template '{{ toJson (atmos.Component "vpc" .stack).outputs.private_subnet_ids }}'
```

## `atmos.GomplateDatasource` Template Function

Fetches external data with automatic caching:

```yaml
settings:
  templates:
    settings:
      gomplate:
        datasources:
          ip:
            url: "https://api.ipify.org?format=json"
          secret:
            url: "aws+smp:///path/to/secret"

vars:
  public_ip: '{{ (atmos.GomplateDatasource "ip").ip }}'
  db_password: '{{ (atmos.GomplateDatasource "secret").password }}'
```

The function caches results per execution -- multiple references to the same datasource make
only one external call.

## `atmos.Store` Template Function

Reads from stores using Go template syntax (same as `!store` YAML function but in template form):

```yaml
vars:
  vpc_id: '{{ atmos.Store "prod/ssm" .stack "vpc" "vpc_id" }}'
  config: !template '{{ (atmos.Store "redis" .stack "config" "config_map").defaults | toJSON }}'
```

## Template Evaluations (Processing Pipelines)

Atmos supports multiple evaluation passes for template processing:

```yaml
# atmos.yaml
templates:
  settings:
    enabled: true
    evaluations: 2  # Two passes
```

With multiple evaluations, output from the first pass becomes input to the second pass. This is
useful for:
- Combining templates from different sections
- Using templates in datasource URLs
- Multi-stage template resolution

## Excluding Templates from Processing

### Passing Templates to External Tools

Use the backtick escape or `!literal` to prevent Atmos from processing templates intended for
external systems (ArgoCD, Helm, Datadog):

```yaml
# Using !literal (recommended, see atmos-yaml-functions skill)
annotation: !literal "{{ .Values.ingress.class }}"

# Using backtick escape
annotation: "{{`{{ .Values.ingress.class }}`}}"

# Using printf
annotation: '{{ printf "{{ .Values.ingress.class }}" }}'
```

### Templates in Imports

When using Go templates in both imports and stack manifests, templates intended for the second
pass (stack processing) must be escaped in the import file:

```yaml
# stacks/catalog/eks/eks_cluster.tmpl
components:
  terraform:
    eks/cluster:
      vars:
        # First pass: resolved from import context
        enabled: "{{ .enabled }}"
        name: "{{ .name }}"
        tags:
          # Second pass: escaped for stack processing
          atmos_component: "{{`{{ .atmos_component }}`}}"
          atmos_stack: "{{`{{ .atmos_stack }}`}}"
```

## Template Configuration in Stack Manifests

Template settings can be defined in `settings.templates.settings` in stack manifests, which
deep-merges with `templates.settings` in `atmos.yaml`. Stack manifest settings take precedence.

```yaml
# stacks/orgs/acme/_defaults.yaml
settings:
  templates:
    settings:
      env:
        AWS_PROFILE: "my-profile"
      gomplate:
        timeout: 7
        datasources:
          config:
            url: "./my-config.json"
```

Note: `enabled`, `sprig.enabled`, `gomplate.enabled`, `evaluations`, and `delimiters` settings
are not supported in stack manifests (only in `atmos.yaml`).

## When to Use Go Templates vs. YAML Functions

| Scenario | Use |
|----------|-----|
| Reading Terraform outputs | YAML functions: `!terraform.state` or `!terraform.output` |
| Reading store values | YAML functions: `!store` or `!store.get` |
| Environment variables | YAML function: `!env` |
| Including files | YAML function: `!include` |
| Complex outputs (lists/maps) | `!template` with `toJson` |
| Conditional logic (`if/else`) | Go templates |
| Loops and iteration | Go templates |
| Dynamic key generation | Go templates |
| External API data | `atmos.GomplateDatasource` |
| Advanced string manipulation | Go templates with Sprig/Gomplate |

## Performance Best Practices

1. **Prefer YAML functions over Go templates** -- Type-safe, cannot break YAML
2. **Prefer `!store` over `atmos.Component` for outputs** -- Avoids Terraform initialization
3. **Use `atmos.GomplateDatasource` instead of `datasource`** -- Built-in caching prevents
   redundant API calls
4. **Minimize `atmos.Component` usage** -- Each call may initialize Terraform
5. **All template functions cache results** per execution for repeated calls

## Common Pitfalls

1. **Go templates break YAML** -- Unquoted `{{ }}` can cause YAML parse errors. Always quote
   template expressions.
2. **Type confusion** -- Go templates always return strings. Use `!template` with `toJson` for
   complex types.
3. **Indentation issues** -- Multi-line template output can break YAML indentation.
4. **Sprig/Gomplate conflicts** -- The `env` function exists in both libraries with different
   syntax. Use `getenv` for Gomplate's version when both are enabled.
5. **Performance degradation** -- Overuse of `atmos.Component` or `!terraform.output` across
   many stacks can dramatically slow `atmos describe stacks` and `atmos describe affected`.

## Additional Resources

- For complete Go template context variables and functions, see [references/go-templates.md](references/go-templates.md)
- For YAML functions (`!terraform.state`, `!store`, `!env`, etc.), see the `atmos-yaml-functions` skill
