# PRD: atmos scaffold - Code Generation from Templates

## Executive Summary

`atmos scaffold` is a command suite for generating code, configurations, and directory structures from templates. It provides a flexible scaffolding system that supports embedded templates, custom templates from `atmos.yaml`, and remote templates from Git repositories. The command enables teams to standardize component creation, enforce best practices, and accelerate development.

## Problem Statement

### User Needs

Infrastructure teams need to create similar resources repeatedly:
1. **Terraform components** - VPC, EKS, RDS, etc. with consistent structure
2. **Stack configurations** - Similar YAML patterns across environments
3. **Documentation** - README files with standard sections
4. **Workflows** - Repeated CI/CD pipeline configurations
5. **Custom resources** - Team-specific patterns and conventions

**Manual creation challenges**:
- Time-consuming to copy-paste and modify
- Inconsistent structure across components
- Easy to miss required files or configuration
- No standard way to share templates across teams
- Difficult to keep templates up-to-date

### Current State

**Status**: `atmos scaffold` does not exist yet. This PRD defines the entire feature from scratch.

**Context**: Atmos currently has `atmos init` for project initialization, but lacks a generic scaffolding system for generating individual components, configurations, and resources from templates.

**What exists**:
- `atmos init` command (for project initialization only)
- `pkg/generator` package (used by init, will be reused for scaffold)

**What doesn't exist**:
- ❌ `atmos scaffold` command or subcommands
- ❌ Scaffold template system
- ❌ Custom template configuration in atmos.yaml
- ❌ Template validation
- ❌ Scaffold-specific UI/prompts

## Goals

### Primary Goals

1. **Rapid scaffolding** - Create components in seconds, not minutes
2. **Template flexibility** - Support embedded, custom, and remote templates
3. **Team standards** - Enforce organizational conventions automatically
4. **Interactive guidance** - Help users through template prompts
5. **Safe updates** - Update scaffolds from templates without losing work

### Non-Goals

1. **Code generation** - Not generating actual Terraform/HCL code logic
2. **Component validation** - Not validating Terraform syntax (use `terraform validate`)
3. **Template IDE** - Not building template authoring tools
4. **Package management** - Not managing template dependencies

## Use Cases

### Use Case 1: Generate Terraform Component (Interactive)

**Actor**: DevOps engineer creating new VPC component

**Flow**:
```bash
$ atmos scaffold generate
? Select a template:
  > terraform-component - Standard Terraform component structure
    stack-config - Stack configuration file
    workflow - GitHub Actions workflow

? Component name: vpc
? AWS region: us-east-1
? Enable NAT Gateway? (y/N) y

✓ Created components/terraform/vpc/main.tf
✓ Created components/terraform/vpc/variables.tf
✓ Created components/terraform/vpc/outputs.tf
✓ Created components/terraform/vpc/README.md
✓ Component scaffolded successfully!
```

**Result**: Complete Terraform component ready to customize

### Use Case 2: Generate from Custom Template (Non-Interactive)

**Actor**: CI/CD pipeline creating standardized components

**Flow**:
```bash
$ atmos scaffold generate my-custom-template ./components/terraform/rds \
    --set component_name=rds \
    --set database_engine=postgres \
    --set instance_class=db.t3.medium
```

**Result**: Component created from team's custom template without prompts

### Use Case 3: List Available Templates

**Actor**: Developer exploring what templates are available

**Flow**:
```bash
$ atmos scaffold list

Available Scaffold Templates:

┌─────────────────────────┬──────────────────────────────────────┬────────────┐
│ Name                    │ Description                          │ Source     │
├─────────────────────────┼──────────────────────────────────────┼────────────┤
│ terraform-component     │ Standard Terraform component         │ embedded   │
│ stack-config            │ Stack configuration file             │ embedded   │
│ custom-vpc              │ Team VPC component template          │ atmos.yaml │
│ eks-cluster             │ EKS cluster with best practices      │ atmos.yaml │
└─────────────────────────┴──────────────────────────────────────┴────────────┘

Configure custom templates in atmos.yaml under 'scaffold.templates'.
```

**Result**: Clear view of available templates

### Use Case 4: Update Generated Scaffold (Future)

**Actor**: Developer updating component to latest template version

**Flow**:
```bash
$ cd components/terraform/vpc
$ atmos scaffold generate vpc --update

Updating from template 'terraform-component'...

✓ main.tf - merged (user customizations preserved)
✓ variables.tf - updated (new template variables added)
⚠ outputs.tf - conflicts detected

Conflicts in outputs.tf:
<<<<<<< HEAD (your version)
  custom_output = var.custom_value
=======
  new_template_output = aws_vpc.main.id
>>>>>>> template

Please resolve conflicts manually or use --merge-strategy.
```

**Result**: Scaffold updated with intelligent merging

### Use Case 5: Validate Custom Templates

**Actor**: Template author ensuring their templates are valid

**Flow**:
```bash
$ atmos scaffold validate ./templates/

Validating ./templates/vpc/scaffold.yaml
✓ ./templates/vpc/scaffold.yaml: valid

Validating ./templates/eks/scaffold.yaml
✗ ./templates/eks/scaffold.yaml: prompt 'cluster_version' missing required field 'type'

Validation Summary:
✓ Valid files: 1
✗ Invalid files: 1
```

**Result**: Template issues identified before use

## Solution Architecture

### Command Structure

```
atmos scaffold
  generate [template] [target]  # Generate code from template
    --force, -f                 # Overwrite existing files
    --dry-run                   # Preview without writing
    --set key=value             # Set template variables
    --update                    # Update existing scaffold (future)
    --merge-strategy            # Conflict resolution (future)
    --max-changes               # Change threshold (future)

  list                          # List available templates

  validate [path]               # Validate template configuration
```

### Implementation Components

```
cmd/scaffold/
├── scaffold.go              # Command definitions
├── scaffold_test.go         # Command tests
└── scaffold-schema.json     # Template validation schema

pkg/generator/
├── templates/
│   ├── embeds.go            # Embedded templates
│   └── embeds_test.go
├── engine/
│   ├── templating.go        # Template processor (uses merge)
│   └── templating_test.go
├── merge/                   # 3-way merge implementation
│   ├── merge.go             # (See three-way-merge PRD)
│   ├── text_merger.go
│   └── yaml_merger.go
└── ui/
    └── ui.go                # Interactive prompts and UI
```

### Template Sources

**1. Embedded Templates** (built into Atmos binary):

```go
// pkg/generator/templates/embeds.go
//go:embed templates/*
var templatesFS embed.FS

// Built-in templates:
// - terraform-component
// - stack-config
// - workflow
```

**2. Custom Templates** (in `atmos.yaml`):

```yaml
# atmos.yaml
scaffold:
  templates:
    custom-vpc:
      description: "Team VPC component template"
      source: "./templates/vpc"
      target_dir: "components/terraform/{{ .Config.component_name }}"
      prompts:
        - name: component_name
          description: "Component name"
          type: input
          required: true
        - name: cidr_block
          description: "VPC CIDR block"
          type: input
          default: "10.0.0.0/16"
        - name: enable_nat_gateway
          description: "Enable NAT Gateway?"
          type: confirm
          default: true

    eks-cluster:
      description: "EKS cluster with best practices"
      source: "git::https://github.com/company/templates.git//eks?ref=v1.0.0"
      prompts:
        - name: cluster_name
          type: input
          required: true
        - name: kubernetes_version
          type: select
          options:
            - "1.28"
            - "1.29"
            - "1.30"
```

**3. Remote Templates** (Git repositories):

```yaml
scaffold:
  templates:
    remote-template:
      source: "git::https://github.com/user/repo.git//path?ref=v1.0.0"
      # Supports:
      # - GitHub, GitLab, Bitbucket
      # - Branch, tag, commit references
      # - Subdirectory paths
```

### Template Structure

**scaffold.yaml** (template configuration):

```yaml
name: terraform-component
description: Standard Terraform component structure
author: CloudPosse
version: 1.0.0

prompts:
  - name: component_name
    description: "Name of the Terraform component"
    type: input
    required: true

  - name: aws_region
    description: "AWS region"
    type: select
    options:
      - us-east-1
      - us-east-2
      - us-west-1
      - us-west-2
    default: us-east-1

  - name: enable_monitoring
    description: "Enable CloudWatch monitoring?"
    type: confirm
    default: true

files:
  - source: main.tf.tmpl
    target: main.tf
    is_template: true

  - source: variables.tf.tmpl
    target: variables.tf
    is_template: true

  - source: outputs.tf.tmpl
    target: outputs.tf
    is_template: true

  - source: README.md.tmpl
    target: README.md
    is_template: true

dependencies:
  - terraform >= 1.5.0
  - aws provider >= 5.0.0

hooks:
  pre_generate:
    - command: "terraform fmt"
  post_generate:
    - command: "terraform validate"
```

**Template files** (with Go template syntax):

```hcl
# main.tf.tmpl
terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0.0"
    }
  }
}

module "{{ .Config.component_name }}" {
  source = "cloudposse/{{ .Config.component_name }}/aws"

  {{ if .Config.enable_monitoring -}}
  enable_monitoring = true
  {{- end }}

  tags = {
    Name        = {{ .Config.component_name | quote }}
    Environment = {{ env "ENVIRONMENT" | default "dev" }}
  }
}
```

### Template Rendering

**Available template functions**:

```yaml
# Gomplate functions (data sources, crypto, AWS, etc.)
{{ env "AWS_REGION" }}
{{ aws.Region }}
{{ file.Read "./config.yaml" }}

# Sprig functions (string manipulation, date, etc.)
{{ .Config.name | upper }}
{{ .Config.name | kebabcase }}
{{ now | date "2006-01-02" }}

# Custom functions
{{ config "component_name" }}         # Access prompt values
```

### Prompt Types

| Type | Description | Example |
|------|-------------|---------|
| `input` | Free-form text input | Component name, CIDR block |
| `select` | Single choice from list | AWS region, instance type |
| `confirm` | Yes/no question | Enable feature? |
| `multiselect` | Multiple choices from list | Availability zones |

### Update Flow (with 3-Way Merge)

Similar to `atmos init`, but for individual scaffolds:

```
Initial generation:
1. Prompt user for template variables
2. Render template files
3. Write files to target directory
4. Store base content in .atmos/scaffold/base/{scaffold-name}/
5. Write metadata to .atmos/scaffold/metadata.yaml

Update (atmos scaffold generate --update):
1. Detect scaffold by analyzing existing files
2. Load base content from .atmos/scaffold/base/
3. Load current files (ours - with user changes)
4. Re-render template with updated version (theirs)
5. Perform 3-way merge (base, ours, theirs)
6. Write merged content
7. Update base content for future updates
```

**For 3-way merge details**, see: [docs/prd/three-way-merge/](./three-way-merge/)

## Implementation Details

### Phase 1: Core Scaffolding System

**Goal**: Build the foundational `atmos scaffold` command with basic generation capabilities.

**Tasks**:
1. Create `cmd/scaffold/` package
   - `scaffold.go` - Parent command and subcommands
   - `scaffold-schema.json` - Template validation schema
   - `scaffold_test.go` - Command tests
2. Implement `scaffold generate` subcommand
   - Interactive template selection
   - Non-interactive mode with arguments
   - Variable substitution via `--set` flags
   - Force overwrite mode (`--force`)
   - Dry-run mode (`--dry-run`)
3. Implement `scaffold list` subcommand
   - Display available templates in table format
   - Show embedded and custom templates
4. Implement `scaffold validate` subcommand
   - Validate scaffold.yaml files
   - Check required fields and structure
5. Create embedded templates
   - terraform-component
   - stack-config
   - workflow (GitHub Actions)
6. Add support for custom templates in atmos.yaml
   - Parse `scaffold.templates` section
   - Load templates from local directories
   - Merge with embedded templates
7. Reuse `pkg/generator` infrastructure
   - Template rendering engine
   - File processing
   - Interactive UI/prompts
8. Write comprehensive tests
   - Unit tests for all subcommands
   - Integration tests for generation flows
   - Template validation tests

**Deliverables**:
- Fully functional `atmos scaffold generate` command
- `atmos scaffold list` and `atmos scaffold validate` subcommands
- Embedded templates for common use cases
- Documentation and examples

### Phase 2: Update Support (Depends on 3-Way Merge)

**Prerequisites**:
- Requires completion of [three-way-merge PRD](./three-way-merge/)
- Specifically: Phase 3 (Base Storage & Integration)

**Tasks**:
1. Add `--update` flag to `generate` subcommand
2. Implement scaffold detection (identify which template was used)
3. Implement base content storage
   - Store original output in `.atmos/scaffold/base/{scaffold-name}/`
   - Write metadata to `.atmos/scaffold/metadata.yaml`
4. Integrate 3-way merge in file handling
5. Add conflict resolution UI
6. Test update scenarios

**Metadata format** (`.atmos/scaffold/metadata.yaml`):

```yaml
version: 1
scaffolds:
  - name: vpc
    template: custom-vpc
    template_version: 1.0.0
    target_dir: components/terraform/vpc
    generated_at: 2025-01-15T10:30:00Z
    variables:
      component_name: vpc
      cidr_block: 10.0.0.0/16
      enable_nat_gateway: true
    files:
      - path: main.tf
        template: main.tf.tmpl
        checksum: sha256:abc123...
      - path: variables.tf
        template: variables.tf.tmpl
        checksum: sha256:def456...
```

### Phase 3: Remote Templates (Future)

**Support Git sources**:
- Parse Git URLs with go-getter syntax
- Clone/fetch remote repositories
- Cache templates locally
- Version pinning support

**Example**:
```bash
atmos scaffold generate \
  git::https://github.com/company/templates.git//eks?ref=v1.0.0 \
  ./components/terraform/eks
```

## CLI Usage Examples

### Basic Usage

```bash
# Interactive mode
atmos scaffold generate

# Non-interactive with template
atmos scaffold generate terraform-component ./components/terraform/vpc

# Pass variables
atmos scaffold generate terraform-component ./components/terraform/vpc \
  --set component_name=vpc \
  --set aws_region=us-east-1 \
  --set enable_monitoring=true
```

### List Templates

```bash
# List all available templates
atmos scaffold list

# Shows:
# - Embedded templates (built-in)
# - Custom templates from atmos.yaml
```

### Validate Templates

```bash
# Validate all templates in current directory
atmos scaffold validate

# Validate specific template
atmos scaffold validate ./templates/vpc

# Validate specific file
atmos scaffold validate ./templates/vpc/scaffold.yaml
```

### Force Overwrite

```bash
# Overwrite existing files
atmos scaffold generate terraform-component ./components/terraform/vpc --force

# Useful for:
# - Regenerating from template
# - Testing templates
# - Resetting to defaults
```

### Dry Run

```bash
# Preview what would be generated
atmos scaffold generate terraform-component ./components/terraform/vpc \
  --dry-run \
  --set component_name=vpc

# Shows:
# - Files that would be created
# - Content preview
# - No files actually written
```

### Update Mode (Future)

```bash
# Update existing scaffold from template
cd components/terraform/vpc
atmos scaffold generate --update

# Auto-resolve conflicts (use template version)
atmos scaffold generate --update --merge-strategy=theirs

# Preview update
atmos scaffold generate --update --dry-run
```

## Relationship with atmos init

**Architecture**: `atmos init` builds on `atmos scaffold` - scaffold is the generic implementation.

```
pkg/generator/          # Generic scaffolding infrastructure
├── setup/              # Context creation
├── templates/          # Template registry
├── engine/             # Template processing and file operations
├── merge/              # 3-way merge (Phase 2)
└── ui/                 # Interactive prompts

atmos scaffold          # Generic scaffolding (this PRD)
  ├── Custom templates
  ├── Template validation
  └── Flexible generation

atmos init              # Specialized for project initialization
  └── Reuses scaffold infrastructure
      with project-specific templates
```

**How init leverages scaffold**:

| Infrastructure | Scaffold Provides | Init Uses |
|----------------|-------------------|-----------|
| `pkg/generator/engine` | Template rendering, file processing | Project file generation |
| `pkg/generator/templates` | Template registry | Init-specific embedded templates |
| `pkg/generator/ui` | Interactive prompts | Project setup flow |
| `pkg/generator/merge` | 3-way merge (Phase 2) | Project update mode |

**Key differences**:

| Feature | atmos scaffold | atmos init |
|---------|----------------|------------|
| **Purpose** | Generate individual components | Initialize entire project |
| **Templates** | Embedded + custom + remote | Embedded only (project templates) |
| **Target** | Any directory | Project root |
| **Frequency** | Many times per project | Once per project |
| **Configuration** | `atmos.yaml` (scaffold section) | N/A |
| **Implementation Status** | New (this PRD) | Exists, will be enhanced |

## Error Handling

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `scaffold template 'foo' not found` | Invalid template name | Use `atmos scaffold list` to see available templates |
| `target directory is required when using --dry-run` | Missing target in dry-run mode | Provide target directory as argument |
| `file exists` | File exists, no `--force` or `--update` | Use `--force` to overwrite or `--update` to merge |
| `merge conflicts detected` | Both user and template modified same content | Resolve conflicts manually or use `--merge-strategy` |
| `failed to parse scaffold YAML` | Invalid scaffold.yaml syntax | Use `atmos scaffold validate` to check syntax |
| `prompt 'name' missing required field 'type'` | Invalid prompt configuration | Fix prompt definition in scaffold.yaml |

## Success Criteria

### Functional Requirements

- [ ] **Template selection** - Interactive and non-interactive modes work
- [ ] **Custom templates** - Load from atmos.yaml successfully
- [ ] **Variable substitution** - `--set` flags correctly passed to templates
- [ ] **File generation** - All template files created with correct permissions
- [ ] **Force overwrite** - `--force` flag overwrites existing files
- [ ] **List command** - Shows all available templates
- [ ] **Validate command** - Detects invalid scaffold.yaml files
- [ ] **Update mode** - `--update` flag performs 3-way merge (Phase 2)
- [ ] **Remote templates** - Git sources supported (Phase 3)

### User Experience Requirements

- [ ] **Fast generation** - Complete scaffold in <10 seconds
- [ ] **Clear prompts** - Interactive mode is intuitive
- [ ] **Helpful errors** - Error messages include actionable solutions
- [ ] **Preview mode** - `--dry-run` shows what would be created

### Quality Requirements

- [ ] **Test coverage >80%** - Comprehensive unit and integration tests
- [ ] **Cross-platform** - Works on Linux, macOS, Windows
- [ ] **Documented** - Website documentation with examples
- [ ] **No regressions** - Existing functionality preserved

## Testing Strategy

### Unit Tests

```go
// cmd/scaffold/scaffold_test.go
TestScaffoldGenerateCommand()      // Command registration
TestScaffoldListCommand()          // List functionality
TestScaffoldValidateCommand()      // Validation
TestScaffoldFlags()                // Flag parsing
TestScaffoldPrompts()              // Interactive prompts
TestScaffoldVariables()            // --set flag handling

// pkg/generator/templates/embeds_test.go
TestGetAvailableConfigurations()   // Template discovery
TestLoadEmbeddedTemplate()         // Embedded template loading
```

### Integration Tests

```bash
# tests/test-cases/scaffold/
test_scaffold_generate_interactive.sh      # Interactive flow
test_scaffold_generate_noninteractive.sh   # Automation
test_scaffold_with_variables.sh            # Variable substitution
test_scaffold_force_overwrite.sh           # --force flag
test_scaffold_dry_run.sh                   # --dry-run flag
test_scaffold_list.sh                      # List templates
test_scaffold_validate.sh                  # Validate templates
test_scaffold_custom_template.sh           # Custom from atmos.yaml
test_scaffold_update.sh                    # Update mode (Phase 2)
```

### Manual Testing Checklist

- [ ] Run `atmos scaffold generate` without arguments → shows interactive prompts
- [ ] Select each embedded template → creates correct files
- [ ] Run with custom template from atmos.yaml → uses custom template
- [ ] Run `atmos scaffold list` → shows all available templates
- [ ] Run `atmos scaffold validate` → validates templates correctly
- [ ] Run with `--set` flags → variables correctly substituted
- [ ] Run with `--force` → overwrites existing files
- [ ] Run with `--dry-run` → shows preview without writing
- [ ] Run with `--update` → merges changes intelligently (Phase 2)

## Documentation Requirements

### Website Documentation

Location: `website/docs/cli/commands/atmos-scaffold.mdx`

**Sections**:
1. **Overview** - What the command does
2. **Subcommands** - generate, list, validate
3. **Flags** - Description of all flags
4. **Examples** - Common use cases with outputs
5. **Custom Templates** - How to configure in atmos.yaml
6. **Template Structure** - scaffold.yaml format
7. **Template Syntax** - Go templates, Gomplate, Sprig
8. **Updating Scaffolds** - Using `--update` flag (Phase 2)
9. **Remote Templates** - Git sources (Phase 3)
10. **Troubleshooting** - Common errors and solutions

### Template Documentation

**For template authors**:
- scaffold.yaml reference
- Prompt types and validation
- Template function examples
- Best practices for reusable templates

## Dependencies

### External Libraries

- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration management
- `github.com/Masterminds/sprig/v3` - Template functions
- `github.com/hairyhenderson/gomplate/v3` - Template rendering
- `gopkg.in/yaml.v3` - YAML parsing (scaffold.yaml)

### Internal Packages

- `pkg/generator/*` - Generator framework (shared with init)
- `pkg/flags` - Flag parsing and precedence
- `pkg/project/config` - Atmos configuration loading
- `errors` - Static error definitions

### Blockers for Phase 2 (Update Support)

- **3-way merge implementation** - See [three-way-merge PRD](./three-way-merge/)
- Specifically depends on:
  - Phase 1: Text-based merge
  - Phase 2: YAML-aware merge
  - Phase 3: Base content storage

## Future Enhancements

### Template Marketplace

- Discover community templates
- Share custom templates
- Rate and review templates

### Template Packages

- Bundle related templates together
- Template dependencies and composition
- Versioned template packages

### Advanced Prompts

- Conditional prompts (based on previous answers)
- Validation rules (regex, min/max, etc.)
- Dynamic default values (computed from other inputs)

### IDE Integration

- VS Code extension for template development
- Syntax highlighting for scaffold.yaml
- Template preview and testing

### Hooks and Lifecycle

```yaml
hooks:
  pre_generate:
    - validate_prerequisites.sh
  post_generate:
    - terraform fmt
    - terraform validate
    - git add .
```

## References

- **Three-Way Merge PRD**: [docs/prd/three-way-merge/](./three-way-merge/)
- **atmos init PRD**: [docs/prd/atmos-init.md](./atmos-init.md)
- **Command Registry Pattern**: [docs/prd/command-registry-pattern.md](./command-registry-pattern.md)
- **Generator Package**: `pkg/generator/`
- **Gomplate**: https://docs.gomplate.ca/
- **Sprig**: https://masterminds.github.io/sprig/
- **Cookiecutter**: https://github.com/cookiecutter/cookiecutter (inspiration)

## Related PRDs

- **atmos init** - Similar command for project initialization
- **Three-way merge** - Core merge algorithm for update mode
- **Template system** - Shared generator package design
