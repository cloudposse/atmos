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

Use native metadata to select a [mixin](https://atmos.tools/howto/mixins) and find the components that inherit it:

```yaml
metadata:
  labels:
    pci: true
  tags:
    - pci

import:
  - catalog/app
  - path: mixins/pci
    when: !cel labels.pci == true
```

Both stacks import the same catalog and use the same condition. The `payments` stack sets
`pci: true`, so it includes the mixin that enables `audit_logging_enabled`. The `standard`
stack sets `pci: false`, so it keeps the catalog's default of `false`.

## Try It

```shell
atmos init examples/conditional-imports
cd conditional-imports

atmos list instances --labels pci:true
atmos list instances --tags pci
# Both select payments/app.

atmos describe component app -s standard --query .vars
# audit_logging_enabled: false

atmos describe component app -s payments --query .vars
# audit_logging_enabled: true
```

These commands and `atmos test` need no cloud credentials and create no resources. The
Terraform component displays the selected configuration with `terraform_data`; it does not
configure a real logging service or establish PCI compliance.

To use the tag to enable the mixin instead, replace its condition with:

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
