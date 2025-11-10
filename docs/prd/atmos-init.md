# PRD: atmos init - Project Initialization

## Executive Summary

`atmos init` is a command that initializes new Atmos projects from templates. It provides both interactive and non-interactive modes for setting up project structure, configuration files, and directory layouts. The command supports template selection, variable substitution, and intelligent file merging for updates.

## Problem Statement

### User Needs

Setting up a new Atmos project involves creating:
1. **Configuration structure** - `atmos.yaml` with proper settings
2. **Directory layout** - Standard directories for stacks, components, workflows
3. **Best practices** - Pre-configured patterns that follow Atmos conventions
4. **Customization** - Ability to tailor templates to specific needs

**Manual setup challenges**:
- Time-consuming to create all necessary files
- Easy to miss important configuration options
- Inconsistent structure across projects
- No standard way to update existing projects when templates evolve

### Current State

**Status**: `atmos init` does not exist yet. This PRD defines the entire feature from scratch.

**Context**: Atmos lacks a built-in way to initialize projects from templates. Users currently must manually create directory structures, configuration files, and follow documentation to set up projects correctly.

**What exists**:
- `pkg/generator` package (will be built as part of scaffold/init implementation)
- Documentation for manual Atmos setup
- Example configurations in documentation

**What doesn't exist**:
- ❌ `atmos init` command
- ❌ Init templates (embedded or custom)
- ❌ Interactive project setup
- ❌ Template-based project generation
- ❌ Update mode for existing projects

## Goals

### Primary Goals

1. **Fast project setup** - Get from zero to working Atmos project in <5 minutes
2. **Template flexibility** - Support embedded and custom templates
3. **Interactive experience** - Guide users through setup with prompts
4. **Non-interactive support** - Allow automation via CLI flags
5. **Safe updates** - Update projects from templates without losing customizations

### Non-Goals

1. **Template authoring tools** - Not building a template IDE
2. **Template marketplace** - Not hosting/discovering third-party templates
3. **Migration wizards** - Not auto-upgrading breaking changes
4. **Git integration** - Not managing version control (user's responsibility)

## Use Cases

### Use Case 1: First-Time Setup (Interactive)

**Actor**: Developer setting up their first Atmos project

**Flow**:
```bash
$ atmos init
? Select a template:
  > simple - Basic Atmos project structure
    atmos - Complete atmos.yaml configuration only

? Enter target directory: ./my-infrastructure
? Project name: my-infrastructure
? AWS region: us-east-1

✓ Created atmos.yaml
✓ Created stacks/README.md
✓ Created components/terraform/README.md
✓ Project initialized successfully!
```

**Result**: Complete project structure ready to use

### Use Case 2: Automated Setup (Non-Interactive)

**Actor**: CI/CD pipeline or automation script

**Flow**:
```bash
$ atmos init simple ./my-infrastructure \
    --interactive=false \
    --set project_name=my-infrastructure \
    --set aws_region=us-east-1
```

**Result**: Project created without prompts, suitable for automation

### Use Case 3: Template Update (Existing Project)

**Actor**: Developer updating their project to latest template version

**Flow**:
```bash
$ cd my-infrastructure
$ atmos init --update

Updating project from template 'simple'...

✓ atmos.yaml - merged (user customizations preserved)
✓ stacks/README.md - updated (no conflicts)
⚠ components/terraform/main.tf - conflicts detected

Conflicts in components/terraform/main.tf:
<<<<<<< HEAD (your version)
  custom_setting = true
=======
  new_template_feature = true
>>>>>>> template

Please resolve conflicts manually or use:
  --merge-strategy=ours   (keep your version)
  --merge-strategy=theirs (use template version)
```

**Result**: Project updated with intelligent merging, conflicts clearly marked

### Use Case 4: Force Overwrite

**Actor**: Developer starting fresh or testing templates

**Flow**:
```bash
$ atmos init simple ./test-project --force

⚠ Warning: Existing files will be overwritten
✓ Overwriting atmos.yaml
✓ Overwriting stacks/README.md
✓ Project initialized successfully!
```

**Result**: All files overwritten, useful for template testing

## Solution Architecture

### Command Structure

```
atmos init [template] [target]
  --force, -f              Overwrite existing files
  --interactive, -i        Interactive mode (default: true)
  --update                 Update existing project from template
  --set key=value          Set template variables
  --merge-strategy         Conflict resolution strategy (manual|ours|theirs)
  --max-changes            Maximum change threshold percentage (default: 50)
  --dry-run                Preview changes without writing files
```

### Implementation Components

```
cmd/init/
├── init.go              # Cobra command definition
└── init_test.go         # Command tests

pkg/generator/
├── setup/
│   └── setup.go         # GeneratorContext creation
├── templates/
│   ├── embeds.go        # Embedded template registry
│   └── embeds_test.go
├── engine/
│   ├── templating.go    # Template processor (uses merge package)
│   └── templating_test.go
├── merge/               # 3-way merge implementation
│   ├── merge.go         # (See three-way-merge PRD)
│   ├── text_merger.go
│   └── yaml_merger.go
└── ui/
    └── ui.go            # Interactive UI and prompts
```

### Template Structure

**Embedded templates** (in `pkg/generator/templates/`):

```
templates/
├── simple/              # Basic project structure
│   ├── atmos.yaml.tmpl
│   ├── stacks/
│   │   └── README.md
│   └── components/
│       └── terraform/
│           └── README.md
└── atmos/               # atmos.yaml only
    └── atmos.yaml.tmpl
```

**Template metadata** (Configuration struct):

```go
type Configuration struct {
    Name        string            // "simple", "atmos"
    Description string            // User-facing description
    TemplateID  string            // Unique identifier
    Files       []File            // Files to generate
    Prompts     []PromptConfig    // Interactive prompts
    Variables   map[string]string // Default variables
    TargetDir   string            // Default target directory
}
```

### Template Rendering

**Go template syntax** with Gomplate and Sprig functions:

```yaml
# atmos.yaml.tmpl
components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: {{ config "auto_approve" | default "false" }}

# Access variables:
# .Config.project_name       - from --set flags
# config "key"               - helper function
# env "VAR"                  - environment variables (Gomplate)
# default "value"            - default values (Sprig)
```

### Update Flow (with 3-Way Merge)

**Key concept**: Store base content for intelligent updates

```
Initial generation:
1. Render template files
2. Write files to target directory
3. Store base content in .atmos/init/base/
4. Write metadata to .atmos/init/metadata.yaml

Update (atmos init --update):
1. Load base content from .atmos/init/base/
2. Load current files (ours - with user changes)
3. Render new template version (theirs)
4. Perform 3-way merge (base, ours, theirs)
5. Write merged content
6. Update base content for future updates
```

**Metadata format** (`.atmos/init/metadata.yaml`):

```yaml
version: 1
command: atmos init
template:
  name: simple
  version: 1.89.0
generated_at: 2025-01-15T10:30:00Z
variables:
  project_name: my-infrastructure
  aws_region: us-east-1
files:
  - path: atmos.yaml
    template: simple/atmos.yaml.tmpl
    checksum: sha256:abc123...
  - path: stacks/README.md
    template: simple/stacks/README.md
    checksum: sha256:def456...
```

**For 3-way merge details**, see: [docs/prd/three-way-merge/](./three-way-merge/)

### File Processing Pipeline

```
For each template file:
├─ 1. Render file path (path may be templated)
│     Example: "stacks/{{ .Config.environment }}/stack.yaml"
│
├─ 2. Check if file should be skipped
│     Skip if path contains: "", "false", "<no value>"
│
├─ 3. Check if file exists
│   ├─ No → Create new file
│   └─ Yes → Handle based on flags:
│       ├─ --force → Overwrite
│       ├─ --update → 3-way merge (see merge PRD)
│       └─ neither → Error (file exists)
│
├─ 4. Render file content (if IsTemplate=true)
│     Process Go templates with variables
│
└─ 5. Write file with permissions
      Create parent directories as needed
```

## Implementation Details

### Phase 1: Core Init Command (Builds on Scaffold)

**Goal**: Build `atmos init` by specializing the `atmos scaffold` infrastructure for project initialization.

**Prerequisites**:
- Requires `atmos scaffold` Phase 1 completion (see [atmos-scaffold.md](./atmos-scaffold.md))
- Specifically: Core scaffolding system with `pkg/generator` infrastructure

**Tasks**:
1. Create `cmd/init/` package
   - `init.go` - Command definition
   - `init_test.go` - Command tests
2. Create init-specific embedded templates
   - `simple` template (full project structure)
   - `atmos` template (atmos.yaml only)
3. Implement `atmos init` command
   - Interactive mode (default)
   - Non-interactive mode with arguments
   - Template selection from embedded templates
   - Variable substitution via `--set` flags
   - Force overwrite mode (`--force`)
4. Reuse `pkg/generator` infrastructure
   - Template rendering engine (from scaffold)
   - File processing (from scaffold)
   - Interactive UI/prompts (from scaffold)
5. Write comprehensive tests
   - Unit tests for command
   - Integration tests for init flows
   - Template rendering tests

**Deliverables**:
- Fully functional `atmos init` command
- Embedded init templates (simple, atmos)
- Documentation and examples

**Note**: This phase builds on scaffold, so scaffold must be implemented first.

### Phase 2: Update Support (Depends on 3-Way Merge)

**Prerequisites**:
- Requires completion of [three-way-merge PRD](./three-way-merge/)
- Specifically: Phase 3 (Base Storage & Integration)

**Tasks**:
1. Add `--update` flag to command
2. Implement base content storage
   - Store original template output in `.atmos/init/base/`
   - Write metadata to `.atmos/init/metadata.yaml`
3. Integrate 3-way merge in file handling
4. Add conflict handling UI
5. Test update scenarios

**Integration with `pkg/generator/engine/templating.go`**:

```go
// The Processor.ProcessFile() method will be enhanced to:
// - Load base content when update=true
// - Perform 3-way merge using pkg/generator/merge
// - Handle conflicts with clear error messages
// - Update base content on successful merge
```

**Deliverables**:
- `--update` flag for updating existing projects
- Base content storage system
- Conflict resolution UI
- Update documentation and examples

### Phase 3: Advanced Features (Future)

**Potential enhancements**:
- Custom template sources (Git URLs)
- Template versioning and compatibility checks
- Pre/post generation hooks
- Template validation
- Diff preview mode (`--dry-run`)

## CLI Usage Examples

### Basic Usage

```bash
# Interactive mode (default)
atmos init

# Non-interactive with template and target
atmos init simple ./my-project

# Pass variables
atmos init simple ./my-project \
  --set project_name=acme \
  --set aws_region=us-west-2 \
  --set environment=production
```

### Force Overwrite

```bash
# Overwrite existing files
atmos init simple ./my-project --force

# Useful for:
# - Testing templates
# - Resetting to template defaults
# - CI/CD environments
```

### Update Mode (Future)

```bash
# Update existing project from template
cd my-project
atmos init --update

# Auto-resolve conflicts (use template version)
atmos init --update --merge-strategy=theirs

# Auto-resolve conflicts (keep user version)
atmos init --update --merge-strategy=ours

# Preview changes without writing
atmos init --update --dry-run
```

### Non-Interactive Automation

```bash
# CI/CD pipeline
ATMOS_INIT_INTERACTIVE=false \
ATMOS_INIT_SET="project_name=my-project,aws_region=us-east-1" \
atmos init simple ./output

# Or with flags
atmos init simple ./output \
  --interactive=false \
  --set project_name=my-project \
  --set aws_region=us-east-1
```

## Error Handling

### Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `template 'foo' not found` | Invalid template name | Use `atmos init` to see available templates |
| `target directory is required in non-interactive mode` | Missing target with `--interactive=false` | Provide target directory as argument |
| `file exists` | File exists, no `--force` or `--update` | Use `--force` to overwrite or `--update` to merge |
| `merge conflicts detected` | Both user and template modified same content | Resolve conflicts manually or use `--merge-strategy` |
| `too many changes detected` | Merge exceeds threshold | Use `--max-changes=75` to allow more changes |

## Success Criteria

### Functional Requirements

- [ ] **Template selection** - Interactive and non-interactive modes work
- [ ] **Variable substitution** - `--set` flags correctly passed to templates
- [ ] **File creation** - All template files created with correct permissions
- [ ] **Force overwrite** - `--force` flag overwrites existing files
- [ ] **Update mode** - `--update` flag performs 3-way merge (Phase 2)
- [ ] **Conflict detection** - Merge conflicts clearly communicated
- [ ] **Base storage** - Original content stored for future updates (Phase 2)

### User Experience Requirements

- [ ] **Fast setup** - Complete initialization in <30 seconds
- [ ] **Clear prompts** - Interactive mode is intuitive
- [ ] **Helpful errors** - Error messages include actionable solutions
- [ ] **Preview mode** - `--dry-run` shows what would be created (Phase 3)

### Quality Requirements

- [ ] **Test coverage >80%** - Comprehensive unit and integration tests
- [ ] **Cross-platform** - Works on Linux, macOS, Windows
- [ ] **Documented** - Website documentation with examples
- [ ] **No data loss** - User customizations preserved during updates (Phase 2)

## Integration Points

### Generator Package

`atmos init` builds on the generator package (created for `atmos scaffold`):

**Package structure** (to be built):
- **`pkg/generator/setup`** - GeneratorContext creation
- **`pkg/generator/templates`** - Template registry and discovery
- **`pkg/generator/engine`** - Template processing and file operations
- **`pkg/generator/merge`** - 3-way merge logic (Phase 2, see [three-way-merge PRD](./three-way-merge/))
- **`pkg/generator/ui`** - Interactive prompts and UI

**Note**: This package will be built as part of scaffold implementation and reused by init.

### Configuration

Reads from `atmos.yaml` for template-specific settings:

```yaml
# Future: Custom template configuration
templates:
  init:
    default: simple
    search_paths:
      - ./.atmos/templates
      - ~/.atmos/templates
```

### Telemetry

Captures usage metrics (no PII):
- Template selected
- Interactive vs non-interactive mode
- Success/failure status
- Error types encountered

## Testing Strategy

### Unit Tests

```go
// cmd/init/init_test.go
TestInitCommand()              // Command registration
TestInitFlags()                // Flag parsing
TestInitInteractive()          // Interactive prompts
TestInitNonInteractive()       // Automation mode
TestInitWithVariables()        // --set flag handling

// pkg/generator/engine/templating_test.go
TestProcessFile()              // File processing
TestHandleExistingFile()       // Force/update behavior
TestTemplateRendering()        // Variable substitution
TestFilePathRendering()        // Dynamic file naming
```

### Integration Tests

```bash
# tests/test-cases/init/
test_init_simple_interactive.sh       # Interactive flow
test_init_simple_noninteractive.sh    # Automation
test_init_with_variables.sh           # Variable substitution
test_init_force_overwrite.sh          # --force flag
test_init_update_merge.sh             # --update flag (Phase 2)
test_init_update_conflicts.sh         # Conflict handling (Phase 2)
```

### Manual Testing Checklist

- [ ] Run `atmos init` without arguments → shows interactive prompts
- [ ] Select each embedded template → creates correct files
- [ ] Run with `--interactive=false` → requires template and target
- [ ] Run with `--set` flags → variables correctly substituted
- [ ] Run with `--force` → overwrites existing files
- [ ] Run with `--update` → merges changes intelligently (Phase 2)
- [ ] Trigger merge conflicts → shows clear error and resolution options (Phase 2)

## Documentation Requirements

### Website Documentation

Location: `website/docs/cli/commands/atmos-init.mdx`

**Sections**:
1. **Overview** - What the command does
2. **Usage** - Command syntax and arguments
3. **Flags** - Description of all flags
4. **Examples** - Common use cases with outputs
5. **Templates** - Available templates and structure
6. **Updating Projects** - Using `--update` flag (Phase 2)
7. **Troubleshooting** - Common errors and solutions

### Inline Documentation

- Command help text (`atmos init --help`)
- Flag descriptions
- Error messages with actionable solutions

## Dependencies

### External Libraries

- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration management
- `github.com/Masterminds/sprig/v3` - Template functions
- `github.com/hairyhenderson/gomplate/v3` - Template rendering

### Internal Packages

- `pkg/generator/*` - Generator framework (shared with scaffold)
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

### Custom Template Sources

```bash
# Git repository
atmos init https://github.com/user/atmos-template.git ./my-project

# Local directory
atmos init ~/my-templates/custom ./my-project
```

### Template Marketplace

- Discover community templates
- Rate and review templates
- Publish custom templates

### Migration Hooks

```yaml
# .atmos/init/metadata.yaml
hooks:
  pre_update:
    - script: .atmos/hooks/pre-update.sh
  post_update:
    - script: .atmos/hooks/post-update.sh
```

### Template Versioning

```bash
# Pin to specific template version
atmos init simple@1.5.0 ./my-project

# Check compatibility
atmos init --check-compatibility
```

## References

- **Three-Way Merge PRD**: [docs/prd/three-way-merge/](./three-way-merge/)
- **Command Registry Pattern**: [docs/prd/command-registry-pattern.md](./command-registry-pattern.md)
- **Generator Package**: `pkg/generator/`
- **Cruft (inspiration)**: https://github.com/cruft/cruft
- **Copier (alternative)**: https://github.com/copier-org/copier

## Related PRDs

- **atmos scaffold** - Similar command for generating components/modules
- **Three-way merge** - Core merge algorithm used by update mode
- **Template system** - Shared generator package design
