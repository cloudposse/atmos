# PRD: Three-Way Merge for File Updates

## Executive Summary

Implement a 3-way merge system that intelligently merges file updates while preserving user customizations. The system uses dual strategies: line-based text merging for general files and structure-aware YAML merging for configuration files. This merge capability is used by template update workflows in `atmos init --update` and `atmos scaffold generate --update`.

## Problem Statement

### Core Problem: Merging File Updates While Preserving Customizations

When updating files from templates or upstream sources, we need to:

1. **Apply new changes** from the template/upstream version
2. **Preserve user customizations** made to their local version
3. **Detect conflicts** when both sides modify the same content
4. **Handle different file types** intelligently (text vs structured data)

This is a fundamental problem in version control and template management. A proper 3-way merge algorithm solves this by using three versions to make intelligent merging decisions.

### Real-World Scenario: Template Updates

**Context**: User initialized project with `atmos init simple`, then template evolves with new features.

**Scenario 1: Scaffold Template Update (Terraform Component)**

```hcl
# Base: Original scaffold output (what atmos scaffold generated)
# components/terraform/vpc/main.tf
module "vpc" {
  source = "cloudposse/vpc/aws"
  version = "1.0.0"

  cidr_block = var.cidr_block
}

# Ours: User's customized version (added monitoring)
module "vpc" {
  source = "cloudposse/vpc/aws"
  version = "1.0.0"

  cidr_block = var.cidr_block

  # User added monitoring
  enable_cloudwatch_logs = true
  flow_log_destination_type = "cloud-watch-logs"
}

# Theirs: New template version (adds tags, updates version)
module "vpc" {
  source = "cloudposse/vpc/aws"
  version = "2.0.0"  # Template updated version

  cidr_block = var.cidr_block

  # Template added default tags
  tags = var.tags
}

# Desired result: Both changes preserved
module "vpc" {
  source = "cloudposse/vpc/aws"
  version = "2.0.0"  # From template

  cidr_block = var.cidr_block

  # User's customization preserved
  enable_cloudwatch_logs = true
  flow_log_destination_type = "cloud-watch-logs"

  # Template's addition preserved
  tags = var.tags
}
```

**Scenario 2: Init Template Update (atmos.yaml)**

```yaml
# Base: Original template (what atmos init generated)
components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: false

# Ours: User's customized version (changed base_path)
components:
  terraform:
    base_path: "infrastructure/terraform"  # User changed to own convention
    apply_auto_approve: false

# Theirs: New template version (adds new settings)
components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: false
    deploy_run_init: true            # Template adds new feature
    auto_generate_backend_file: true

# Desired result: User's path + template's new features
components:
  terraform:
    base_path: "infrastructure/terraform"  # User's customization preserved
    apply_auto_approve: false
    deploy_run_init: true                  # Template's new feature
    auto_generate_backend_file: true
```

**Why this matters**: Without 3-way merge, users lose customizations or templates can't evolve.

## Solution: 3-Way Merge Algorithm

### What is 3-Way Merge?

A **3-way merge** uses three versions to intelligently merge changes:

- **Base**: Original version (common ancestor)
- **Ours**: Local version (with our changes)
- **Theirs**: Remote version (with their changes)

**Core Algorithm**:
```
For each section:
├─ If both unchanged → Keep (no changes needed)
├─ If only ours changed → Use ours (we made the change)
├─ If only theirs changed → Use theirs (they made the change)
├─ If both changed identically → Use changed version (both agree)
└─ If both changed differently → CONFLICT (manual resolution needed)
```

**Key Insight**: By comparing both sides to the base, we can distinguish:
- **Intentional change** (different from base)
- **Unchanged** (same as base)
- **Deleted** (not present, but was in base)

**Benefits**:
- Preserves customizations from both sides automatically
- Reduces false conflicts (compared to 2-way diff)
- Standard approach used by Git, SVN, Mercurial, and modern VCS
- Only surfaces genuine conflicts that require human decision

### Use Cases in Atmos

This 3-way merge capability is used exclusively by **template update workflows**:

1. **`atmos scaffold generate --update`**: Update generated files from scaffold templates
   - Base: Original scaffold output (what was initially generated)
   - Ours: User's customized files (with their changes)
   - Theirs: New scaffold template version (with template updates)
   - **Examples**: Updating Terraform components, README files, workflow configs

2. **`atmos init --update`**: Update project structure from init templates
   - Base: Original init template output (initial project structure)
   - Ours: User's customized project files (atmos.yaml, stack files, etc.)
   - Theirs: New init template version (updated project structure)
   - **Examples**: Updating atmos.yaml, directory structure, documentation

**Important**: This is NOT for:
- ❌ Merging Terraform component configurations (that's stack inheritance)
- ❌ Merging stack YAML files (that's import/override system)
- ❌ Version control operations (that's Git's job)

**This IS for**:
- ✅ Keeping template-generated files up-to-date
- ✅ Preserving user customizations to generated files
- ✅ Safely evolving templates over time

## Goals

### Primary Goals

1. **Preserve user customizations** - Never lose user changes during template updates
2. **Apply template updates automatically** - Bring in new template features without manual intervention
3. **Detect conflicts explicitly** - Surface genuine conflicting changes to user with clear context
4. **Support multiple file types** - Handle text files, YAML configs, and other formats intelligently

### Non-Goals

1. **Semantic conflict resolution** - Won't understand code meaning or logical conflicts
2. **Cross-file dependency tracking** - Won't analyze relationships between files
3. **Automatic migration** - Won't auto-upgrade breaking changes in templates

## Implementation Architecture

### Dual-Strategy Approach

The merge system uses different strategies based on file type to optimize merge quality:

**Automatic strategy selection based on file extension**:

```go
func (m *ThreeWayMerger) Merge(base, ours, theirs, fileName string) (string, error) {
    if isYAMLFile(fileName) {
        return m.yamlMerger.Merge(base, ours, theirs)
    }
    return m.textMerger.Merge(base, ours, theirs)
}
```

### Strategy 1: Text-Based Merge

**For**: `.tf`, `.md`, `.sh`, `.json`, `.hcl`, and other text files

**Approach**:
- Line-by-line 3-way merge using Myers diff algorithm
- Conflict detection for overlapping changes
- Standard Git-style conflict markers
- Threshold-based safety check (reject if >50% changes by default)

**Example**:
```hcl
# Base
variable "region" {
  default = "us-east-1"
}

# Ours (user added description)
variable "region" {
  description = "AWS region"
  default = "us-east-1"
}

# Theirs (template changed default)
variable "region" {
  default = "us-west-2"
}

# Result: CONFLICT (both modified)
variable "region" {
<<<<<<< HEAD (ours)
  description = "AWS region"
  default = "us-east-1"
=======
  default = "us-west-2"
>>>>>>> template (theirs)
}
```

### Strategy 2: YAML-Aware Merge

**For**: `.yaml`, `.yml` files (config files, stack files)

**Approach**:
- Structure-aware merging at key/path level using `gopkg.in/yaml.v3` Node API
- Recursive merging of nested structures (mappings, sequences, scalars)
- Full preservation of comments, anchors, and formatting
- Dramatically fewer false conflicts than line-based merging

**Why YAML-aware merging matters**:

```yaml
# Both modify the same section, different keys

# Text-based merge: CONFLICT (lines overlap)
<<<<<<< HEAD
    base_path: "components/terraform"
    custom_var: true
=======
    base_path: "components/terraform"
    new_feature: true
>>>>>>> template

# YAML-aware merge: CLEAN MERGE (different keys)
    base_path: "components/terraform"
    custom_var: true      # from ours
    new_feature: true     # from theirs
```

**Preservation guarantees using yaml.v3**:

1. **Comments** - Via `Node.HeadComment`, `Node.LineComment`, `Node.FootComment`
   ```yaml
   # User's important note preserved
   components:
     terraform:
       custom_var: true  # inline comment preserved
   ```

2. **Anchors and aliases** - Via `Node.Anchor`, `Node.Alias`
   ```yaml
   defaults: &defaults  # anchor preserved
     enabled: true

   production:
     <<: *defaults       # alias preserved
   ```

3. **Formatting** - Via `Node.Style` (flow/literal/folded styles)

4. **Key order** - Maintains insertion order where possible

### Implementation Structure

```
pkg/generator/merge/
├── merge.go            # Main 3-way merger with auto-detection
├── merge_test.go       # Integration tests
├── text_merger.go      # Line-based text merging (Myers diff)
├── text_merger_test.go
├── yaml_merger.go      # Structure-aware YAML merging (yaml.v3)
├── yaml_merger_test.go
└── conflicts.go        # Conflict types and handling
```

### Core API

```go
package merge

// ThreeWayMerger performs 3-way merges with automatic strategy selection.
type ThreeWayMerger struct {
    thresholdPercent int         // Change threshold (0-100)
    textMerger       *TextMerger  // Line-based text strategy
    yamlMerger       *YAMLMerger  // Structure-aware YAML strategy
}

// NewThreeWayMerger creates a merger with the specified change threshold.
// thresholdPercent: Maximum percentage of changes allowed (0 = no limit, 50 = default)
func NewThreeWayMerger(thresholdPercent int) *ThreeWayMerger

// Merge performs a 3-way merge with automatic strategy selection.
// Selects YAML strategy for .yaml/.yml files, text strategy for others.
//
// Parameters:
//   base: Original template content (what was initially generated)
//   ours: User's current content (with customizations)
//   theirs: New template content (with updates)
//   fileName: File name for strategy selection and error messages
//
// Returns:
//   merged content string
//   error if conflicts detected or merge fails
func (m *ThreeWayMerger) Merge(base, ours, theirs, fileName string) (string, error)

// SetMaxChanges configures the change threshold percentage.
func (m *ThreeWayMerger) SetMaxChanges(thresholdPercent int)
```

### Conflict Handling

When conflicts are detected:

1. **Insert conflict markers** (Git-style):
   ```
   <<<<<<< HEAD (ours)
   user's version
   =======
   template's version
   >>>>>>> template (theirs)
   ```

2. **Return clear error** with context:
   ```
   failed to merge file atmos.yaml: merge conflicts detected
   Conflicts:
     - Line 15-20: Both modified 'terraform.base_path'
     - Line 45-50: Both modified 'stacks.name_pattern'

   File written with conflict markers.
   Please resolve manually or use --force to overwrite.
   ```

3. **Provide resolution options**:
   - Manual: Edit file to resolve markers
   - Use `--force`: Overwrite with template version
   - Use `--merge-strategy=ours`: Keep user version
   - Use `--merge-strategy=theirs`: Use template version

### Base Content Storage

**Requirement**: 3-way merge requires the original version (base) to make intelligent decisions.

**Storage strategy**: Store base content when files are initially generated, retrieve during updates.

**Storage location**: `.atmos/init/base/` (for init templates)

```
.atmos/
└── init/
    ├── metadata.yaml     # Tracks what was generated
    └── base/
        ├── atmos.yaml    # Original template version
        ├── stacks/
        └── components/
```

**Metadata format**:
```yaml
version: 1
generated_by: atmos init
generated_at: 2025-01-15T10:30:00Z
template:
  name: atmos
  version: 1.89.0
files:
  - path: atmos.yaml
    template: atmos/atmos.yaml
    checksum: sha256:abc123...
  - path: stacks/README.md
    template: atmos/stacks/README.md
    checksum: sha256:def456...
```

**Generic workflow** (used by init, scaffold, etc.):

1. **Initial generation**:
   - Generate files from template/source
   - Store original content as base
   - Write metadata (what was generated, when, from what source)

2. **Update operation**:
   - Load base content (original version)
   - Load current content (ours - with user changes)
   - Render/fetch new version (theirs - with updates)
   - Perform 3-way merge
   - Update base content on success

3. **No base found** (graceful degradation):
   - Fall back to 2-way diff (best effort)
   - Warn user: "No base version found, merge may lose customizations"
   - Suggest manual review of changes

## Text-Based Merge Algorithm

### Implementation

Use `github.com/hexops/gotextdiff` (already in dependencies) for Myers diff:

```go
type TextMerger struct {
    thresholdPercent int
}

func (m *TextMerger) Merge(base, ours, theirs string) (string, error) {
    // 1. Compute diffs
    baseDiff := myers.ComputeEdits(base, ours)   // What user changed
    theirsDiff := myers.ComputeEdits(base, theirs) // What template changed

    // 2. Apply both diffs, detect conflicts
    merged, conflicts := m.applyBothDiffs(base, baseDiff, theirsDiff)

    // 3. Check threshold
    if m.exceedsThreshold(base, ours, theirs) {
        return "", fmt.Errorf("too many changes detected")
    }

    // 4. Handle conflicts
    if len(conflicts) > 0 {
        return m.insertConflictMarkers(merged, conflicts),
               fmt.Errorf("merge conflicts detected: %d conflicts", len(conflicts))
    }

    return merged, nil
}
```

### Test Cases

Required test coverage:

```go
// Clean merges (no conflicts)
- Both sides add different lines at different locations
- One side modifies, other unchanged
- Both sides make identical changes
- One side adds, other deletes different section

// Conflicts (require manual resolution)
- Both sides modify same line differently
- One side deletes, other modifies
- Both sides add at same location differently
- Multiple conflict regions in same file

// Edge cases
- Empty base (initial generation)
- Empty ours (user deleted everything)
- Empty theirs (template removed file)
- All three identical (no changes)
- Large files (>10K lines) - performance
- Mixed line endings (CRLF/LF)
- Unicode content
```

## YAML-Aware Merge Algorithm

### Implementation

Use `gopkg.in/yaml.v3` Node API for structure-aware merging:

```go
type YAMLMerger struct {
    thresholdPercent int
    textMerger       *TextMerger // Fallback for invalid YAML
}

func (m *YAMLMerger) Merge(base, ours, theirs string) (string, error) {
    // 1. Parse to nodes
    var baseNode, oursNode, theirsNode yaml.Node
    if err := yaml.Unmarshal([]byte(base), &baseNode); err != nil {
        return m.textMerger.Merge(base, ours, theirs) // Fallback
    }
    yaml.Unmarshal([]byte(ours), &oursNode)
    yaml.Unmarshal([]byte(theirs), &theirsNode)

    // 2. Recursively merge nodes
    merged, conflicts, err := m.mergeNodes(&baseNode, &oursNode, &theirsNode)
    if err != nil {
        return "", err
    }

    // 3. Serialize back to YAML
    result, err := yaml.Marshal(merged)
    if err != nil {
        return "", err
    }

    // 4. Handle conflicts
    if len(conflicts) > 0 {
        return string(result), fmt.Errorf("merge conflicts detected: %d conflicts", len(conflicts))
    }

    return string(result), nil
}
```

### Recursive Merge Strategy

```go
func (m *YAMLMerger) mergeNodes(base, ours, theirs *yaml.Node) (*yaml.Node, []Conflict, error) {
    switch base.Kind {
    case yaml.MappingNode:
        return m.mergeMappings(base, ours, theirs)
    case yaml.SequenceNode:
        return m.mergeSequences(base, ours, theirs)
    case yaml.ScalarNode:
        return m.mergeScalars(base, ours, theirs)
    }
}

func (m *YAMLMerger) mergeMappings(base, ours, theirs *yaml.Node) (*yaml.Node, []Conflict, error) {
    result := &yaml.Node{Kind: yaml.MappingNode}
    conflicts := []Conflict{}

    // Build key maps for fast lookup
    baseKeys := buildKeyMap(base)
    ourKeys := buildKeyMap(ours)
    theirKeys := buildKeyMap(theirs)

    // Process all unique keys
    allKeys := union(baseKeys, ourKeys, theirKeys)

    for key := range allKeys {
        baseVal := baseKeys[key]
        ourVal := ourKeys[key]
        theirVal := theirKeys[key]

        switch {
        case ourVal == nil && theirVal == nil:
            // Both deleted - skip

        case ourVal == nil && theirVal != nil:
            // We deleted, they kept/modified
            if !nodesEqual(baseVal, theirVal) {
                // They modified after we deleted - conflict
                conflicts = append(conflicts, Conflict{
                    Type: DeletedByUs,
                    Path: key,
                })
            }

        case ourVal != nil && theirVal == nil:
            // They deleted, we kept/modified
            if !nodesEqual(baseVal, ourVal) {
                // We modified after they deleted - conflict
                conflicts = append(conflicts, Conflict{
                    Type: DeletedByThem,
                    Path: key,
                })
            }

        case nodesEqual(ourVal, theirVal):
            // Both made same change - use either
            appendKeyValue(result, key, ourVal)

        case nodesEqual(baseVal, ourVal):
            // We didn't change, they did - use theirs
            appendKeyValue(result, key, theirVal)

        case nodesEqual(baseVal, theirVal):
            // They didn't change, we did - use ours
            appendKeyValue(result, key, ourVal)

        default:
            // Both changed differently
            if canRecurse(ourVal, theirVal) {
                // Both are mappings/sequences - recurse
                merged, childConflicts, err := m.mergeNodes(baseVal, ourVal, theirVal)
                if err != nil {
                    return nil, nil, err
                }
                conflicts = append(conflicts, childConflicts...)
                appendKeyValue(result, key, merged)
            } else {
                // Scalar conflict - mark conflict
                conflicts = append(conflicts, Conflict{
                    Type: BothModified,
                    Path: key,
                })
                // Use ours by default (strategy configurable)
                appendKeyValue(result, key, ourVal)
            }
        }
    }

    // Preserve comments from ours (user's comments are valuable)
    result.HeadComment = ours.HeadComment
    result.LineComment = ours.LineComment
    result.FootComment = ours.FootComment

    return result, conflicts, nil
}
```

### Comment Preservation Rules

1. **Head comments**: Prefer user's comments (they provide context)
2. **Line comments**: Keep with their associated keys/values
3. **Foot comments**: Preserve from user unless deleted
4. **Conflict comments**: Include both versions with markers

### Test Cases

Required YAML-specific test coverage:

```go
// Structure-aware merging
- Add different keys at same level (clean merge)
- Add nested keys in different sections (clean merge)
- Modify different keys in same mapping (clean merge)
- Modify same scalar value (conflict)

// Comment preservation
- User adds comments, template adds keys (preserve both)
- Both add comments to same location (prefer user's)
- User adds inline comment (preserve)
- Template removes section with user's comments (conflict)

// Anchor preservation
- User adds anchor reference (preserve)
- Template modifies anchor definition (merge anchor content)
- Both modify anchor definition (conflict in anchor)
- User deletes anchor, template references it (conflict)

// Edge cases
- Empty mappings/sequences
- Nested structures (3+ levels deep)
- Mixed sequences and mappings
- Very large YAML files (>100KB)
- Invalid YAML (fallback to text merge)
```

## Integration Examples

### Example 1: Generator Integration

Update `pkg/generator/engine/templating.go` to use 3-way merge:

```go
func (p *Processor) handleExistingFile(file File, existingPath, targetPath string, force, update bool, ...) error {
    if update {
        // Load base version
        baseContent, err := p.loadBaseContent(file.Path)
        if err != nil {
            // No base found - warn and continue with empty base
            ui.Warning("No base version found for %s, merge may be imperfect", file.Path)
            baseContent = ""
        }

        // Perform 3-way merge
        merged, err := p.merger.Merge(baseContent, existingContent, newContent, file.Path)
        if err != nil {
            return p.handleMergeConflict(file.Path, merged, err)
        }

        // Write merged content
        if err := os.WriteFile(existingPath, []byte(merged), file.Permissions); err != nil {
            return err
        }

        // Update base content for future merges
        return p.saveBaseContent(file.Path, newContent)
    }
}

func (p *Processor) handleMergeConflict(path, content string, mergeErr error) error {
    ui.Error("Merge conflicts detected in %s", path)

    // Write file with conflict markers
    if err := os.WriteFile(path, []byte(content), 0644); err != nil {
        return err
    }

    ui.Info("File written with conflict markers. Options:")
    ui.Info("  1. Edit %s to resolve conflicts manually", path)
    ui.Info("  2. Run: atmos init --update --force (overwrite with template)")
    ui.Info("  3. Run: atmos init --update --merge-strategy=ours (keep your version)")

    return mergeErr
}
```

## CLI Flags

### New Flags

```bash
# Merge strategy
--merge-strategy=manual|ours|theirs   # Default: manual

# Change threshold
--max-changes=50                      # Percentage (0-100, default: 50)

# Preview changes
--dry-run                             # Show what would be merged
```

### Usage Examples

```bash
# Standard update with conflict resolution
atmos init --update

# Auto-resolve using template version
atmos init --update --merge-strategy=theirs

# Keep all user changes, ignore template updates
atmos init --update --merge-strategy=ours

# Allow larger changes
atmos init --update --max-changes=75

# Preview without writing
atmos init --update --dry-run
```

## Implementation Plan

### Phase 1: Text-Based Merge

**Goal**: Replace `pkg/generator/merge` with proper 3-way merge for text files.

**Tasks**:
- [ ] Add `github.com/nasdf/diff3` dependency
- [ ] Implement `TextMerger` wrapper with conflict detection
- [ ] Add threshold checking and configurable strategies
- [ ] Write 20+ test cases for text merging
- [ ] Benchmark performance on large files
- [ ] Update existing `merge.go` interface

**Deliverables**:
- `pkg/generator/merge/text_merger.go`
- `pkg/generator/merge/text_merger_test.go`
- Comprehensive test coverage (>80%)

### Phase 2: YAML-Aware Merge

**Goal**: Add structure-aware merging for YAML configuration files.

**Tasks**:
- [ ] Implement `YAMLMerger` with yaml.v3 Node API
- [ ] Add recursive mapping/sequence/scalar merge
- [ ] Implement comment preservation (HeadComment, LineComment, FootComment)
- [ ] Implement anchor and alias preservation
- [ ] Write 20+ test cases for YAML merging
- [ ] Test comment/anchor round-trip
- [ ] Add fallback to text merge for invalid YAML

**Deliverables**:
- `pkg/generator/merge/yaml_merger.go`
- `pkg/generator/merge/yaml_merger_test.go`
- Comment preservation tests
- Anchor/alias tests

### Phase 3: Base Storage & Integration

**Goal**: Store base content and integrate merge into generator.

**Tasks**:
- [ ] Implement base content storage in `.atmos/init/base/`
- [ ] Add metadata file format (`.atmos/init/metadata.yaml`)
- [ ] Update `pkg/generator/engine/templating.go` to use new merge API
- [ ] Implement `loadBaseContent()` and `saveBaseContent()` methods
- [ ] Implement conflict handling UI (error messages, resolution guidance)
- [ ] Update existing tests to use 3-way merge
- [ ] Add integration tests

**Deliverables**:
- Base storage implementation
- Updated `handleExistingFile()` method
- Conflict handling UI
- Integration tests

### Phase 4: CLI & Documentation

**Goal**: Add user-facing flags and documentation.

**Tasks**:
- [ ] Add CLI flags (--merge-strategy, --max-changes, --dry-run)
- [ ] Update `atmos init` command documentation
- [ ] Update `atmos scaffold generate` command documentation
- [ ] Write user guide for conflict resolution
- [ ] Add examples and tutorials
- [ ] Update website documentation

**Deliverables**:
- CLI flags implementation
- User documentation
- Examples in website/docs/

## Success Criteria

### Functional Requirements

- [ ] **Preserves user customizations** - User changes never lost in clean merge
- [ ] **Applies template updates** - New template features added automatically
- [ ] **Detects conflicts** - Conflicting changes surfaced with clear context
- [ ] **YAML comment preservation** - Comments maintained in YAML files
- [ ] **YAML anchor preservation** - Anchors and aliases work after merge
- [ ] **Fallback behavior** - Graceful degradation when base not found

### Performance Requirements

- [ ] **Small files (<1KB)** - Merge in <10ms
- [ ] **Medium files (1-100KB)** - Merge in <100ms
- [ ] **Large files (>100KB)** - Merge in <1s

### Quality Requirements

- [ ] **Test coverage >80%** - High confidence in merge correctness
- [ ] **No data loss** - User changes never silently discarded
- [ ] **Clear error messages** - Users understand conflicts

## Open Questions

1. **Sequence merging in YAML** - How to handle list additions/removals? (Use positional matching or semantic matching?)
2. **Binary files** - Should we detect and skip binary files in merge? (Yes, check for null bytes)
3. **Per-file strategies** - Allow configuration of merge strategy per file pattern? (Future enhancement)
4. **Git integration** - Use `git merge-file` if available for better merge quality? (Consider for Phase 2)

## Prior Art and References

### Template Update Systems

**Cruft** (Primary Inspiration):
- Repository: https://github.com/cruft/cruft
- Python wrapper for Cookiecutter that adds update functionality
- Stores template metadata in `.cruft.json` (template URL, commit hash, variables)
- Uses `git apply -3` for 3-way merge, falls back to `.rej` files for conflicts
- **Key Learning**: Store base version reference (commit hash) to enable intelligent updates

**Copier** (Alternative Approach):
- Repository: https://github.com/copier-org/copier
- Built-in templating with update support
- Stores answers in `.copier-answers.yml` with `_commit` field
- Uses regeneration approach: generate fresh → calculate diff → reapply
- **Key Learning**: Inline conflict markers preferred over `.rej` files

### Go Libraries for 3-Way Merge

**Recommended for Phase 1: nasdf/diff3** (simple, academically sound):
```go
import "github.com/nasdf/diff3"

merged := diff3.Merge(base, ours, theirs)
```
- Based on academic paper (formal correctness)
- Simpler API (single function)
- Stable v1.0.0 release
- Pure Go, no external dependencies
- Repository: https://github.com/nasdf/diff3

**Alternative: epiclabs-io/diff3** (more features if needed):
```go
import "github.com/epiclabs-io/diff3"

result, err := diff3.Merge(baseReader, oursReader, theirsReader, detailed, "ours", "theirs")
```
- Most actively maintained (commit November 15, 2024)
- Pure Go implementation, no CGO
- Feature-rich API (streams, conflict detection, labels)
- Zero open issues (responsive maintenance)
- Repository: https://github.com/epiclabs-io/diff3

**For reference: git merge-file** (not recommended - requires git binary):
```bash
git merge-file -p --diff3 -L ours -L base -L theirs ours.txt base.txt theirs.txt
```
- Most battle-tested (uses Git's implementation)
- Requires git binary installed
- File I/O overhead
- Exit code 0 = clean, 1 = conflicts

**Advanced: git2go** (libgit2 bindings):
```go
import "github.com/libgit2/git2go/v34"

result, err := git.MergeFile(ancestor, ours, theirs, &git.MergeFileOptions{})
```
- Full Git functionality in Go
- Supports rename detection, merge strategies
- Requires CGO and libgit2
- Repository: https://github.com/libgit2/git2go

### Academic and Technical References

**Diff3 Algorithm**:
- "A Formal Investigation of Diff3" - Khanna, Kunal, Pierce (2007)
- Paper: https://www.cis.upenn.edu/~bcpierce/papers/diff3-short.pdf
- Defines formal properties of 3-way merge for well-separated regions

**Git Merge Documentation**:
- Git merge: https://git-scm.com/docs/git-merge
- Git merge-file: https://git-scm.com/docs/git-merge-file
- Git merge strategies (ort, recursive, etc.)
- Conflict styles (merge, diff3, zdiff3)

**YAML Merging**:
- Kubernetes Strategic Merge Patch: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-api-machinery/strategic-merge-patch.md
- yaml.v3 Node API: https://pkg.go.dev/gopkg.in/yaml.v3
- Structure-aware merging for configuration files

### Implementation Recommendations

**Phase 1 (Text Merge)**: Use `nasdf/diff3`
- Pure Go, no external dependencies
- Academically sound algorithm
- Simple API, easy to integrate

**Phase 2 (YAML Merge)**: Custom implementation with `gopkg.in/yaml.v3`
- Structure-aware merging at key level
- Comment and anchor preservation
- Reduces false conflicts for config files

**Future Consideration**: `git2go` if we need:
- Rename detection across file moves
- Advanced merge strategies
- Full Git compatibility

### Key Learnings Applied

From Cruft:
- ✅ Store base version (commit/checksum) for future updates
- ✅ Two-stage conflict handling (try smart merge, fallback to markers)
- ✅ Clear metadata format (`.atmos/init/metadata.yaml`)

From Copier:
- ✅ Inline conflict markers (not separate `.rej` files)
- ✅ Support both git and non-git workflows
- ✅ Migration hooks (future: pre/post update scripts)

From Git:
- ✅ diff3 conflict style (shows base version)
- ✅ Clear exit codes (0 = clean, 1 = conflicts)
- ✅ Configurable merge strategies (manual/ours/theirs)

From Diff3 Paper:
- ✅ Well-separated edits never conflict
- ✅ Longest common subsequence (LCS) based algorithm
- ✅ Formal correctness guarantees
