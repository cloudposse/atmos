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
  # https://atmos.tools/reference/schemas/
  # https://atmos.tools/cli/commands/validate/stacks/
  # https://atmos.tools/quick-start/configure-validation/
  # https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json
  # https://json-schema.org/draft/2020-12/release-notes
  atmos:
    # Can also be set using 'ATMOS_SCHEMAS_ATMOS_MANIFEST' ENV var, or '--schemas-atmos-manifest' command-line arguments
    # Supports both absolute and relative paths (relative to the `base_path` setting in `atmos.yaml`)
    manifest: "stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"
```

<br/>

:::tip

For more information, refer to:

- [Quick-Start: Configure CLI](/quick-start/configure-cli)
- [Atmos CLI Configuration](/cli/configuration)

:::

<br/>

## Configure Atmos Manifests Validation

[Atmos Manifest JSON Schema](pathname:///schemas/atmos/atmos-manifest/1.0/atmos-manifest.json) can be used to validate Atmos stack manifests and provide
auto-completion in IDEs and editors.

Complete the following steps to configure Atmos manifest validation:

- Add the [Atmos Manifest JSON Schema](pathname:///schemas/atmos/atmos-manifest/1.0/atmos-manifest.json) to your repository, for example
  in  [`stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json`](https://github.com/cloudposse/atmos/blob/master/examples/quick-start/stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json)

- Configure the `schemas.atmos.manifest` section in the `atmos.yaml` [CLI config file](/cli/configuration) as described
  in [Atmos Manifests Validation using JSON Schema](/reference/schemas)

  ```yaml title="atmos.yaml"
  # Validation schemas (for validating atmos stacks and components)
  schemas:
    atmos:
      # Can also be set using 'ATMOS_SCHEMAS_ATMOS_MANIFEST' ENV var, or '--schemas-atmos-manifest' command-line arguments
      # Supports both absolute and relative paths (relative to the `base_path` setting in `atmos.yaml`)
      manifest: "stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"
  ```

- Execute the command [`atmos validate stacks`](/cli/commands/validate/stacks)

- Instead of configuring the `schemas.atmos.manifest` section in `atmos.yaml`, you can provide the path to
  the [Atmos Manifest JSON Schema](pathname:///schemas/atmos/atmos-manifest/1.0/atmos-manifest.json) file by using the ENV
  variable `ATMOS_SCHEMAS_ATMOS_MANIFEST` or the `--schemas-atmos-manifest` command line argument:

  ```shell
  ATMOS_SCHEMAS_ATMOS_MANIFEST=stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json atmos validate stacks
  atmos validate stacks --schemas-atmos-manifest stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json
  ```

<br/>

:::tip

For more information, refer to:

- [Atmos Manifests Validation using JSON Schema](/reference/schemas)
- [atmos validate stacks](/cli/commands/validate/stacks)

:::

<br/>

## Configure Atmos Components Validation

Atmos component validation allows:

* Validate component config (`vars`, `settings`, `backend`, `env`, `overrides` and other sections) using [JSON Schema](https://json-schema.org/)

* Check if the component config (including relations between different component variables) is correct to allow or deny component provisioning using
  [OPA Policies](https://www.openpolicyagent.org/)

To configure Atmos components validation, complete the steps described in [Atmos Components Validation](/core-concepts/components/validation).

<br/>

:::tip

For more information, refer to:

- [Atmos Components Validation](/core-concepts/components/validation)
- [atmos validate component](/cli/commands/validate/component)

:::

<br/>
