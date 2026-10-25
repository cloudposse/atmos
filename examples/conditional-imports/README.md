---
title: Conditional Imports with Labels
tags: [Stacks, Mixins]
description: Use a native Atmos label to enable an optional mixin.
cast:
  file: /casts/examples/conditional-imports/pci.cast
  title: Select a mixin with a native label
related_docs:
  - label: "Conditional imports"
    url: /stacks/imports#conditional-imports
  - label: "Mixins"
    url: /howto/mixins
---

# Conditional Imports with Labels

Use one native Atmos label to select an optional [mixin](https://atmos.tools/howto/mixins):

```yaml
metadata:
  labels:
    pci: true

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

atmos describe component app -s standard --query .vars
# audit_logging_enabled: false

atmos describe component app -s payments --query .vars
# audit_logging_enabled: true
```

These commands and `atmos test` need no cloud credentials and create no resources. The
Terraform component displays the selected configuration with `terraform_data`; it does not
configure a real logging service or establish PCI compliance.

The flag lives in stack-wide `metadata.labels`, so local and CI runs use the same checked-in
value. It is an Atmos label, not a Terraform variable or a cloud resource tag. Use an unquoted
boolean: `true` and the string `"true"` are different values.

Conditions can also read labels inherited from earlier imports. The current manifest's labels
take precedence, and flow into nested imports. Later imports do not change decisions already
made. Import conditions use literal string and boolean labels; templates, YAML functions, and
component-specific labels are unavailable at this stage. Labels must be readable as YAML before template rendering.
