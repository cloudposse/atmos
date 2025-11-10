---
slug: yaml-function-circular-dependency-detection
title: "Atmos Now Detects Circular Dependencies in YAML Functions"
authors: [atmos]
tags: [enhancement, bugfix]
---

Atmos now detects circular dependencies in YAML function calls and provides a clear call stack showing exactly where the cycle occurs.

<!--truncate-->

## What Changed

Previously, circular dependencies in YAML functions like `!terraform.state` and `!terraform.output` would cause stack overflow panics with cryptic error messages. Now Atmos detects these cycles before they cause problems and shows you exactly where the circular dependency exists.

## Why This Matters

Circular dependencies can easily occur when components reference each other:

```yaml
# Component A references Component B
vars:
  vpc_id: !terraform.state vpc core vpc_id

# Component B references Component A
vars:
  transit_gateway_id: !terraform.state transit-gateway core tgw_id
```

## How It Works

When Atmos encounters a circular dependency, it now provides a detailed error message with the full dependency chain:

```
circular dependency detected

Dependency chain:
  1. Component 'vpc' in stack 'core'
     → !terraform.state transit-gateway core transit_gateway_id
  2. Component 'transit-gateway' in stack 'core'
     → !terraform.state vpc core vpc_id
  3. Component 'vpc' in stack 'core' (cycle detected)
     → !terraform.state transit-gateway core transit_gateway_id

To fix this issue:
  - Review your component dependencies and break the circular reference
  - Consider using Terraform data sources or direct remote state instead
  - Ensure dependencies flow in one direction only
```

## Performance Impact

The cycle detection adds negligible overhead (less than 0.001% of execution time) and uses goroutine-local storage to ensure thread safety.

## Get Involved

- [GitHub Pull Request](https://github.com/cloudposse/atmos/pull/1708)
- Share your feedback in the [Atmos Community Slack](https://cloudposse.com/slack)
