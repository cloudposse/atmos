# {{ .TemplateName | default "Atmos Scaffold Template" }}

{{ .TemplateDescription | default "This is an Atmos scaffold template for managing infrastructure as code." }}

## Overview

This project uses [Atmos](https://atmos.tools/) to manage infrastructure components and stacks in a consistent, scalable way.

## Project Structure

```
.
├── components/          # Infrastructure components
│   ├── terraform/      # Terraform components
│   └── helmfile/       # Helmfile components
├── stacks/             # Stack configurations
├── workflows/          # Atmos workflows
├── atmos.yaml          # Atmos CLI configuration
└── README.md           # This file
```

## Getting Started

1. **Install Atmos**: Follow the [installation guide](https://atmos.tools/getting-started/installation)

2. **Initialize the project**:
   ```bash
   atmos init
   ```

3. **Describe components**:
   ```bash
   atmos describe components
   ```

4. **Describe stacks**:
   ```bash
   atmos describe stacks
   ```

## Usage

### Terraform Components

```bash
# Plan a component
atmos terraform plan <component> -s <stack>

# Apply a component
atmos terraform apply <component> -s <stack>

# Destroy a component
atmos terraform destroy <component> -s <stack>
```

### Helmfile Components

```bash
# Deploy a component
atmos helmfile deploy <component> -s <stack>

# Destroy a component
atmos helmfile destroy <component> -s <stack>
```

### Workflows

```bash
# List available workflows
atmos workflow list

# Execute a workflow
atmos workflow <workflow-name> -f <workflow-file>
```

## Configuration

The main configuration is in `atmos.yaml`. Key settings:

- **Components**: Define where Terraform and Helmfile components are located
- **Stacks**: Define stack configurations and variables
- **Workflows**: Define automation workflows
- **Settings**: Configure terminal output, templating, and other options

## Documentation

- [Atmos Documentation](https://atmos.tools/)
- [CLI Commands](https://atmos.tools/cli/commands/)
- [Core Concepts](https://atmos.tools/core-concepts/)

## Support

For support and questions:
- [GitHub Issues](https://github.com/cloudposse/atmos/issues)
- [Discord Community](https://cloudposse.co/discord)
