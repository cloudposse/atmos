- Specify the path to the schema file used for validating the component configuration in the given stack, supporting schema types like jsonschema or opa.

```bash
 $ atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type <jsonschema|opa>
```shell

- Validate the specified component configuration in the given stack using the provided schema file path and schema type (`jsonschema` or `opa`).

```bash
 $ atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type <jsonschema|opa>
```

- Specify the paths to OPA policy modules or catalogs used for validating the component configuration in the given stack.

```bash
 $ atmos validate component <component> -s <stack> --schema-path <schema_path> --schema-type opa --module-paths catalog
```shell

- Specify validation timeout in seconds

```bash
 $ atmos validate component <component> -s <stack> --timeout 15
```
