# PRD: Atmos Init and Scaffold Commands

## Overview

The `atmos init` and `atmos scaffold` commands provide streamlined project initialization and code generation capabilities for Atmos users. These commands enable users to quickly bootstrap new Atmos projects with best-practice configurations and generate infrastructure code from reusable templates.

## Problem Statement

Users face several challenges when starting new Atmos projects or creating new infrastructure components:

1. **Manual Configuration**: Setting up a new Atmos project requires creating multiple configuration files (`atmos.yaml`, stack files, component directories) with correct structure and defaults
2. **Learning Curve**: New users don't know the recommended directory structure or configuration patterns
3. **Repetitive Tasks**: Creating similar infrastructure components (VPCs, EKS clusters, etc.) requires copying and modifying boilerplate code
4. **Inconsistency**: Without templates, different team members create projects with varying structures and conventions
5. **Time-Consuming Setup**: Manual project setup can take hours, especially for complex multi-environment configurations

## Goals

### Primary Goals

1. **Instant Project Creation**: Enable users to initialize a complete Atmos project in seconds
2. **Best Practices by Default**: Ensure generated projects follow Cloud Posse recommendations
3. **Interactive and Non-Interactive Modes**: Support both CLI automation and guided setup
4. **Template Ecosystem**: Provide extensible templating system for custom project structures
5. **Developer Experience**: Make it trivial to get started with Atmos

### Non-Goals

1. **Terraform Module Generation**: Not generating actual Terraform module code (that's terraform-null-label, etc.)
2. **Cloud Resource Provisioning**: Not creating actual cloud infrastructure
3. **Migration Tools**: Not migrating existing projects to Atmos structure

## Solution

### Architecture

The solution consists of two main commands with distinct but complementary purposes:

#### 1. `atmos init` - Project Initialization

Bootstraps new Atmos projects from built-in templates embedded in the Atmos binary.

**Key Characteristics:**
- Built-in templates bundled with Atmos releases
- Focused on project structure and `atmos.yaml` configuration
- Versioned with Atmos (templates match Atmos version capabilities)
- Minimal, focused templates for quick starts

#### 2. `atmos scaffold` - Code Generation

Generates infrastructure code and configurations from templates (local or remote).

**Key Characteristics:**
- Supports custom templates from local filesystem or Git repositories
- Configurable via `atmos.yaml` for organization-specific patterns
- Supports complex templating with Go templates and Gomplate
- Designed for repetitive code generation tasks

### Implementation Details

#### Command Structure

Both commands follow the Atmos command registry pattern:

```
cmd/
├── init/
│   └── init.go          # Init command implementation
├── scaffold/
│   └── scaffold.go      # Scaffold parent + subcommands
└── internal/
    ├── command.go       # CommandProvider interface
    └── registry.go      # Command registration
```

#### Core Packages

```
pkg/
├── init/
│   ├── embeds/         # Embedded template loader
│   ├── config/         # Scaffold configuration parsing
│   ├── ui/             # Interactive UI components
│   └── templates/      # Built-in init templates
└── scaffold/
    └── templating/     # Template processing engine
```

#### Template System

**Template Discovery:**
1. **Built-in Templates** (init): Embedded in binary at `pkg/init/templates/`
2. **Configured Templates** (scaffold): Defined in `atmos.yaml` under `scaffold.templates`
3. **Local Templates** (scaffold): Filesystem paths to template directories
4. **Remote Templates** (scaffold): Git repositories with templates

**Template Structure:**
```
template-name/
├── scaffold.yaml      # Template metadata and prompts
├── README.md          # Template documentation
└── files...           # Template files (support Go templates)
```

**scaffold.yaml Format:**
```yaml
name: "template-name"
description: "Template description"
author: "Author Name"
version: "1.0.0"

prompts:
  - name: "project_name"
    description: "Project name"
    type: "input"
    default: "my-project"

  - name: "aws_region"
    description: "AWS region"
    type: "input"
    default: "us-east-1"
```

#### Templating Engine

Uses Go text/template with Gomplate functions for maximum flexibility:

**Template Variables:**
- `.Config.<field>` - User-provided values from prompts or `--set` flags
- `.ScaffoldPath` - Target directory path
- `.TemplateName` - Template name
- `.TemplateDescription` - Template description

**Supported Template Features:**
- Conditional file generation: `{{if .Config.enable_monitoring}}file.yaml{{end}}`
- Dynamic paths: `{{.Config.namespace}}/config.yaml`
- Content templating: `project: {{.Config.project_name}}`
- Gomplate functions: `upper`, `lower`, `title`, `default`, etc.

### User Workflows

#### Workflow 1: Initialize New Atmos Project (Interactive)

```bash
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

Next steps:
1. Add your Terraform components in components/terraform/
2. Create stack configurations in stacks/
3. Run `atmos terraform plan <component> -s <stack>` to get started
```

#### Workflow 2: Initialize Project (Non-Interactive)

```bash
$ atmos init atmos ./my-project \
  --set project_name=my-infra \
  --set terraform_version=1.5.0 \
  --set aws_region=us-east-1 \
  --no-interactive

Initializing atmos in ./my-project

  ✓ atmos.yaml
  ✓ README.md
  ✓ components/terraform/.gitkeep
  ✓ components/helmfile/.gitkeep
  ✓ stacks/.gitkeep
  ✓ schemas/jsonschema/.gitkeep
  ✓ schemas/opa/.gitkeep

Initialized 7 files.
```

#### Workflow 3: Generate from Scaffold Template

```bash
$ atmos scaffold generate vpc-component ./components/terraform/vpc \
  --set vpc_name=main \
  --set cidr_block=10.0.0.0/16

Generating vpc-component in ./components/terraform/vpc

  ✓ main.tf
  ✓ variables.tf
  ✓ outputs.tf
  ✓ versions.tf
  ✓ README.md

Initialized 5 files.
```

#### Workflow 4: List Available Scaffolds

```bash
$ atmos scaffold list

Available scaffold templates:

Name              Source                                    Version    Description
────────────────  ────────────────────────────────────────  ─────────  ──────────────────────────────
vpc-component     ./scaffolds/vpc                           1.0.0      AWS VPC component template
eks-cluster       ./scaffolds/eks                           2.1.0      EKS cluster template
rds-instance      github.com/acme/scaffolds/rds.git         1.5.0      RDS database template
```

### Command Reference

#### `atmos init`

**Syntax:**
```bash
atmos init [template] [target] [flags]
```

**Arguments:**
- `template` (optional): Template name (e.g., `simple`, `atmos`). If omitted, interactive selection is shown.
- `target` (optional): Target directory for project. If omitted, user is prompted.

**Flags:**
- `-f, --force`: Overwrite existing files
- `-i, --interactive`: Interactive mode (default: true)
- `--set key=value`: Set template values (repeatable)

**Examples:**
```bash
# Interactive mode
atmos init

# Specific template
atmos init simple ./my-project

# Non-interactive with values
atmos init atmos ./my-project \
  --set project_name=acme-infra \
  --set aws_region=us-east-1 \
  --no-interactive
```

#### `atmos scaffold generate`

**Syntax:**
```bash
atmos scaffold generate <template> [target] [flags]
```

**Arguments:**
- `template` (required): Template name, path, or URL
- `target` (optional): Target directory. If omitted, user is prompted (interactive mode only).

**Flags:**
- `-f, --force`: Overwrite existing files
- `--dry-run`: Preview changes without writing files
- `--set key=value`: Set template values (repeatable)

**Examples:**
```bash
# From configured template
atmos scaffold generate vpc-component ./components/terraform/vpc

# From local path
atmos scaffold generate ./my-templates/eks ./components/terraform/eks

# From Git repository
atmos scaffold generate https://github.com/acme/templates/vpc.git ./components/terraform/vpc

# Dry run
atmos scaffold generate vpc-component ./components/terraform/vpc --dry-run
```

#### `atmos scaffold list`

Lists all scaffold templates configured in `atmos.yaml`.

**Syntax:**
```bash
atmos scaffold list
```

#### `atmos scaffold validate`

Validates `scaffold.yaml` files against the Atmos scaffold schema.

**Syntax:**
```bash
atmos scaffold validate [path]
```

**Arguments:**
- `path` (optional): Path to scaffold directory (default: current directory)

### Configuration

Scaffold templates can be configured in `atmos.yaml`:

```yaml
scaffold:
  # Base path for local scaffold templates
  base_path: "./scaffolds"

  # Configured templates
  templates:
    vpc-component:
      description: "AWS VPC component template"
      source: "./scaffolds/vpc"
      version: "1.0.0"

    eks-cluster:
      description: "EKS cluster template"
      source: "github.com/cloudposse/atmos-scaffolds/eks.git"
      version: "2.1.0"
      ref: "tags/v2.1.0"
```

## Technical Design

### Package Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    CLI Commands (cmd/)                       │
│  ┌──────────────┐              ┌──────────────────────┐    │
│  │ cmd/init/    │              │ cmd/scaffold/        │    │
│  │  - init.go   │              │  - scaffold.go       │    │
│  └──────┬───────┘              └──────────┬───────────┘    │
└─────────┼───────────────────────────────┼──────────────────┘
          │                               │
          │  Calls ExecuteInit            │  Calls ExecuteScaffold*
          ▼                               ▼
┌─────────────────────────────────────────────────────────────┐
│            Execution Layer (internal/exec/)                  │
│  ┌──────────────┐              ┌──────────────────────┐    │
│  │ init.go      │              │ scaffold.go          │    │
│  └──────┬───────┘              └──────────┬───────────┘    │
└─────────┼───────────────────────────────┼──────────────────┘
          │                               │
          │  Uses UI & Embeds             │  Uses UI & Config
          ▼                               ▼
┌─────────────────────────────────────────────────────────────┐
│              Core Packages (pkg/)                            │
│  ┌──────────────┐  ┌─────────────┐  ┌──────────────────┐  │
│  │ init/ui/     │  │ init/embeds/│  │ init/config/     │  │
│  │ - ui.go      │  │ - embeds.go │  │ - config.go      │  │
│  │ - prompts    │  │ - templates │  │ - validation     │  │
│  └──────┬───────┘  └─────────────┘  └──────────────────┘  │
│         │                                                    │
│         │  Uses templating                                  │
│         ▼                                                    │
│  ┌──────────────────────┐                                  │
│  │ scaffold/templating/ │                                  │
│  │ - templating.go      │                                  │
│  │ - Go templates       │                                  │
│  │ - Gomplate funcs     │                                  │
│  └──────────────────────┘                                  │
└─────────────────────────────────────────────────────────────┘
```

### Key Design Decisions

#### 1. Embedded vs External Templates

**Decision**: Init uses embedded templates; scaffold supports both.

**Rationale**:
- Init templates are stable, version-locked foundations
- Scaffold templates are organization-specific and evolve independently
- Embedded templates ensure consistent experience across Atmos versions
- External templates enable customization and community contributions

#### 2. Command Registry Pattern

**Decision**: Use command registry for both init and scaffold commands.

**Rationale**:
- Follows established Atmos pattern (see `cmd/about/`, `cmd/version/`)
- Enables clean separation of concerns
- Makes commands independently testable
- Simplifies command addition and maintenance

#### 3. Shared UI and Templating

**Decision**: Both commands share UI (`pkg/init/ui`) and templating (`pkg/scaffold/templating`) packages.

**Rationale**:
- Consistent user experience across commands
- Reduces code duplication
- Single source of truth for template processing logic
- Easier to maintain and test

#### 4. Go Templates + Gomplate

**Decision**: Use Go text/template with Gomplate functions.

**Rationale**:
- Powerful template capabilities (conditionals, loops, functions)
- Familiar to Go developers and Atmos users
- Gomplate provides rich function library (string manipulation, data sources, etc.)
- No external dependencies (compiled into binary)
- Consistent with Atmos stack template rendering

## Testing Strategy

### Unit Tests

- `pkg/init/config/` - Configuration parsing and validation
- `pkg/init/embeds/` - Template loading and file structure
- `pkg/scaffold/templating/` - Template processing and rendering
- `pkg/init/ui/` - UI components and prompts

### Integration Tests

- `internal/exec/init/` - End-to-end init workflows
- `internal/exec/scaffold/` - End-to-end scaffold workflows
- Template rendering with various input combinations
- Error handling and validation scenarios

### Test Coverage Requirements

- Minimum 80% coverage for new code
- All template processing logic must be tested
- UI components tested with mock inputs
- Error paths tested for all commands

## Success Metrics

### User Experience Metrics

1. **Time to First Project**: < 2 minutes from install to working Atmos project
2. **Command Discoverability**: Users find init/scaffold in `atmos --help` within first 5 minutes
3. **Template Reuse**: Organizations create and share scaffold templates
4. **Error Recovery**: Users understand and fix template errors without support tickets

### Technical Metrics

1. **Test Coverage**: > 80% for all init/scaffold packages
2. **Build Time Impact**: < 5% increase in binary size from embedded templates
3. **Performance**: Init completes in < 1 second for simple template
4. **Reliability**: Zero crashes or data loss in production use

## Future Enhancements

### Phase 2 (Post-MVP)

1. **Template Marketplace**: Central registry of community scaffold templates
2. **Template Validation**: Schema validation for scaffold.yaml files
3. **Git Integration**: Clone templates directly from GitHub/GitLab
4. **Template Composition**: Combine multiple templates
5. **Custom Delimiters**: Support `[[` `]]` delimiters for templates with heavy `{{ }}` usage

### Phase 3 (Long-term)

1. **Interactive Template Builder**: Web UI for creating scaffold templates
2. **Template Testing**: Built-in test framework for templates
3. **Version Management**: Template versioning and updates
4. **AI-Assisted Generation**: Generate templates from natural language descriptions
5. **Cross-Platform Installers**: Homebrew, Chocolatey packages with init command promotion

## Migration and Rollout

### Release Strategy

1. **Feature Flag**: Ship behind feature flag in v1.95.0
2. **Beta Release**: Enable by default in v1.96.0-beta
3. **GA Release**: Full release in v1.97.0
4. **Documentation**: Complete docs and blog post at GA

### Backwards Compatibility

- No breaking changes to existing Atmos functionality
- New commands are additive
- Existing workflows unaffected

### Deprecation Plan

N/A - No existing functionality is being deprecated.

## Risks and Mitigations

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Users confused about init vs scaffold | Medium | Medium | Clear documentation, examples, and blog post explaining use cases |
| Template security concerns | High | Low | Document template validation; future: template signing |
| Template compatibility across Atmos versions | Medium | Medium | Version templates with Atmos; clear compatibility matrix |
| Large binary size from embedded templates | Low | Low | Monitor binary size; implement compression if needed |
| Performance degradation | Medium | Low | Benchmark template processing; optimize hot paths |

## Documentation Requirements

### User Documentation

1. **CLI Reference**: Complete command reference for all commands and flags
2. **Quick Start Guide**: Step-by-step tutorial for new users
3. **Template Authoring Guide**: How to create custom scaffold templates
4. **Examples Repository**: Sample templates for common use cases
5. **Blog Post**: Announcement with use cases and examples

### Developer Documentation

1. **PRD** (this document): Product requirements and design decisions
2. **Architecture Diagram**: System components and data flow
3. **Testing Guide**: How to run and write tests
4. **Template Specification**: scaffold.yaml schema and conventions

## Appendix

### Related Projects

- **Cookiecutter**: Python-based project templating (inspiration)
- **Yeoman**: JavaScript scaffolding tool (similar UX)
- **Terraform Scaffolding**: terraform-provider-scaffolding
- **Cloud Posse Modules**: Reference architecture patterns

### References

- [Atmos Architecture](https://atmos.tools/core-concepts/stacks)
- [Go Templates](https://pkg.go.dev/text/template)
- [Gomplate Functions](https://docs.gomplate.ca/)
- [Command Registry PRD](./command-registry-pattern.md)
