---
title: Configure Validation
sidebar_position: 7
sidebar_label: Configure Validation
---

Atmos supports [Atmos Manifests Validation](/reference/schemas) and [Atmos Components Validation](/core-concepts/components/validation)
using [JSON Schema](https://json-schema.org/) and [OPA Policies](https://www.openpolicyagent.org/).

:::tip

This Quick Start guide describes the steps to configure and provision the infrastructure
from the [Quick Start](https://github.com/cloudposse/atmos/tree/master/examples/quick-start) repository.

You can clone the repository and modify to your own needs. The repository will help you understand the validation configurations for
Atmos manifests and components.

:::

<br/>

Configuring validation for Atmos manifests and components consists of the three steps:

- Configure validation schemas in `atmos.yaml`
- Configure Atmos manifests validation
- Configure Atmos components validation

## Configure Validation Schemas in `atmos.yaml`

In `atmos.yaml` CLI config file, add the `schemas` section as shown below:

```yaml title="atmos.yaml"
# Validation schemas (for validating atmos stacks and components)
schemas:
  # https://json-schema.org
  jsonschema:
    # Can also be set using 'ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH' ENV var, or '--schemas-jsonschema-dir' command-line arguments
    # Supports both absolute and relative paths
    base_path: "stacks/schemas/jsonschema"
  # https://www.openpolicyagent.org
  opa:
    # Can also be set using 'ATMOS_SCHEMAS_OPA_BASE_PATH' ENV var, or '--schemas-opa-dir' command-line arguments
    # Supports both absolute and relative paths
    base_path: "stacks/schemas/opa"
  # JSON Schema to validate Atmos manifests
  # https://www.schemastore.org/json
  # https://json-schema.org/draft/2020-12/release-notes
  # https://github.com/SchemaStore/schemastore
  atmos:
    # Can also be set using 'ATMOS_SCHEMAS_ATMOS_MANIFEST' ENV var, or '--schemas-atmos-manifest' command-line arguments
    # Supports both absolute and relative paths (relative to the `base_path` setting in `atmos.yaml`)
    manifest: "schemas/atmos-manifest/1.0/atmos-manifest.json"
```

<br/>

:::note

For more information, refer to:

- [Quick-Start: Configure CLI](/quick-start/configure-cli)
- [Atmos CLI Configuration](/cli/configuration)

:::

<br/>

## Configure Atmos Manifests Validation

## Configure Atmos Components Validation

