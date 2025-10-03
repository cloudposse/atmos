# PRD: Line-Level Provenance Tracking System

## Executive Summary

Implement a comprehensive provenance tracking system that records the file and line number for **every value** in Atmos configurations, not just top-level keys. The system will be generic and reusable across all Atmos configuration types (stack configs, `atmos.yaml`, vendor manifests, workflows) and will replace the current limited `sources` implementation.

## Background

### Current State

Atmos currently has two mechanisms for tracking configuration origins:

1. **`sources` system** (`pkg/schema/schema.go`, `ConfigSources`):
   - Tracks only top-level section keys (e.g., `vars.name`, `settings.templates`)
   - No line number tracking, only file paths
   - Data structure is verbose and pollutes output (750+ lines)
   - Only works for stack component configurations
   - Cannot track nested values, array elements, or map entries

2. **`MergeContext` system** (`pkg/merge/merge_context.go`):
   - Tracks import chains for error reporting
   - Records file paths but no line numbers
   - Used for formatting helpful error messages
   - Not used for provenance display

### Problems

1. **Incomplete tracking**: Users cannot determine where nested values come from
   ```yaml
   vars:
     tags:
       environment: dev  # Which file set this?
     availability_zones:
       - us-east-2a  # Which file added this element?
   ```

2. **No line precision**: File path without line numbers makes debugging difficult in large files

3. **Output pollution**: Raw `sources` data adds hundreds of lines to `describe component` output

4. **Not reusable**: Cannot track provenance for `atmos.yaml` imports, vendor configs, or workflows

5. **Broken two-column display**: Current implementation can't align provenance with complex YAML structures

6. **Tests didn't catch issues**: The initial implementation had fundamental bugs that tests didn't detect

## Requirements

### Functional Requirements

1. **Line-level granularity**:
   - Track file:line:column for every value (primitives, arrays, maps, nested structures)
   - Support tracking individual array elements
   - Support tracking individual map entries
   - Support deeply nested structures (unlimited depth)

2. **Full inheritance chain**:
   - Show complete chain: base → mixin1 → mixin2 → final value
   - Indicate override vs. merge vs. new value
   - Mark computed values (templates, functions)

3. **Universal coverage**:
   - Stack component configurations (current focus)
   - `atmos.yaml` configuration imports
   - Vendor manifest (`vendor.yaml`) origins
   - Workflow definitions
   - Future: Any YAML/JSON merging scenario

4. **Multiple output formats**:
   - Two-column display (TTY): YAML/JSON on left, file:line on right, perfectly aligned
   - Inline comments (headless/piping): `key: value  # from: file.yaml:42`
   - JSON embedded: `{value, __provenance: {file, line, chain}}`

5. **Clean output**:
   - Provenance data separate from configuration data
   - No pollution of `describe` output when provenance not requested
   - Filter `sources` and `component_info` when `--provenance` enabled

### Non-Functional Requirements

1. **Performance**: <10% overhead when provenance tracking enabled
2. **Memory**: <50MB additional memory for typical projects (100-200 components)
3. **Generic**: Interface-based design for extensibility
4. **Optional**: Can be completely disabled for performance-critical scenarios
5. **Backward compatible**: Existing `sources` continue to work during deprecation period

### User Experience Requirements

1. **Intuitive**: Clear, easy-to-read provenance information
2. **Helpful error messages**: Integration with `MergeContext` for better error reporting
3. **IDE integration ready**: Structured output suitable for IDE plugins
4. **Documentation**: Comprehensive examples and use cases

## Technical Design

### Architecture Overview

The provenance system extends the existing `MergeContext` with value-level tracking capabilities.

```
┌─────────────────────────────────────────────────────────────┐
│ YAML Parser (gopkg.in/yaml.v3 Node API)                    │
│ - Preserves line/column numbers                            │
│ - Builds AST with position info                            │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ Stack Processor with Provenance-Aware Merge                │
│ - Intercepts all merge operations                          │
│ - Records source for each value via MergeContext            │
│ - Builds inheritance chains                                │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ MergeContext with Provenance Tracking                      │
│ - Stores file:line:column for each value path              │
│ - Uses JSONPath for addressing nested values               │
│ - Maintains inheritance chains                             │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ Provenance Renderers                                        │
│ - TwoColumnRenderer: TTY with alignment                    │
│ - InlineCommentRenderer: Headless/piping                   │
│ - JSONEmbeddedRenderer: Structured output                  │
└─────────────────────────────────────────────────────────────┘
```

### Data Structures

#### Enhanced MergeContext

```go
// pkg/merge/merge_context.go

type ProvenanceEntry struct {
    File      string   // Source file path
    Line      int      // Line number (1-indexed)
    Column    int      // Column number (1-indexed)
    Type      string   // "import", "inline", "override", "computed", "default"
    ValueHash string   // Hash of value for change detection
}

type MergeContext struct {
    // Existing fields
    CurrentFile   string
    ImportChain   []string
    ParentContext *MergeContext

    // NEW: Provenance tracking
    Provenance    map[string][]ProvenanceEntry  // JSONPath → inheritance chain
}

// Record provenance during merge
func (mc *MergeContext) RecordProvenance(path string, value any, entry ProvenanceEntry) {
    if mc.Provenance == nil {
        mc.Provenance = make(map[string][]ProvenanceEntry)
    }
    mc.Provenance[path] = append(mc.Provenance[path], entry)
}

// Get full provenance chain for a path
func (mc *MergeContext) GetProvenance(path string) []ProvenanceEntry {
    return mc.Provenance[path]
}

// Get all paths with provenance
func (mc *MergeContext) GetProvenancePaths() []string {
    paths := make([]string, 0, len(mc.Provenance))
    for path := range mc.Provenance {
        paths = append(paths, path)
    }
    return paths
}

// Check if provenance exists for a path
func (mc *MergeContext) HasProvenance(path string) bool {
    _, exists := mc.Provenance[path]
    return exists
}
```

#### JSONPath Addressing

Use JSONPath format to address any value in the configuration:

- Simple key: `vars.name`
- Nested key: `vars.tags.environment`
- Array element: `vars.availability_zones[0]`
- Map in array: `settings.validation.schemas[0].path`

### Implementation Phases

#### Phase 1: Enhance MergeContext (2-3 days)

**Goal**: Add provenance tracking capabilities to MergeContext

**Tasks**:
1. Add `Provenance` map to `MergeContext` struct
2. Implement `RecordProvenance()`, `GetProvenance()`, `HasProvenance()` methods
3. Add `ProvenanceEntry` struct with file, line, column, type fields
4. Unit tests for provenance storage and retrieval
5. Benchmark tests for performance impact

**Deliverables**:
- Enhanced `pkg/merge/merge_context.go`
- Unit tests with >90% coverage
- Performance benchmarks

#### Phase 2: YAML Parser Integration (3-4 days)

**Goal**: Extract line/column numbers during YAML parsing

**Tasks**:
1. Modify YAML parsing to use `gopkg.in/yaml.v3` Node API (already in use)
2. Create mapping from parsed nodes to line/column positions
3. Pass position info to merge operations
4. Handle multi-document YAML files
5. Integration tests with test fixtures

**Deliverables**:
- Modified YAML parsing in `pkg/utils/yaml_utils.go`
- Position tracking for all YAML constructs
- Integration tests

#### Phase 3: Stack Processor Integration (3-4 days)

**Goal**: Record provenance during stack configuration merging

**Tasks**:
1. Modify deep merge functions to accept and update `MergeContext`
2. Record provenance at every merge operation
3. Generate JSONPath for nested values
4. Handle array merging (append, replace, merge)
5. Mark template-evaluated values as "computed"
6. Pass `MergeContext` through entire stack processing pipeline

**Deliverables**:
- Modified `internal/exec/stack_processor_utils.go`
- Provenance tracking in all merge paths
- End-to-end tests for stack processing

#### Phase 4: Provenance Renderers (4-5 days)

**Goal**: Implement multiple rendering formats

**Tasks**:
1. **Two-column renderer**:
   - Calculate terminal width
   - Align provenance with YAML lines
   - Handle multi-line values
   - Syntax highlighting for both columns

2. **Inline comment renderer**:
   - Insert `# from:` comments at end of lines
   - Handle arrays and maps
   - Compact format for multiple sources

3. **JSON embedded renderer**:
   - Add `__provenance` fields
   - Preserve JSON structure
   - Support for nested provenance

**Deliverables**:
- `pkg/provenance/renderer_two_column.go`
- `pkg/provenance/renderer_inline.go`
- `pkg/provenance/renderer_json.go`
- Visual regression tests
- Snapshot testing

#### Phase 5: CLI Integration (2-3 days)

**Goal**: Expose provenance via CLI commands

**Tasks**:
1. Add `--provenance` flag to `describe component`
2. Add `--provenance` flag to `describe stacks`
3. Filter `sources` and `component_info` when provenance enabled
4. Update help text and examples
5. Integration tests

**Deliverables**:
- Modified `cmd/describe_component.go`
- Modified `cmd/describe_stacks.go`
- CLI integration tests

#### Phase 6: Extensibility (3-4 days)

**Goal**: Enable provenance for other config types

**Tasks**:
1. Add provenance to `atmos.yaml` loading
2. Add provenance to vendor manifest processing
3. Add provenance to workflow definitions
4. Generic interface for adding new config types
5. Documentation and examples

**Deliverables**:
- Provenance for all config types
- Generic `Trackable` interface
- Migration guide

**Total Estimated Timeline**: 3-4 weeks

## Use Cases

### 1. Debugging Configuration Values

**Scenario**: A user sees an unexpected value and wants to know where it came from.

```bash
$ atmos describe component vpc -s prod-ue2 --provenance

vars:
  cidr: "10.100.0.0/16"  # from: stacks/prod/networking.yaml:15
                          #       overrides: stacks/base/defaults.yaml:42
```

### 2. Auditing Security Settings

**Scenario**: Compliance team needs to verify origins of security-related settings.

```bash
$ atmos describe component bastion -s prod-ue2 --provenance --query .vars.security

security:
  allowed_cidr_blocks:  # from: stacks/prod/security.yaml:8-12
    - "10.0.0.0/8"      #   line 9
    - "172.16.0.0/12"   #   line 10
```

### 3. Understanding Inheritance

**Scenario**: A developer wants to understand how values are inherited and merged.

```bash
$ atmos describe component app -s staging-uw2 --provenance --format=json

{
  "vars": {
    "replicas": 3,
    "__provenance": {
      "replicas": [
        {"file": "stacks/base/app.yaml", "line": 15, "type": "import", "value": 1},
        {"file": "stacks/staging/app.yaml", "line": 22, "type": "override", "value": 3}
      ]
    }
  }
}
```

### 4. Tracking atmos.yaml Configuration

**Scenario**: User wants to see where `atmos.yaml` settings come from.

```bash
$ atmos config show --provenance

components:
  terraform:
    base_path: "components/terraform"  # from: atmos.yaml:5
    command: "tofu"                    # from: atmos.yaml.override:12
```

## Backward Compatibility

### Deprecation Strategy

1. **Phase 1 (Immediate)**:
   - New `--provenance` flag available
   - Old `sources` still present in output
   - Add deprecation warning to docs

2. **Phase 2 (After 6 months)**:
   - Add warning message when `sources` appears in output
   - Recommend migration to `--provenance`

3. **Phase 3 (After 12 months, v2.0.0)**:
   - Remove `sources` from output
   - Breaking change documented in release notes
   - Migration guide available

### Migration Path

Users currently relying on `sources`:

1. **CLI users**: Use `--provenance` instead
2. **Automation**: Parse `--provenance --format=json` output
3. **Documentation**: Update to show provenance examples

## Testing Strategy

### Unit Tests

- MergeContext provenance methods (>95% coverage)
- YAML parser position tracking
- JSONPath generation
- Each renderer type

### Integration Tests

- Stack processing with provenance
- Multiple inheritance scenarios
- Complex nested structures
- Array and map merging

### Visual Tests

- Snapshot testing for two-column rendering
- Terminal width variations
- Syntax highlighting

### Performance Tests

- Benchmark overhead with provenance enabled
- Memory usage with large projects
- Comparison with/without provenance

### Regression Tests

- Ensure existing functionality unchanged
- Test all `describe` command variations

## Success Criteria

1. ✅ Every line in configuration output has provenance information available
2. ✅ Provenance works for stacks, atmos.yaml, vendor.yaml, workflows
3. ✅ Performance overhead <10% when enabled
4. ✅ Two-column rendering aligns perfectly with complex YAML
5. ✅ Inline rendering is clean and readable
6. ✅ JSON embedding preserves structure
7. ✅ >90% test coverage on new code
8. ✅ Documentation complete with examples
9. ✅ Migration guide clear and tested
10. ✅ Users report improved debugging experience

## References

- Current `sources` implementation: `pkg/schema/schema.go` lines 686-701
- MergeContext: `pkg/merge/merge_context.go`
- YAML parser: Uses `gopkg.in/yaml.v3` with Node API
- Similar systems:
  - Terraform: Resource traceback with line numbers
  - Kubernetes: Field path tracking in validation errors
  - Ansible: Task provenance in playbooks

## Alternatives Considered

### Alternative 1: Keep Current sources System

**Pros**: No work needed, existing code
**Cons**: Doesn't meet requirements, user dissatisfaction, limited use cases
**Decision**: Rejected - doesn't solve the problem

### Alternative 2: External Tool for Provenance

**Pros**: Doesn't complicate Atmos codebase
**Cons**: Poor UX, requires separate installation, limited integration
**Decision**: Rejected - should be built-in feature

### Alternative 3: Build Separate System Instead of Extending MergeContext

**Pros**: Clean separation of concerns
**Cons**: Duplicates import chain tracking, harder to maintain
**Decision**: Rejected - MergeContext is ideal foundation

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|-----------|
| Performance overhead too high | High | Benchmark early, optimize hot paths, make optional |
| Complex YAML structures don't align | Medium | Extensive testing, visual regression tests |
| Line numbers become inaccurate after processing | Medium | Track transformations, mark computed values |
| Backward compatibility issues | Medium | Thorough testing, clear deprecation timeline |
| Increased memory usage | Low | Efficient storage, lazy loading, optional tracking |

## Future Enhancements

1. **IDE Integration**: Language server protocol support for provenance hover
2. **Web UI**: Visual provenance browser for complex configs
3. **Diff Mode**: Show provenance changes between versions
4. **Search by File**: Find all values from a specific file
5. **Blame Integration**: Link to git blame for source files
