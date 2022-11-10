---
sidebar_position: 3
---

# Component Validation

Validation is critical to maintaining hygenic configurations in distributed team environments. 

`atmos` component validation allows:

* Validate component config (vars, settings, backend, and other sections) using JSON Schema
* Check if the component config (including relations between different component variables) is correct to allow or deny component provisioning using OPA/Rego policies

## JSON Schema

Atmos has native support for [JSON Schema](https://json-schema.org/), which can validate the schema of configurations. JSON Schema is an industry standard and provides a vocabulary to annotate and validate JSON documents for correctness.

This is powerful stuff: because you can define many schemas, it's possible to validate components differently for different environments or teams.

## Open Policy Agent (OPA)

The [Open Policy Agent](https://www.openpolicyagent.org/docs/latest/) (OPA, pronounced “oh-pa”) is another open-source industry standard that provides a general-purpose policy engine to unify policy enforcement across your stacks. The OPA language (rego) is a high-level declarative language for specifying policy as code. Atmos has native support for the OPA decision-making engine to enforce policies across all the components in your stacks (e.g. for microservice configurations).


## Usage

`atmos` `validate component` command supports `--schema-path` and `--schema-type` command line arguments.
If the arguments are not provided, `atmos` will try to find and use the `settings.validation` section defined in the component's YAML config.

```bash
atmos validate component infra/vpc -s tenant1-ue2-prod --schema-path validate-infra-vpc-component.json --schema-type jsonschema

atmos validate component infra/vpc -s tenant1-ue2-prod --schema-path validate-infra-vpc-component.rego --schema-type opa

atmos validate component infra/vpc -s tenant1-ue2-prod

atmos validate component infra/vpc -s tenant1-ue2-dev
```

### Configure Component Validation

In `atmos.yaml`, add the `schemas` config:

```yaml
# Validation schemas (for validating atmos stacks and components)
schemas:
  # https://json-schema.org
  jsonschema:
    # Can also be set using `ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH` ENV var, or `--schemas-jsonschema-dir` command-line arguments
    # Supports both absolute and relative paths
    base_path: "stacks/schemas/jsonschema"
  # https://www.openpolicyagent.org
  opa:
    # Can also be set using `ATMOS_SCHEMAS_OPA_BASE_PATH` ENV var, or `--schemas-opa-dir` command-line arguments
    # Supports both absolute and relative paths
    base_path: "stacks/schemas/opa"
```

In the component YAML config, add the `settings.validation` section:

```yaml
components:
  terraform:
    infra/vpc:
      settings:
        # Validation
        # Supports JSON Schema and OPA policies
        # All validation steps must succeed to allow the component to be provisioned
        validation:
          validate-infra-vpc-component-with-jsonschema:
            schema_type: jsonschema
            # 'schema_path' can be an absolute path or a path relative to 'schemas.jsonschema.base_path' defined in `atmos.yaml`
            schema_path: validate-infra-vpc-component.json
            description: Validate 'infra/vpc' component variables using JSON Schema
          check-infra-vpc-component-config-with-opa-policy:
            schema_type: opa
            # 'schema_path' can be an absolute path or a path relative to 'schemas.opa.base_path' defined in `atmos.yaml`
            schema_path: validate-infra-vpc-component.rego
            description: Check 'infra/vpc' component configuration using OPA policy
```

Add the following JSON Schema in the file `stacks/schemas/jsonschema/validate-infra-vpc-component.json`:

```json
{
  "$id": "infra-vpc-component",
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "infra/vpc component validation",
  "description": "JSON Schema for infra/vpc atmos component.",
  "type": "object",
  "properties": {
    "vars": {
      "type": "object",
      "properties": {
        "region": {
          "type": "string"
        },
        "cidr_block": {
          "type": "string",
          "pattern": "^([0-9]{1,3}\\.){3}[0-9]{1,3}(/([0-9]|[1-2][0-9]|3[0-2]))?$"
        },
        "map_public_ip_on_launch": {
          "type": "boolean"
        }
      },
      "additionalProperties": true,
      "required": [
        "region",
        "cidr_block",
        "map_public_ip_on_launch"
      ]
    }
  }
}
```

Add the following OPA policy in the file `stacks/schemas/opa/validate-infra-vpc-component.rego`:

```rego
# 'atmos' looks for the 'errors' (array of strings) output from all OPA policies
# If the 'errors' output contains one or more error messages, 'atmos' considers the policy failed

# 'package atmos' is required in all `atmos` OPA policies
package atmos

# In production, don't allow mapping public IPs on launch
errors[message] {
  input.vars.stage == "prod"
  input.vars.map_public_ip_on_launch == true
  message = "Mapping public IPs on launch is not allowed in 'prod'. Set 'map_public_ip_on_launch' variable to 'false'"
}

# In 'dev', only 2 Availability Zones are allowed
  errors[message] {
  input.vars.stage == "dev"
  count(input.vars.availability_zones) != 2
  message = "In 'dev', only 2 Availability Zones are allowed"
}
```

Run the following commands to validate the component in the stacks:

```bash
> atmos validate component infra/vpc -s tenant1-ue2-prod

Check 'infra/vpc' component configuration using OPA policy
Mapping public IPs on launch is not allowed in 'prod'. Set 'map_public_ip_on_launch' variable to 'false'

exit status 1
```

```bash
> atmos validate component infra/vpc -s tenant1-ue2-dev

Check 'infra/vpc' component configuration using OPA policy
In 'dev', only 2 Availability Zones are allowed

exit status 1
```

```bash
> atmos validate component infra/vpc -s tenant1-ue2-staging

Validate 'infra/vpc' component variables using JSON Schema
{
  "valid": false,
  "errors": [
    {
      "keywordLocation": "",
      "absoluteKeywordLocation": "file:///examples/complete/stacks/schemas/jsonschema/infra-vpc-component#",
      "instanceLocation": "",
      "error": "doesn't validate with file:///examples/complete/stacks/schemas/jsonschema/infra-vpc-component#"
    },
    {
      "keywordLocation": "/properties/vars/properties/cidr_block/pattern",
      "absoluteKeywordLocation": "file:///examples/complete/stacks/schemas/jsonschema/infra-vpc-component#/properties/vars/properties/cidr_block/pattern",
      "instanceLocation": "/vars/cidr_block",
      "error": "does not match pattern '^([0-9]{1,3}\\\\.){3}[0-9]{1,3}(/([0-9]|[1-2][0-9]|3[0-2]))?$'"
    }
  ]
}

exit status 1
```

Run the following commands to provision the component in the stacks:

```bash
atmos terraform apply infra/vpc -s tenant1-ue2-prod
atmos terraform apply infra/vpc -s tenant1-ue2-dev
```

Since the OPA validation policies don't pass, `atmos` does not allow provisioning the component in the stacks:


![atmos-validate-infra-vpc-in-tenant1-ue2-prod](/img/atmos-validate-infra-vpc-in-tenant1-ue2-dev.png)
![atmos-validate-infra-vpc-in-tenant1-ue2-dev](/img/atmos-validate-infra-vpc-in-tenant1-ue2-dev.png)




