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

**Status**: `atmos scaffold` is implemented and shipped, including Phase 1 (core
scaffolding) and Phase 2 (`--update` with a real 3-way merge). This PRD was
originally written before the feature existed; the sections below have been
updated where they described the shipped feature as not existing, but this
remains primarily a historical design document — see `cmd/scaffold/scaffold.go`
and `pkg/generator/` for the source of truth on current behavior.

**What exists today**:
- `atmos scaffold generate`, `atmos scaffold list`, `atmos scaffold validate`
- Embedded, custom (`atmos.yaml` `scaffold.templates`), and catalog/remote templates
- `--force`, `--update`, `--base-ref`, `--dry-run`, `--merge-strategy` flags
  (see "Update Mode" below for what shipped vs. this PRD's original phasing)
- `pkg/generator` package (shared with `atmos init`)

**Also implemented, shipped after this PRD was first written**:
- `spec.fields[].when:` and `spec.files[].when:` — declarative conditional
  prompting and file generation, gated on prompt answers via the same CEL
  `when:` engine workflows/hooks/custom commands use (`pkg/condition`)
- `spec.hooks:` — real `pre`/`post`-generate hooks (`before.scaffold.generate`
  / `after.scaffold.generate`), reusing `pkg/hooks.Hook`'s exact vocabulary
  (`events`/`kind`/`when`/`type`/`with`) and executed through the same shared
  step engine (`pkg/runner/step`) workflows/custom commands use. Only
  `kind: step`/`kind: steps` are supported today. `--skip-hooks` (mirroring
  `terraform`'s flag) bypasses hooks for a run.

**Still not implemented** (see "Future Enhancements" below):
- ❌ Remote-template caching/version pinning beyond a single `--ref`
- ❌ `for_each`-style dynamic file generation over an unbounded/unknown answer
  set (static per-file `when:`, above, covers a fixed/enumerable option set)
- ❌ A `--max-changes` CLI flag — the merger has an internal conflict-percentage
  threshold (hardcoded default, currently 50%), but it isn't exposed as a flag

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

### Use Case 4: Update Generated Scaffold

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
    --dry-run                   # Preview without writing (with --update, drives the real merge and reports would-create/would-update/conflicts; skips only the disk write)
    --set key=value             # Set template variables
    --update                    # Update an existing target directory via a 3-way merge (requires a git base; see --base-ref)
    --base-ref                  # Git ref to use as the 3-way merge base with --update (defaults to HEAD)
    --merge-strategy            # Conflict resolution for --update: manual (default), ours, theirs
    --max-changes               # Change threshold (not implemented — no CLI flag; internal default is hardcoded)

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

**scaffold.yaml** (template configuration — the real `AtmosScaffoldConfig` manifest
shape; template files themselves are auto-discovered by walking the template
directory, not declared under `spec.files:` — that key is reserved for the
optional conditional-generation overlay shown below):

```yaml
apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: terraform-component
  description: Standard Terraform component structure
  author: CloudPosse
  version: 1.0.0
spec:
  fields:
    - name: component_name
      label: Name of the Terraform component
      type: input
      required: true

    - name: aws_region
      label: AWS region
      type: select
      options:
        - us-east-1
        - us-east-2
        - us-west-1
        - us-west-2
      default: us-east-1

    - name: enable_monitoring
      label: Enable CloudWatch monitoring?
      type: confirm
      default: true

    # Only prompted for when enable_monitoring is true -- per-field when:
    # gates a question on an earlier field's answer.
    - name: alert_email
      label: Alert notification email
      type: input
      when: "answers.enable_monitoring == true"

  # Optional: gate specific auto-discovered files on collected answers.
  # Files not listed here (main.tf.tmpl, variables.tf.tmpl, ...) always
  # generate.
  files:
    - path: alerts.tf
      when: "answers.enable_monitoring == true"

  # Optional: step-backed hooks around generation. Reuses the exact
  # events/kind/when/type/with vocabulary stack-level lifecycle hooks use
  # (pkg/hooks.Hook) -- see the atmos-hooks skill for the shared vocabulary.
  # Only kind: step / kind: steps are supported for scaffold hooks.
  hooks:
    format:
      events:
        - after.scaffold.generate
      kind: step
      type: shell
      with:
        command: "terraform fmt"
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

Any field can declare `when:` (a predicate keyword, CEL expression, or list treated as
"all") to gate whether it's prompted for, evaluated against answers already collected
from fields declared earlier — e.g. `when: "answers.enable_monitoring == true"`. `spec.files:`
supports the same `when:` mechanism to gate whether a specific auto-discovered file is
generated. Compound conditions use CEL's `&&`/`||`/`!` (the `{all:/any:/not:}` map form
pkg/condition also parses is not accepted by the scaffold JSON Schema).

### Update Flow (with 3-Way Merge)

Similar to `atmos init`, but for individual scaffolds:

```
Initial generation:
1. Prompt user for template variables
2. Render template files
3. Write files to target directory

Update (atmos scaffold generate --update):
1. Resolve --base-ref (defaults to HEAD) in the target directory's git repository
2. Load each file's base content directly from that git ref (no on-disk base
   storage/snapshot — GitBaseStorage.LoadBase reads the blob straight out of
   git)
3. Load current files (ours - with user changes)
4. Re-render template with updated version (theirs)
5. Perform 3-way merge (base, ours, theirs), honoring --merge-strategy
   (manual/ours/theirs) for any genuine conflict
6. Write merged content (skipped in --dry-run; the merge still runs so
   conflicts are still reported)
```

**Note**: this shipped as a git-ref-based design, not the `.atmos/scaffold/base/`
file-snapshot approach originally sketched below in "Phase 2" — there is no
on-disk base storage or `.atmos/scaffold/metadata.yaml` in the current
implementation.

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

### Phase 2: Update Support (Shipped)

**Status**: Implemented. The original plan below described a file-snapshot base
store (`.atmos/scaffold/base/`, `.atmos/scaffold/metadata.yaml`) and scaffold
auto-detection; what shipped instead is a git-ref-based base (`--base-ref`,
defaulting to `HEAD`, resolved via `pkg/generator/storage.GitBaseStorage`) with
no on-disk snapshot or metadata file, and no auto-detection step — the user
supplies the same template/target-dir on `--update` as on initial generation.

**What shipped**:
1. `--update` (and `--base-ref`) flags on `generate`
2. 3-way merge integrated into file handling (`pkg/generator/merge`), with
   `--merge-strategy=manual|ours|theirs` for conflict resolution
3. `--dry-run` combined with `--update` runs the real merge and previews
   would-create/would-update/conflict status without writing files
4. Path-traversal and symlink-write protection (`validateWriteTarget` in
   `pkg/generator/engine/templating.go`), so a write can't escape the target
   directory through a symlinked intermediate path or destination
5. Test coverage for update scenarios

**Not shipped from the original plan**: scaffold auto-detection, a
`.atmos/scaffold/base/`+`metadata.yaml` file store (the original plan sketched
one; the shipped design reads bases directly from git instead — see "Update
Flow" above), and `pre_generate`/`post_generate` hooks.

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

### Update Mode

```bash
# Update existing scaffold from template (base = HEAD by default)
cd components/terraform/vpc
atmos scaffold generate --update

# Use a specific git ref as the merge base instead of HEAD
atmos scaffold generate --update --base-ref=v1.2.0

# Auto-resolve conflicts (use template version, or keep your version)
atmos scaffold generate --update --merge-strategy=theirs
atmos scaffold generate --update --merge-strategy=ours

# Preview update: runs the real merge (base load + 3-way merge + conflict
# checks) and reports create/update/conflict status per file, but writes nothing
atmos scaffold generate --update --dry-run
```

**`--force` and `--update` together**: `--update` takes precedence. If both
flags are set, existing files go through the 3-way merge path, not a raw
overwrite (see `handleExistingFile` in `pkg/generator/engine/templating.go`).

## Relationship with atmos init

**Architecture**: `atmos init` builds on `atmos scaffold` - scaffold is the generic implementation.

```
pkg/generator/          # Generic scaffolding infrastructure
├── setup/              # Context creation
├── templates/          # Template registry
├── engine/             # Template processing and file operations
├── merge/              # 3-way merge (shipped)
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
| `pkg/generator/merge` | 3-way merge (shipped) | Project update mode |

Because `atmos init` drives generation through the same `pkg/generator/ui.InitUI`
code path as `atmos scaffold generate`, it inherits `spec.fields[].when:`,
`spec.files[].when:`, `spec.hooks:`, and `--skip-hooks` automatically — see
`docs/prd/atmos-init.md` for the init-specific flag/behavior differences.

**Key differences**:

| Feature | atmos scaffold | atmos init |
|---------|----------------|------------|
| **Purpose** | Generate individual components | Initialize entire project |
| **Templates** | Embedded + custom + remote | Embedded only (project templates) |
| **Target** | Any directory | Project root |
| **Frequency** | Many times per project | Once per project |
| **Configuration** | `atmos.yaml` (scaffold section) | N/A |
| **Implementation Status** | Shipped | Shipped |

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
| `resolved path escapes target directory` | A template path (or a symlink in the target directory) would write outside the target directory | Check for symlinks redirecting outside the target; this is a hard-stop safety guard, not a flag to bypass |

**Path confinement**: every file write is confined under the resolved target
directory. `validateWriteTarget` (`pkg/generator/engine/templating.go`) resolves
symlinks in the write path and rejects both writes that escape the target
directory and writes through a symlink at the destination itself.

## Success Criteria

### Functional Requirements

- [ ] **Template selection** - Interactive and non-interactive modes work
- [ ] **Custom templates** - Load from atmos.yaml successfully
- [ ] **Variable substitution** - `--set` flags correctly passed to templates
- [ ] **File generation** - All template files created with correct permissions
- [ ] **Force overwrite** - `--force` flag overwrites existing files
- [ ] **List command** - Shows all available templates
- [ ] **Validate command** - Detects invalid scaffold.yaml files
- [x] **Update mode** - `--update` flag performs 3-way merge (shipped)
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
test_scaffold_update.sh                    # Update mode (shipped)
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
- [ ] Run with `--update` → merges changes intelligently (shipped; verify manually)

## Documentation Requirements

### Website Documentation

Location: `website/docs/cli/commands/scaffold.mdx`

**Sections**:
1. **Overview** - What the command does
2. **Subcommands** - generate, list, validate
3. **Flags** - Description of all flags
4. **Examples** - Common use cases with outputs
5. **Custom Templates** - How to configure in atmos.yaml
6. **Template Structure** - scaffold.yaml format
7. **Template Syntax** - Go templates, Gomplate, Sprig
8. **Updating Scaffolds** - Using `--update` flag (shipped)
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

- ✅ **Shipped**: conditional prompts based on previous answers
  (`spec.fields[].when:`, see "Template Structure" above)
- ✅ **Shipped**: regex validation (`spec.fields[].validation.pattern`/`message`)
- Dynamic default values (computed from other inputs) — still future
- `for_each`-style dynamic file generation over an unbounded/unknown answer set
  (e.g. a free-text multi-value field, rather than a fixed `options:` list) —
  the shipped `spec.files[].when:` overlay only gates a *known, fixed* set of
  files declared by the template author; a true loop would require the
  file-discovery step itself to become answer-aware, which today runs once
  before any prompt

### IDE Integration

- VS Code extension for template development
- Syntax highlighting for scaffold.yaml
- Template preview and testing

### Hooks and Lifecycle

**Shipped.** `spec.hooks:` runs step-backed actions around generation, keyed by
hook name, reusing the exact vocabulary stack-level lifecycle hooks use
(`pkg/hooks.Hook` — `events`/`kind`/`when`/`type`/`with`; see the `atmos-hooks`
skill for the shared vocabulary and `atmos-steps` for available step types).
Events are `before.scaffold.generate` and `after.scaffold.generate`; only
`kind: step`/`kind: steps` are supported (the `command`/`store`/`git` kinds
stack-level hooks also support are not yet wired up for scaffold — their
engines assume stack/component context scaffold generation doesn't have).
`--skip-hooks`/`--skip-hooks=name1,name2` bypasses hooks for a run, mirroring
`terraform`'s flag of the same name. See "Template Structure" above for a
worked example.

Future: `command`/`store`/`git` scaffold hook kinds, once/if there's a real
need to run them outside a stack/component context.

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
