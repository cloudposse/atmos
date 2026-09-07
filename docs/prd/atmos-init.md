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

**Status**: `atmos init` is implemented and shipped, including Phase 1 (core init
command) and Phase 2 (`--update` with a real 3-way merge). This PRD was originally
written before the feature existed; the sections below have been updated where
they described the shipped feature as not existing, but this remains primarily a
historical design document — see `cmd/init/init.go` and `pkg/generator/` for the
source of truth on current behavior.

**What exists today**:
- `atmos init` command with embedded templates (`simple`, `atmos`)
- Interactive and non-interactive (`--interactive=false`) project setup
- `--force`, `--update`, `--base-ref`, `--merge-strategy`, `--skip-hooks` flags
- `pkg/generator` package (shared with `atmos scaffold`)
- Because `atmos init` shares `atmos scaffold`'s `pkg/generator/ui.InitUI` code
  path, it also inherits `spec.fields[].when:`, `spec.files[].when:`, and
  generic `spec.hooks:` (`before.scaffold.generate`/`after.scaffold.generate`,
  reusing `pkg/hooks.Hook`'s vocabulary) from a project template's
  `scaffold.yaml` — see `docs/prd/atmos-scaffold.md` for the full schema

**Still not implemented** (a real, open gap — not just historical planning):
- ❌ **`--dry-run` on `atmos init`.** Unlike `atmos scaffold generate`, `atmos
  init` has no `--dry-run` flag at all today (confirmed by `grep` over
  `cmd/init/init.go`). Any example below showing `atmos init --update --dry-run`
  describes a still-unimplemented combination.
- ❌ A `--max-changes` CLI flag — the merger has an internal conflict-percentage
  threshold (hardcoded default, currently 50%), but it isn't exposed as a flag
- ❌ Custom template sources (Git URLs), template versioning/compatibility checks
- ❌ `pre_update`/`post_update` *migration* hooks scoped to `.atmos/init/metadata.yaml`
  (see "Migration Hooks" under Future Enhancements) — a distinct, still-unbuilt
  concept from the generic `spec.hooks:` inherited from `atmos scaffold` above,
  which fire on every generate/update, not specifically on migration

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
  --update                 Update an existing project via a 3-way merge (requires a git base; see --base-ref)
  --base-ref               Git ref to use as the 3-way merge base with --update (defaults to HEAD)
  --set key=value          Set template variables
  --merge-strategy         Conflict resolution strategy for --update (manual|ours|theirs; default: manual)
  --max-changes            Maximum change threshold percentage (NOT IMPLEMENTED as a flag — internal default is hardcoded)
  --dry-run                NOT IMPLEMENTED — atmos init has no --dry-run flag today (unlike atmos scaffold generate)
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

**Key concept**: The merge base is read directly from git — there is no
on-disk base snapshot or metadata file.

```
Initial generation:
1. Render template files
2. Write files to target directory

Update (atmos init --update):
1. Resolve --base-ref (defaults to HEAD) in the target directory's git repository
2. Load each file's base content directly from that git ref
   (pkg/generator/storage.GitBaseStorage.LoadBase reads the blob straight out
   of git — no `.atmos/init/base/` snapshot is written or read)
3. Load current files (ours - with user changes)
4. Render new template version (theirs)
5. Perform 3-way merge (base, ours, theirs), honoring --merge-strategy
   (manual/ours/theirs) for any genuine conflict
6. Write merged content (skipped in a hypothetical --dry-run; note `atmos init`
   has no --dry-run flag today, unlike `atmos scaffold generate --update --dry-run`)
```

**Note**: this shipped as a git-ref-based design, not the `.atmos/init/base/` +
`.atmos/init/metadata.yaml` file-snapshot approach originally sketched in this
PRD — there is no on-disk base storage or metadata file in the current
implementation. The `metadata.yaml` format below is retained for historical
context only; it does not exist in the shipped code.

**Historical metadata format** (never implemented — `.atmos/init/metadata.yaml`):

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
│   └─ Yes → Handle based on flags (--update takes precedence over --force
│       when both are set — see handleExistingFile in
│       pkg/generator/engine/templating.go):
│       ├─ --update → 3-way merge (see merge PRD)
│       ├─ --force (and not --update) → Overwrite
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

### Phase 2: Update Support (Shipped)

**Status**: Implemented. The original plan below described a file-snapshot base
store (`.atmos/init/base/`, `.atmos/init/metadata.yaml`); what shipped instead
is a git-ref-based base (`--base-ref`, defaulting to `HEAD`, resolved via
`pkg/generator/storage.GitBaseStorage`) with no on-disk snapshot or metadata
file.

**What shipped**:
1. `--update` (and `--base-ref`) flags on the command
2. 3-way merge integrated into file handling (`pkg/generator/merge`), with
   `--merge-strategy=manual|ours|theirs` for conflict resolution
3. Path-traversal and symlink-write protection (`validateWriteTarget` in
   `pkg/generator/engine/templating.go`)
4. Test coverage for update scenarios

**Not shipped from the original plan**: the `.atmos/init/base/` +
`metadata.yaml` file store (kept above for historical context only — the
shipped design reads bases directly from git instead), and `--dry-run` support
for `atmos init` (still absent; see "Current State" above).

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
- ✅ **Shipped** (inherited from `atmos scaffold`): generic pre/post generation
  hooks via a project template's `spec.hooks:` — see "Current State" above
- Template validation
- Diff preview mode (`--dry-run`) — this is the same gap noted in "Current
  State" above: `atmos scaffold generate` got `--dry-run` (including a real
  dry-run merge preview with `--update`), but `atmos init` has not

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

### Update Mode

```bash
# Update existing project from template (base = HEAD by default)
cd my-project
atmos init --update

# Use a specific git ref as the merge base instead of HEAD
atmos init --update --base-ref=v1.2.0

# Auto-resolve conflicts (use template version)
atmos init --update --merge-strategy=theirs

# Auto-resolve conflicts (keep user version)
atmos init --update --merge-strategy=ours

# NOT SUPPORTED TODAY: atmos init has no --dry-run flag (unlike
# `atmos scaffold generate --update --dry-run`, which does support this).
```

**`--force` and `--update` together**: `--update` takes precedence. If both
flags are set, existing files go through the 3-way merge path, not a raw
overwrite (see `handleExistingFile` in `pkg/generator/engine/templating.go`).

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
| `too many changes detected` | Merge exceeds the internal conflict-percentage threshold | No CLI flag to raise the threshold today (`--max-changes` is not implemented) — resolve conflicts or use `--merge-strategy=ours`/`theirs` |
| `resolved path escapes target directory` | A template path (or a symlink in the target directory) would write outside the target directory | Check for symlinks redirecting outside the target; this is a hard-stop safety guard, not a flag to bypass |

**Path confinement**: every file write is confined under the resolved target
directory. `validateWriteTarget` (`pkg/generator/engine/templating.go`) resolves
symlinks in the write path and rejects both writes that escape the target
directory and writes through a symlink at the destination itself.

## Success Criteria

### Functional Requirements

- [x] **Template selection** - Interactive and non-interactive modes work
- [x] **Variable substitution** - `--set` flags correctly passed to templates
- [x] **File creation** - All template files created with correct permissions
- [x] **Force overwrite** - `--force` flag overwrites existing files
- [x] **Update mode** - `--update` flag performs 3-way merge (shipped)
- [x] **Conflict detection** - Merge conflicts clearly communicated
- [x] **Base retrieval** - Base content read from git via `--base-ref` (shipped;
      not the file-snapshot store originally planned here)

### User Experience Requirements

- [ ] **Fast setup** - Complete initialization in <30 seconds
- [ ] **Clear prompts** - Interactive mode is intuitive
- [ ] **Helpful errors** - Error messages include actionable solutions
- [ ] **Preview mode** - `--dry-run` shows what would be created (still not
      implemented for `atmos init`; see "Current State" above)

### Quality Requirements

- [ ] **Test coverage >80%** - Comprehensive unit and integration tests
- [ ] **Cross-platform** - Works on Linux, macOS, Windows
- [ ] **Documented** - Website documentation with examples
- [x] **No data loss** - User customizations preserved during updates (shipped)

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
test_init_update_merge.sh             # --update flag (shipped)
test_init_update_conflicts.sh         # Conflict handling (shipped)
```

### Manual Testing Checklist

- [ ] Run `atmos init` without arguments → shows interactive prompts
- [ ] Select each embedded template → creates correct files
- [ ] Run with `--interactive=false` → requires template and target
- [ ] Run with `--set` flags → variables correctly substituted
- [ ] Run with `--force` → overwrites existing files
- [ ] Run with `--update` → merges changes intelligently (shipped; verify manually)
- [ ] Trigger merge conflicts → shows clear error and resolution options (shipped; verify manually)

## Documentation Requirements

### Website Documentation

Location: `website/docs/cli/commands/init.mdx`

**Sections**:
1. **Overview** - What the command does
2. **Usage** - Command syntax and arguments
3. **Flags** - Description of all flags
4. **Examples** - Common use cases with outputs
5. **Templates** - Available templates and structure
6. **Updating Projects** - Using `--update` flag (shipped)
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

Distinct from the generic `spec.hooks:` block already shipped in a project
template's `scaffold.yaml` (see "Current State" above, inherited from `atmos
scaffold`) — this would be a project-record-scoped hook keyed to the *update*
lifecycle specifically, not a template-authored, every-generate hook:

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
