# Project Memory

This file provides persistent context to the Atmos AI Assistant about this project.

## Project Overview

This is an example project demonstrating Atmos AI features. It contains:
- A sample application component (`myapp`)
- Development and production stacks
- Multi-provider AI configuration

## Stack Naming Convention

Stacks follow the pattern: `{environment}-{stage}`
- `dev-main` - Development environment, main stage
- `prod-main` - Production environment, main stage

## Component Patterns

### myapp Component
The `myapp` component is a sample Terraform module that demonstrates:
- Environment-specific configuration
- Variable inheritance from stacks
- Output values for AI inspection

Configuration differences by environment:
- **dev**: Smaller instance types, single replica, debug enabled
- **prod**: Production instance types, multiple replicas, optimized settings

## Common Operations

### Describe a component
```bash
atmos describe component myapp -s dev-main
```

### List all stacks
```bash
atmos list stacks
```

### Validate configuration
```bash
atmos validate stacks
```

## Team Preferences

- Use Terraform for infrastructure components
- Follow environment-specific sizing (dev = small, prod = production-grade)
- Enable detailed logging in dev, minimal logging in prod
- All resources should have environment and stage tags

## Important Notes

- The `myapp` component is a mock component for demonstration
- No real cloud resources are created by this example
- AI tools are read-only and safe to use
