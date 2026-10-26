---
title: Configure and Select Components with Metadata
tags: [Stacks, Mixins]
description: Use native labels and tags to select configuration and find the components that inherit it.
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

Use one shared [conditional import](https://atmos.tools/stacks/imports#conditional-imports) to select log retention for different environments. Both dev and prod import
`catalog/environment`, which contains the rule:

```yaml
# stacks/catalog/environment.yaml
import:
  - catalog/app
  - path: mixins/pci
    when: !cel labels.pci == true
```

The shared catalog defaults `log_retention_days` to `7`. The PCI mixin overrides it to `365`.
Each environment supplies its classification, which flows into the shared configuration's nested imports:

```yaml
# stacks/dev.yaml
name: dev
metadata:
  labels:
    pci: false
  tags: []
import:
  - catalog/environment
```

```yaml
# stacks/prod.yaml
name: prod
metadata:
  labels:
    pci: true
  tags:
    - pci
import:
  - catalog/environment
```

Dev keeps 7 days; prod selects 365 days. Define the conditional rule once, then let each environment's metadata
choose the configuration and identify its component instances.

## Try It

```shell
atmos init examples/conditional-imports
cd conditional-imports

atmos list instances --labels pci:true
atmos list instances --tags pci
# Both select prod/app.

atmos describe component app -s dev --query .vars
# log_retention_days: 7

atmos describe component app -s prod --query .vars
# log_retention_days: 365
```

These commands and `atmos test` need no cloud credentials and create no resources. The
Terraform component displays the selected configuration with `terraform_data`; it does not
configure a real logging service or establish PCI compliance. The retention periods are illustrative.

To use the tag to enable the mixin instead, replace the condition in `stacks/catalog/environment.yaml` with:

```yaml
when: !cel "'pci' in tags"
```

The label and tag selectors identify the same instance in this example. `list instances` keeps the
stack name alongside the component name; use that pair with `describe component` to inspect it.

The flag lives in stack-wide `metadata.labels`, so local and CI runs use the same checked-in
value. It is an Atmos label, not a Terraform variable or a cloud resource tag. Use an unquoted
boolean: `true` and the string `"true"` are different values.

Conditions can also read labels and tags inherited from earlier imports. The current manifest's labels
take precedence, and flow into nested imports. Later imports do not change decisions already
made. Import conditions use literal string and boolean labels; templates, YAML functions, and
component-specific labels are unavailable at this stage. Metadata must be readable as YAML before template rendering. Tags follow the configured list-merge
strategy, including structural `!append` lists. Component overrides and later imports can change
final metadata after the import decision; keep the classification in the root stack for this pattern.
