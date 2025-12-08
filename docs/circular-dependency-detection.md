# Circular Dependency Detection in Atmos

## Overview

Atmos now automatically detects circular dependencies in YAML functions and template functions, preventing infinite recursion and stack overflow errors.

## How It Works

The cycle detection system uses a goroutine-local resolution context that tracks all component dependencies during YAML function resolution. When a circular dependency is detected, Atmos immediately stops processing and returns a detailed error message.

## Protected Functions

The following functions are automatically protected against circular dependencies:

- **`!terraform.state`** - Reading Terraform state from backends
- **`!terraform.output`** - Executing Terraform output commands
- **`!store.get`** - Retrieving values from external stores
- **`!store`** - Store function calls
- **`atmos.Component()`** - Template function for component lookups

## Error Messages

When a circular dependency is detected, you'll see an error like this:

```
Error: circular dependency detected in identity chain

Dependency chain:
  1. Component 'vpc' in stack 'core'
     → !terraform.state vpc staging attachment_ids
  2. Component 'vpc' in stack 'staging'
     → !terraform.state vpc core transit_gateway_id
  3. Component 'vpc' in stack 'core' (cycle detected)
     → !terraform.state vpc staging attachment_ids

To fix this issue:
  - Review your component dependencies and break the circular reference
  - Consider using Terraform data sources or direct remote state instead
  - Ensure dependencies flow in one direction only
```

## Common Circular Dependency Patterns

### Direct Circular Dependency

Two components that depend on each other:

```yaml
# stacks/core.yaml
components:
  terraform:
    vpc:
      vars:
        transit_gateway_attachments: !terraform.state vpc staging attachment_ids

# stacks/staging.yaml
components:
  terraform:
    vpc:
      vars:
        transit_gateway_id: !terraform.state vpc core transit_gateway_id
```

**Problem**: Core depends on Staging, Staging depends on Core → Cycle!

### Indirect Circular Dependency

Multiple components forming a dependency cycle:

```yaml
# Component A depends on Component B
component-a:
  vars:
    dependency: !terraform.state component-b stack-b value

# Component B depends on Component C
component-b:
  vars:
    dependency: !terraform.state component-c stack-c value

# Component C depends on Component A
component-c:
  vars:
    dependency: !terraform.state component-a stack-a value
```

**Problem**: A → B → C → A creates a cycle!

### Mixed Function Circular Dependency

Cycles can occur across different function types:

```yaml
# Component A uses !terraform.state
component-a:
  vars:
    output: !terraform.state component-b stack-b value

# Component B uses atmos.Component() in templates
component-b:
  vars:
    config: '{{ (atmos.Component "component-a" "stack-a").outputs.value }}'
```

**Problem**: A → B → A cycle across different function types!

## How to Fix Circular Dependencies

### 1. Identify the Dependency Chain

Look at the error message to understand the full dependency chain. The error shows each step in the cycle.

### 2. Break the Cycle

Choose one of these strategies:

**Option A: Use Terraform Data Sources**

Instead of using `!terraform.state` bidirectionally, use Terraform data sources in one direction:

```hcl
# In the Terraform component code
data "terraform_remote_state" "core_vpc" {
  backend = "s3"
  config = {
    bucket = "my-terraform-state"
    key    = "core/vpc/terraform.tfstate"
  }
}
```

**Option B: Introduce an Intermediate Component**

Create a shared component that both depend on:

```yaml
# stacks/shared.yaml
components:
  terraform:
    transit-gateway:
      vars:
        # Core configuration

# stacks/core.yaml
components:
  terraform:
    vpc:
      vars:
        transit_gateway_id: !terraform.state transit-gateway shared tgw_id

# stacks/staging.yaml
components:
  terraform:
    vpc:
      vars:
        transit_gateway_id: !terraform.state transit-gateway shared tgw_id
```

**Option C: Restructure Dependencies**

Ensure dependencies flow in one direction only. Dependencies should form a DAG (Directed Acyclic Graph):

```
Shared Services → Core Infrastructure → Application Stacks
```

### 3. Apply Dependencies in Order

Use `atmos workflow` to apply components in the correct order:

```yaml
# workflows/vpc-setup.yaml
name: vpc-setup
workflows:
  deploy:
    steps:
      - command: terraform apply
        component: transit-gateway
        stack: shared
      - command: terraform apply
        component: vpc
        stack: core
      - command: terraform apply
        component: vpc
        stack: staging
```

## Architecture

The cycle detection uses:

1. **Goroutine-Local Storage**: Each goroutine maintains its own resolution context
2. **Call Stack Tracking**: Records each component/stack being resolved
3. **Visited Set**: O(1) lookup to detect when revisiting a component
4. **Automatic Cleanup**: Resolution context is cleared after processing completes

## For Developers

### Adding Cycle Detection to New Functions

To add cycle detection to a new YAML or template function:

```go
func processTagMyFunction(
    atmosConfig *schema.AtmosConfiguration,
    input string,
    currentStack string,
    resolutionCtx *ResolutionContext,
) any {
    // ... parse input ...

    // Add cycle detection
    if resolutionCtx != nil {
        node := DependencyNode{
            Component:    component,
            Stack:        stack,
            FunctionType: "my.function",
            FunctionCall: input,
        }

        if err := resolutionCtx.Push(atmosConfig, node); err != nil {
            errUtils.CheckErrorPrintAndExit(err, "", "")
        }
        defer resolutionCtx.Pop(atmosConfig)
    }

    // ... execute function logic ...
}
```

### Testing Cycle Detection

See `internal/exec/yaml_func_circular_deps_test.go` for comprehensive test examples.

## References

- [PRD: Circular Dependency Detection](prd/circular-dependency-detection.md)
- [Error Handling Strategy](prd/error-handling-strategy.md)
- [Testing Strategy](prd/testing-strategy.md)
