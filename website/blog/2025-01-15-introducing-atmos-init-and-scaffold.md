---
slug: introducing-atmos-init-and-scaffold
title: "Introducing atmos init and atmos scaffold: Get Started in Seconds"
authors: [atmos]
tags: [feature, enhancement, atmos-core]
---

We're excited to announce two new commands that dramatically simplify getting started with Atmos: `atmos init` and `atmos scaffold`. These commands eliminate the manual setup process and help you bootstrap new Atmos projects or generate infrastructure code in seconds.

<!--truncate-->

## The Problem: Setup Friction

Setting up a new Atmos project has traditionally required several manual steps:

1. Creating the project directory structure (`components/`, `stacks/`, `schemas/`)
2. Writing a complete `atmos.yaml` configuration file
3. Understanding the recommended patterns and conventions
4. Setting up initial stack files and component configurations

For new users, this process could take hours and required deep knowledge of Atmos conventions. Even experienced users found themselves repeatedly creating similar boilerplate for new projects or infrastructure components.

## The Solution: Instant Project Creation

### `atmos init`: Bootstrap New Projects

The `atmos init` command creates a complete Atmos project from built-in templates with a single command:

```bash
# Interactive mode - guided setup with prompts
$ atmos init

? Select a template:
  ❯ simple - Basic Atmos project structure
    atmos - Complete Atmos project with full configuration

? Enter project name: my-infrastructure
? Enter Terraform version: 1.5.0
? Enter default AWS region: us-west-2
? Enter target directory: ./my-infrastructure

Initializing my-infrastructure in ./my-infrastructure

  ✓ atmos.yaml
  ✓ README.md
  ✓ stacks/.gitkeep
  ✓ components/terraform/.gitkeep

Initialized 4 files.
```

For automation and CI/CD, use non-interactive mode:

```bash
$ atmos init atmos ./my-project \
  --set project_name=my-infra \
  --set terraform_version=1.5.0 \
  --set aws_region=us-east-1 \
  --no-interactive
```

### `atmos scaffold`: Generate Infrastructure Code

The `atmos scaffold` commands help you generate infrastructure code from templates:

```bash
# Generate from a local scaffold template
$ atmos scaffold generate vpc-component ./components/terraform/vpc \
  --set vpc_name=main \
  --set cidr_block=10.0.0.0/16

Generating vpc-component in ./components/terraform/vpc

  ✓ main.tf
  ✓ variables.tf
  ✓ outputs.tf
  ✓ versions.tf

Initialized 4 files.
```

List available scaffold templates:

```bash
$ atmos scaffold list

Available scaffold templates:

Name              Source                    Version    Description
────────────────  ────────────────────────  ─────────  ──────────────────────────────
vpc-component     ./scaffolds/vpc           1.0.0      AWS VPC component template
eks-cluster       ./scaffolds/eks           2.1.0      EKS cluster template
rds-instance      github.com/acme/rds.git   1.5.0      RDS database template
```

## Key Features

### Built-in Templates

The `atmos init` command includes two carefully crafted templates:

**simple**: Perfect for getting started quickly
- Minimal `atmos.yaml` configuration
- Basic directory structure
- Essential `.gitkeep` files

**atmos**: Complete project setup
- Full `atmos.yaml` with all sections
- Directory structure for components, stacks, schemas, and workflows
- Configured for multiple environments
- Backend configuration examples

### Powerful Templating

Both commands use Go templates with Gomplate functions, supporting:

- **Conditional file generation**: `{{if .Config.enable_monitoring}}file.yaml{{end}}`
- **Dynamic paths**: `{{.Config.namespace}}/config.yaml`
- **Content templating**: `project: {{.Config.project_name}}`
- **Rich functions**: `upper`, `lower`, `title`, `default`, and 200+ Gomplate functions

### Interactive and Automated Workflows

**Interactive Mode** (default):
- Guided prompts for template selection
- User-friendly questions for configuration values
- Preview of what will be created
- Safe defaults for all values

**Non-Interactive Mode** (automation):
- Pass all values via `--set` flags
- Perfect for CI/CD pipelines
- Reproducible project creation
- Scriptable workflows

### Extensible Scaffold System

Configure custom scaffold templates in `atmos.yaml`:

```yaml
scaffold:
  base_path: "./scaffolds"

  templates:
    vpc-component:
      description: "AWS VPC component template"
      source: "./scaffolds/vpc"
      version: "1.0.0"

    eks-cluster:
      description: "EKS cluster template"
      source: "github.com/acme/atmos-scaffolds/eks.git"
      version: "2.1.0"
      ref: "tags/v2.1.0"
```

## Use Cases

### 1. Onboarding New Team Members

New developers can have a working Atmos project in under 2 minutes:

```bash
$ atmos init simple ./my-first-project
$ cd my-first-project
$ # Start adding components and stacks
```

### 2. Starting New Infrastructure Projects

Bootstrap production-ready projects with complete configurations:

```bash
$ atmos init atmos ./prod-infrastructure \
  --set project_name=acme-production \
  --set aws_region=us-east-1 \
  --set terraform_version=1.6.0
```

### 3. Generating Repetitive Components

Create similar infrastructure components without copy-paste:

```bash
# Generate VPC for dev environment
$ atmos scaffold generate vpc-component ./components/terraform/vpc-dev \
  --set vpc_name=dev \
  --set cidr_block=10.0.0.0/16

# Generate VPC for prod environment
$ atmos scaffold generate vpc-component ./components/terraform/vpc-prod \
  --set vpc_name=prod \
  --set cidr_block=10.1.0.0/16
```

### 4. Organization-Wide Standardization

Create organization-specific scaffold templates that encode your team's best practices:

```bash
# Team members use your custom scaffolds
$ atmos scaffold generate acme-microservice ./services/api \
  --set service_name=user-api \
  --set team=platform
```

## Creating Custom Scaffold Templates

Scaffold templates are simple directories with a `scaffold.yaml` file:

```yaml
# scaffolds/vpc/scaffold.yaml
name: "vpc-component"
description: "AWS VPC component template"
author: "Cloud Posse"
version: "1.0.0"

prompts:
  - name: "vpc_name"
    description: "VPC name"
    type: "input"
    default: "main"

  - name: "cidr_block"
    description: "CIDR block"
    type: "input"
    default: "10.0.0.0/16"
```

Template files support Go templates:

```hcl
# scaffolds/vpc/main.tf
module "vpc" {
  source  = "cloudposse/vpc/aws"
  version = "2.1.0"

  name       = "{{.Config.vpc_name}}"
  cidr_block = "{{.Config.cidr_block}}"

  tags = {
    Name = "{{.Config.vpc_name}}"
  }
}
```

## Technical Highlights

### For Atmos Contributors

These commands represent significant architectural improvements:

1. **Command Registry Pattern**: Both commands use the new command registry pattern, making them independently testable and maintainable

2. **Shared Core Packages**:
   - `pkg/init/ui` - Interactive UI components and prompts
   - `pkg/scaffold/templating` - Template processing engine
   - `pkg/init/config` - Scaffold configuration parsing

3. **Embedded Templates**: Built-in templates are embedded in the Atmos binary, ensuring version compatibility and eliminating external dependencies

4. **Comprehensive Testing**: Over 80% test coverage with unit and integration tests for all template processing logic

See the [PRD document](https://github.com/cloudposse/atmos/blob/main/docs/prd/atmos-init-and-scaffold-commands.md) for complete technical details.

## What's Next

These commands lay the foundation for future enhancements:

- **Template Marketplace**: Central registry of community scaffold templates
- **Git Integration**: Clone templates directly from GitHub/GitLab repositories
- **Template Validation**: Schema validation for scaffold.yaml files
- **Template Composition**: Combine multiple templates into complex projects

## Get Started Today

The `atmos init` and `atmos scaffold` commands are available in Atmos v1.97.0. Try them out:

```bash
# Install or upgrade Atmos
brew upgrade atmos

# Create your first project
atmos init

# Explore scaffold templates
atmos scaffold list
```

We'd love to hear your feedback! Join the discussion in our [GitHub Discussions](https://github.com/cloudposse/atmos/discussions) or share your custom scaffold templates with the community.

## Documentation

- [atmos init command reference](/cli/commands/init)
- [atmos scaffold command reference](/cli/commands/scaffold)
- [Creating Custom Scaffold Templates](/core-concepts/scaffold-templates)
- [PRD: Init and Scaffold Commands](https://github.com/cloudposse/atmos/blob/main/docs/prd/atmos-init-and-scaffold-commands.md)

🚀 Generated with [Atmos](https://atmos.tools)
