# JSON Schema Validation Reference for Atmos

## Overview

Atmos supports JSON Schema validation (draft 2020-12) for validating component configurations.
JSON Schema is ideal for structural validation: ensuring required fields exist, values have
correct types, strings match patterns, and numbers fall within ranges.

## Configuration

### Schema Base Path in atmos.yaml

```yaml
# atmos.yaml
schemas:
  jsonschema:
    # Supports absolute and relative paths
    # Can be set via ATMOS_SCHEMAS_JSONSCHEMA_BASE_PATH env var
    # or --schemas-jsonschema-dir command-line argument
    base_path: "stacks/schemas/jsonschema"
```

### Referencing Schemas in Component Settings

```yaml
components:
  terraform:
    vpc:
      settings:
        validation:
          validate-vpc-jsonschema:
            schema_type: jsonschema
            # Path relative to schemas.jsonschema.base_path
            schema_path: "vpc/validate-vpc-component.json"
            description: Validate VPC component variables using JSON Schema
            # Optional: disable this validation step
            disabled: false
```

### Command-Line Validation

```shell
# Validate using schema defined in settings.validation
atmos validate component vpc -s plat-ue2-prod

# Validate with explicit schema path
atmos validate component vpc -s plat-ue2-prod \
  --schema-path vpc/validate-vpc-component.json \
  --schema-type jsonschema
```

## Schema File Structure

Schema files are standard JSON Schema documents. The input document is the complete component
configuration as returned by `atmos describe component`, so you validate against the full
structure including `vars`, `settings`, `env`, `backend`, etc.

### Basic Schema Template

```json
{
  "$id": "component-name",
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "Component validation",
  "description": "JSON Schema for the component.",
  "type": "object",
  "properties": {
    "vars": {
      "type": "object",
      "properties": {},
      "required": [],
      "additionalProperties": true
    }
  }
}
```

### Complete VPC Example

```json
{
  "$id": "vpc-component",
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "vpc component validation",
  "description": "JSON Schema for the 'vpc' Atmos component.",
  "type": "object",
  "properties": {
    "vars": {
      "type": "object",
      "properties": {
        "region": {
          "type": "string"
        },
        "ipv4_primary_cidr_block": {
          "type": "string",
          "pattern": "^([0-9]{1,3}\\.){3}[0-9]{1,3}(/([0-9]|[1-2][0-9]|3[0-2]))?$"
        },
        "map_public_ip_on_launch": {
          "type": "boolean"
        },
        "max_subnet_count": {
          "type": "integer",
          "minimum": 1,
          "maximum": 6
        },
        "availability_zones": {
          "type": "array",
          "items": {
            "type": "string"
          },
          "minItems": 1,
          "maxItems": 6
        },
        "name": {
          "type": "string",
          "minLength": 2,
          "maxLength": 64,
          "pattern": "^[a-z][a-z0-9-]*[a-z0-9]$"
        },
        "enabled": {
          "type": "boolean"
        },
        "tags": {
          "type": "object",
          "additionalProperties": {
            "type": "string"
          }
        }
      },
      "additionalProperties": true,
      "required": [
        "region",
        "ipv4_primary_cidr_block",
        "map_public_ip_on_launch"
      ]
    }
  }
}
```

## JSON Schema Features

### Type Validation

```json
{
  "properties": {
    "count": { "type": "integer" },
    "name": { "type": "string" },
    "enabled": { "type": "boolean" },
    "ratio": { "type": "number" },
    "tags": { "type": "object" },
    "subnets": { "type": "array" }
  }
}
```

### Required Fields

```json
{
  "required": ["region", "name", "enabled"]
}
```

### String Constraints

```json
{
  "name": {
    "type": "string",
    "minLength": 3,
    "maxLength": 64,
    "pattern": "^[a-z][a-z0-9-]+$"
  },
  "environment": {
    "type": "string",
    "enum": ["dev", "staging", "prod"]
  }
}
```

### Numeric Constraints

```json
{
  "instance_count": {
    "type": "integer",
    "minimum": 1,
    "maximum": 100,
    "multipleOf": 1
  },
  "disk_size_gb": {
    "type": "number",
    "minimum": 10,
    "exclusiveMaximum": 1000
  }
}
```

### Array Constraints

```json
{
  "availability_zones": {
    "type": "array",
    "items": {
      "type": "string",
      "pattern": "^[a-z]{2}-[a-z]+-[0-9][a-z]$"
    },
    "minItems": 1,
    "maxItems": 6,
    "uniqueItems": true
  }
}
```

### Object Constraints

```json
{
  "tags": {
    "type": "object",
    "properties": {
      "Environment": {
        "type": "string",
        "enum": ["dev", "staging", "prod"]
      },
      "Team": {
        "type": "string"
      }
    },
    "required": ["Environment", "Team"],
    "additionalProperties": {
      "type": "string"
    }
  }
}
```

### Conditional Validation

Use `if/then/else` for conditional rules:

```json
{
  "if": {
    "properties": {
      "vars": {
        "properties": {
          "stage": { "const": "prod" }
        }
      }
    }
  },
  "then": {
    "properties": {
      "vars": {
        "properties": {
          "min_size": { "minimum": 2 },
          "encryption_enabled": { "const": true }
        },
        "required": ["min_size", "encryption_enabled"]
      }
    }
  }
}
```

### Pattern Properties

Validate map keys that match a pattern:

```json
{
  "tags": {
    "type": "object",
    "patternProperties": {
      "^[A-Z][a-zA-Z]+$": {
        "type": "string",
        "minLength": 1
      }
    }
  }
}
```

## File Organization

Recommended directory structure:

```text
stacks/
  schemas/
    jsonschema/
      vpc/
        validate-vpc-component.json
      eks/
        validate-eks-component.json
      rds/
        validate-rds-component.json
      common/
        validate-tags.json
```

## Schema Composition

### Using `$ref` for Reusable Definitions

```json
{
  "$id": "eks-component",
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$defs": {
    "tags": {
      "type": "object",
      "properties": {
        "Environment": { "type": "string" },
        "Team": { "type": "string" }
      },
      "required": ["Environment", "Team"]
    }
  },
  "properties": {
    "vars": {
      "properties": {
        "tags": { "$ref": "#/$defs/tags" }
      }
    }
  }
}
```

### Using `allOf` for Combined Validation

```json
{
  "allOf": [
    {
      "properties": {
        "vars": {
          "required": ["region", "name"]
        }
      }
    },
    {
      "properties": {
        "vars": {
          "properties": {
            "name": {
              "pattern": "^[a-z][a-z0-9-]+$"
            }
          }
        }
      }
    }
  ]
}
```

## When to Use JSON Schema vs. OPA

| Use Case | JSON Schema | OPA |
|----------|------------|-----|
| Required fields | Yes | Possible but verbose |
| Type checking | Yes | Possible but verbose |
| String patterns | Yes | Yes |
| Numeric ranges | Yes | Yes |
| Cross-field validation | Limited (`if/then`) | Yes (natural) |
| Environment-specific rules | Limited | Yes (natural) |
| Complex business logic | No | Yes |
| Command-aware policies | No | Yes |
| Reusable constants/modules | Limited (`$ref`) | Yes |

**Recommendation:** Use JSON Schema for structural validation (types, required fields, patterns)
and OPA for business logic and environment-specific rules. They complement each other and can
both be applied to the same component.

## Best Practices

1. Always set `"additionalProperties": true` on `vars` to allow variables not covered by the schema
2. Use `pattern` for string format validation (CIDRs, ARNs, naming conventions)
3. Use `enum` for fields with a fixed set of allowed values
4. Use `$defs` and `$ref` to share common definitions across schemas
5. Start with required fields and basic types, then add constraints incrementally
6. Keep schemas in the same repository as stack configurations for version control
7. Test schemas with `atmos validate component` during development
