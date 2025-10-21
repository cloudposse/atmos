# Helmfile Demo

This example demonstrates how to use Atmos with Helmfile to manage Kubernetes applications.

## Prerequisites

- Kubernetes cluster (local or remote)
- kubectl configured
- Helm 3.x
- Helmfile

## Structure

- **components/helmfile/**: Contains Helmfile components
- **stacks/**: Contains stack configurations for different environments

## Usage

1. **List available components**:
   ```bash
   atmos describe components --type helmfile
   ```

2. **Describe a component**:
   ```bash
   atmos describe component nginx --type helmfile -s dev-tenant
   ```

3. **Deploy a component**:
   ```bash
   atmos helmfile deploy nginx -s dev-tenant
   ```

4. **Destroy a component**:
   ```bash
   atmos helmfile destroy nginx -s dev-tenant
   ```

## Key Features

- **Multi-environment**: Different configurations for dev, staging, prod
- **Component Reuse**: Same Helmfile components across environments
- **Variable Substitution**: Environment-specific values
- **Dependency Management**: Automatic chart dependency resolution

## Benefits

- **Declarative**: Define desired state in YAML
- **Versioned**: Pin chart versions for reproducibility
- **Scalable**: Manage multiple environments consistently
- **Integrated**: Works seamlessly with Atmos workflows 