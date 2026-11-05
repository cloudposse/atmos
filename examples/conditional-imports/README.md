---
title: Configure and Select Components with Metadata
tags: [Stacks, Mixins]
description: Use a native label as a feature flag to select configuration and find the components that inherit it.
cast:
  file: /casts/examples/conditional-imports/pci.cast
  title: Configure and select components with metadata
related_docs:
  - label: "Conditional imports"
    url: /stacks/imports#conditional-imports
  - label: "Mixins"
    url: /howto/mixins
---

# Configure and Select Components with Metadata

Use the native `pci` label as a feature flag: one shared [conditional import](https://atmos.tools/stacks/imports#conditional-imports)
selects the retention mixin wherever the workload is PCI-scoped. The rule depends on that classification, independent
of the environment name. In this example, dev is outside PCI scope and prod is inside it. Both import
`catalog/environment`, which contains the rule:

```yaml
# stacks/catalog/environment.yaml
import:
  - catalog/app
  - path: mixins/pci
    when: !cel labels.pci == true
```

The app reads its log retention from [settings](https://atmos.tools/stacks/settings), falling back to `7` days:

```yaml
# stacks/catalog/app.yaml
components:
  terraform:
    app:
      vars:
        log_retention_days: !template '{{ .settings | dig "default" "log_retention_days" 7 }}'
```

The PCI mixin supplies the setting for any component that consumes it:

```yaml
# stacks/mixins/pci.yaml
settings:
  default:
    log_retention_days: 366
```

For example, [PCI DSS v4.0.1, requirement 10.5.1](https://www.middlebury.edu/sites/default/files/2025-01/PCI-DSS-v4_0_1.pdf#page=256) calls for retaining audit logs for at least
12 months, with the most recent three months immediately available for analysis.

Here we use 366 days to account for leap years.

Atmos evaluates the condition, merges the imported settings, then renders the variable. The
[`!template` function](https://atmos.tools/functions/yaml/template) preserves the numeric value;
`dig` returns `7` when the setting is absent. Templates and Sprig are enabled by default in the current
[configuration edition](https://atmos.tools/cli/configuration/edition). Older edition pins can enable them explicitly.

Each environment supplies its classification, which flows into the shared configuration's nested imports:

```yaml
# stacks/dev.yaml
name: dev
metadata:
  labels:
    pci: false
import:
  - catalog/environment
```

```yaml
# stacks/prod.yaml
name: prod
metadata:
  labels:
    pci: true
import:
  - catalog/environment
```

Dev keeps 7 days; prod selects 366 days. Define the conditional rule once, then let each environment's metadata
choose the configuration and identify its component instances.

## Try It

```shell
atmos init examples/conditional-imports
cd conditional-imports

atmos list instances --labels pci:true
# Selects prod/app.

atmos describe component app -s dev --query .vars
# log_retention_days: 7

atmos describe component app -s prod --query .vars
# log_retention_days: 366
```

These commands and `atmos test` need no cloud credentials and create no resources. The
Terraform component displays the selected configuration with `terraform_data`; it does not
configure a real logging service.

The `list instances` command keeps the stack name alongside the component name; use that pair with
`describe component` to inspect it.

The flag lives in stack-wide `metadata.labels`, so local and CI runs use the same checked-in
value. It is an Atmos label, not a Terraform variable or a cloud resource tag. Use an unquoted
boolean: `true` and the string `"true"` are different values.

Conditions can also read labels inherited from earlier imports. The current manifest's labels
take precedence, and flow into nested imports. Later imports do not change decisions already
made. Import conditions use literal string and boolean labels; templates, YAML functions, and
component-specific labels are unavailable at this stage. Metadata must be readable as YAML before template rendering. Component overrides and later imports can change
final metadata after the import decision; keep the classification in the root stack for this pattern.
