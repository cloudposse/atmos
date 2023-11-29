---
title: Atmos Schemas
description: Atmos Schemas
sidebar_label: Schemas
sidebar_position: 5
---

## Atmos Manifest JSON Schema

[Atmos Manifest JSON Schema](pathname://../schemas/1.0/atmos-manifest.json) can be used to validate Atmos stack manifests and provide auto-completion.

### Validate and Auto-Complete Atmos Manifests in IDEs

In supported editors like [JetBrains IDEs](https://www.jetbrains.com/), [Microsoft Visual Studio](https://visualstudio.microsoft.com/)
or [Visual Studio Code](https://code.visualstudio.com/), the schema can offer auto-completion and validation to ensure that Atmos stack manifests, and
all sections in them, are
correct.

<br/>

:::tip

A list of editors that support validation using [JSON Schema](https://json-schema.org/) can be
found [here](https://json-schema.org/implementations#editors).

:::

<br/>

### Validate Atmos Manifests on the Command Line

Atmos can use the [Atmos Manifest JSON Schema](pathname://../schemas/1.0/atmos-manifest.json) to validate Atmos stack manifests on the command line
by executing the command [`atmos validate stacks`](/cli/commands/validate/stacks).

For this to work, configure the following:

- Add the [Atmos Manifest JSON Schema](pathname://../schemas/1.0/atmos-manifest.json) to your repository, for example
  in  `schemas/1.0/atmos-manifest.json`

- Configure the following section in the `atmos.yaml` [CLI config file](/cli/configuration)

  ```yaml title="atmos.yaml"
  # Validation schemas (for validating atmos stacks and components)
  schemas:
    # JSON Schema to validate Atmos manifests
    atmos:
    # Can also be set using 'ATMOS_SCHEMAS_ATMOS_MANIFEST' ENV var, or '--schemas-atmos-manifest' command-line arguments
    # Supports both absolute and relative paths
      manifest: "schemas/1.0/atmos-manifest.json"
  ```

- Execute the command [`atmos validate stacks`](/cli/commands/validate/stacks)

- Instead of configuring the `schemas.atmos.manifest` section in `atmos.yaml`, you can provide the path to
  the [Atmos Manifest JSON Schema](pathname://../schemas/1.0/atmos-manifest.json) file by using the ENV variable `ATMOS_SCHEMAS_ATMOS_MANIFEST` or the
  `--schemas-atmos-manifest` command line argument:

  ```shell
  ATMOS_SCHEMAS_ATMOS_MANIFEST=schemas/1.0/atmos-manifest.json atmos validate stacks
  atmos validate stacks --schemas-atmos-manifest schemas/1.0/atmos-manifest.json
  ```

<br/>

## References

- https://json-schema.org
- https://json-schema.org/draft/2020-12/release-notes
- https://www.schemastore.org/json
- https://github.com/SchemaStore/schemastore
- https://www.jetbrains.com/help/idea/json.html#ws_json_using_schemas
- https://code.visualstudio.com/docs/languages/json
