---
slug: introducing-atmos-init-and-scaffold
title: "Project Setup Without the Boilerplate: atmos init and atmos scaffold"
authors: [osterman]
tags: [feature, dx]
---

A new infrastructure repository starts as an empty directory plus a page of conventions somebody has to remember: which directories exist, what belongs in the configuration file, how stacks and components are meant to line up. The first time you assemble that by hand, you spend as long reading documentation as writing files. Every time after that, you copy the last project and inherit whatever was wrong with it. The `atmos init` and `atmos scaffold` commands generate the structure for you, from built-in templates or your own.

<!--truncate-->

## The Problem

Setting up a new Atmos project has traditionally required several manual steps:

1. Creating the project directory structure (`components/`, `stacks/`, `schemas/`)
2. Writing a complete `atmos.yaml` configuration file
3. Understanding the recommended patterns and conventions
4. Setting up initial stack files and component configurations

For new users, this process could take hours and required deep knowledge of Atmos conventions. Even experienced users found themselves repeatedly creating similar boilerplate for new projects or infrastructure components.

## The Fix

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

The `atmos init` command includes two templates:

**simple**: the smallest thing that works
- Minimal `atmos.yaml` configuration
- Basic directory structure
- Essential `.gitkeep` files

**atmos**: Complete project setup
- Full `atmos.yaml` with all sections
- Directory structure for components, stacks, schemas, and workflows
- Configured for multiple environments
- Backend configuration examples

### Templating

Both commands use Go templates with Gomplate functions, supporting:

- **Conditional file generation**: `{{if .Config.enable_monitoring}}file.yaml{{end}}`
- **Dynamic paths**: `{{.Config.namespace}}/config.yaml`
- **Content templating**: `project: {{.Config.project_name}}`
- **Rich functions**: `upper`, `lower`, `title`, `default`, and 200+ Gomplate functions

### Interactive and Automated Workflows

Run either command with no values and it prompts you: first for the template, then one question per configuration value, each carrying a default, and a preview of the files it is about to create before it writes anything.

Pass every value up front with `--set` and add `--no-interactive`, and the same run happens with no prompts at all. That is the mode you want in a CI/CD pipeline or a shell script, where the same inputs have to produce the same project every time.

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
apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: vpc-component
  description: AWS VPC component template
  author: Cloud Posse
  version: 1.0.0
spec:
  fields:
    - name: vpc_name
      label: VPC name
      type: input
      default: main
    - name: cidr_block
      label: CIDR block
      type: input
      default: 10.0.0.0/16
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

## How to Use It

The `atmos init` and `atmos scaffold` commands are available in Atmos v1.97.0.

```bash
# Install or upgrade Atmos
brew upgrade atmos

# Create your first project
atmos init

# Explore scaffold templates
atmos scaffold list
```

## What's Next

These commands lay the foundation for future enhancements:

- **Template Marketplace**: Central registry of community scaffold templates
- **Git Integration**: Clone templates directly from GitHub/GitLab repositories
- **Template Validation**: Schema validation for scaffold.yaml files
- **Template Composition**: Combine multiple templates into complex projects

## Documentation

- [atmos init command reference](/cli/commands/init)
- [atmos scaffold command reference](/cli/commands/scaffold)
- [Creating Custom Scaffold Templates](/cli/commands/scaffold/generate)
- [PRD: Init Command](https://github.com/cloudposse/atmos/blob/main/docs/prd/atmos-init.md)
- [PRD: Scaffold Command](https://github.com/cloudposse/atmos/blob/main/docs/prd/atmos-scaffold.md)

## Get Involved

Tell us what your team's project layout looks like, or share a scaffold template you have built, by [opening an issue](https://github.com/cloudposse/atmos/issues).
