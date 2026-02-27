# OPA Policy Reference for Atmos

## Rego Fundamentals for Atmos

All Atmos OPA policies must use the `package atmos` declaration and define `errors` rules.
Atmos evaluates all rules and collects error messages from the `errors` set. If any error
messages are present, validation fails.

### Minimal Policy Structure

```rego
# Required package declaration
package atmos

# Optional imports
import future.keywords.in

# Error rules: each rule adds a message to the errors set when its conditions are true
errors[message] {
    # conditions...
    message = "Error description"
}
```

## Input Structure

The `input` object contains the full component configuration. Key fields:

### Core Component Configuration

```
input.vars                 # Component variables (the values passed to Terraform)
input.settings             # Component settings
input.env                  # Environment variables
input.backend              # Backend configuration
input.backend_type         # Backend type (e.g., "s3")
input.metadata             # Component metadata
input.metadata.component   # The Terraform component name
input.workspace            # Terraform workspace name
```

### Context Variables

```
input.vars.namespace       # Organization namespace
input.vars.tenant          # Organizational unit
input.vars.environment     # Region/environment identifier
input.vars.stage           # Account/stage identifier
input.vars.name            # Component instance name
input.vars.tags            # Resource tags map
```

### Execution Context (available during plan/apply)

```
input.process_env          # Map of OS environment variables
input.cli_args             # List of CLI arguments (e.g., ["terraform", "plan"])
input.tf_cli_vars          # Map from -var arguments with type conversion
input.env_tf_cli_args      # List from TF_CLI_ARGS env var
input.env_tf_cli_vars      # Map from TF_CLI_ARGS -var values with type conversion
```

## Writing Deny Rules

### Simple Field Validation

```rego
package atmos

# Deny if a required variable is missing
errors[message] {
    not input.vars.region
    message = "The 'region' variable is required"
}

# Deny if a boolean is incorrectly set for the environment
errors[message] {
    input.vars.stage == "prod"
    input.vars.map_public_ip_on_launch == true
    message = "Public IP mapping on launch is not allowed in production"
}
```

### Numeric Range Validation

```rego
package atmos

errors[message] {
    input.vars.instance_count > 10
    message = sprintf("instance_count cannot exceed 10, got %d", [input.vars.instance_count])
}

errors[message] {
    input.vars.stage == "prod"
    input.vars.min_size < 2
    message = sprintf("Production requires min_size >= 2, got %d", [input.vars.min_size])
}
```

### String Pattern Validation

```rego
package atmos

# Validate naming conventions
errors[message] {
    not re_match("^[a-z][a-z0-9-]*$", input.vars.name)
    message = sprintf("Name '%s' must be lowercase alphanumeric with hyphens", [input.vars.name])
}

# Validate CIDR format
errors[message] {
    not re_match("^([0-9]{1,3}\\.){3}[0-9]{1,3}/[0-9]{1,2}$", input.vars.cidr_block)
    message = sprintf("Invalid CIDR block: '%s'", [input.vars.cidr_block])
}
```

**Note:** Backslashes in regex patterns must be double-escaped: `\\.` to match a literal dot.

### List and Array Validation

```rego
package atmos

# Validate list length
errors[message] {
    input.vars.stage == "dev"
    count(input.vars.availability_zones) != 2
    message = "Dev environment must use exactly 2 availability zones"
}

# Validate list contents
errors[message] {
    az := input.vars.availability_zones[_]
    not startswith(az, input.vars.region)
    message = sprintf("AZ '%s' does not match region '%s'", [az, input.vars.region])
}

# Check for prohibited values in a list
errors[message] {
    cidr := input.vars.allowed_cidr_blocks[_]
    cidr == "0.0.0.0/0"
    message = "Open CIDR block 0.0.0.0/0 is not allowed"
}
```

### Map and Tag Validation

```rego
package atmos

# Required tags
errors[message] {
    required := {"Environment", "Team", "CostCenter", "Project"}
    missing := required - {key | input.vars.tags[key]}
    count(missing) > 0
    message = sprintf("Missing required tags: %v", [missing])
}

# Tag value constraints
errors[message] {
    input.vars.tags.Environment
    allowed_envs := {"dev", "staging", "prod"}
    not input.vars.tags.Environment in allowed_envs
    message = sprintf("Invalid Environment tag: '%s'. Must be one of: %v",
      [input.vars.tags.Environment, allowed_envs])
}
```

## Command-Aware Policies

### Blocking Apply in Specific Conditions

```rego
package atmos

# Block apply if a variable has an unsafe value
errors[message] {
    count(input.cli_args) >= 2
    input.cli_args[0] == "terraform"
    input.cli_args[1] == "apply"
    input.vars.delete_protection == false
    input.vars.stage == "prod"
    message = "Cannot apply in prod with delete_protection disabled"
}
```

### Environment Variable Requirements

```rego
package atmos

# Require approval for production
errors[message] {
    "apply" in input.cli_args
    input.vars.stage == "prod"
    not input.process_env.DEPLOYMENT_APPROVED
    message = "Set DEPLOYMENT_APPROVED=true for production deployments"
}

# Validate AWS region matches configuration
errors[message] {
    input.process_env.AWS_REGION
    input.vars.region
    input.process_env.AWS_REGION != input.vars.region
    message = sprintf("AWS_REGION '%s' does not match configured region '%s'",
      [input.process_env.AWS_REGION, input.vars.region])
}
```

### CLI Variable Validation

```rego
package atmos

# Block sensitive variables from CLI
errors[message] {
    sensitive_vars := {"password", "secret", "api_key", "token"}
    cli_var := sensitive_vars[_]
    input.tf_cli_vars[cli_var]
    message = sprintf("Sensitive variable '%s' must not be passed via CLI", [cli_var])
}
```

## Modular Policies

### Constants Module

```rego
# stacks/schemas/opa/catalog/constants/constants.rego
package atmos.constants

max_dev_instances := 3
max_prod_instances := 50
required_tags := {"Environment", "Team", "CostCenter"}
name_regex := "^[a-z][a-z0-9-]{1,62}[a-z0-9]$"
name_error := "Name must be 3-64 chars, lowercase alphanumeric with hyphens"
```

### Using Constants in Policies

```rego
# stacks/schemas/opa/vpc/validate-vpc.rego
package atmos

import data.atmos.constants.required_tags
import data.atmos.constants.name_regex
import data.atmos.constants.name_error

errors[name_error] {
    not re_match(name_regex, input.vars.name)
}

errors[message] {
    missing := required_tags - {key | input.vars.tags[key]}
    count(missing) > 0
    message = sprintf("Missing required tags: %v", [missing])
}
```

### Helper Functions Module

```rego
# stacks/schemas/opa/catalog/helpers/helpers.rego
package atmos.helpers

is_production {
    input.vars.stage == "prod"
}

is_development {
    input.vars.stage == "dev"
}

has_tag(tag_name) {
    input.vars.tags[tag_name]
}
```

## Example Policies

### VPC Component Policy

```rego
package atmos

import future.keywords.in

# No public IPs in production
errors[message] {
    input.vars.stage == "prod"
    input.vars.map_public_ip_on_launch == true
    message = "Public IPs on launch are not allowed in production"
}

# Limit AZs in dev
errors[message] {
    input.vars.stage == "dev"
    count(input.vars.availability_zones) > 2
    message = "Dev is limited to 2 availability zones"
}

# Validate CIDR block
errors[message] {
    not re_match("^10\\.", input.vars.ipv4_primary_cidr_block)
    message = "VPC CIDR must be in the 10.0.0.0/8 range"
}

# Require flow logs in production
errors[message] {
    input.vars.stage == "prod"
    not input.vars.vpc_flow_logs_enabled
    message = "VPC flow logs are required in production"
}
```

### EKS Cluster Policy

```rego
package atmos

# Minimum node count for production
errors[message] {
    input.vars.stage == "prod"
    input.vars.min_node_count < 3
    message = sprintf("Production EKS requires min 3 nodes, got %d",
      [input.vars.min_node_count])
}

# Validate Kubernetes version
errors[message] {
    allowed_versions := {"1.28", "1.29", "1.30"}
    not input.vars.kubernetes_version in allowed_versions
    message = sprintf("Kubernetes version '%s' is not approved. Use: %v",
      [input.vars.kubernetes_version, allowed_versions])
}

# Require encryption
errors[message] {
    not input.vars.encryption_config_enabled
    message = "EKS encryption must be enabled"
}
```

### Cost Control Policy

```rego
package atmos

# Block expensive instance types in dev
errors[message] {
    input.vars.stage == "dev"
    expensive := {"m5.xlarge", "m5.2xlarge", "c5.xlarge", "c5.2xlarge",
                  "r5.xlarge", "r5.2xlarge"}
    input.vars.instance_type in expensive
    message = sprintf("Instance type '%s' is too expensive for dev",
      [input.vars.instance_type])
}

# Limit storage in non-production
errors[message] {
    input.vars.stage != "prod"
    input.vars.storage_gb > 100
    message = sprintf("Non-production storage limited to 100GB, got %d",
      [input.vars.storage_gb])
}
```

## Best Practices

1. Always use `sprintf` for dynamic error messages with variable interpolation
2. Use `import future.keywords.in` for cleaner set membership checks
3. Separate constants and helper functions into reusable modules
4. Write environment-specific rules using `input.vars.stage` or `input.vars.environment`
5. Double-escape backslashes in regex patterns (`\\.` not `\.`)
6. Test policies with `atmos validate component` before deploying to CI/CD
7. Use `input.cli_args` to create command-aware policies that only apply during plan or apply
8. Provide actionable error messages that tell the user how to fix the issue
