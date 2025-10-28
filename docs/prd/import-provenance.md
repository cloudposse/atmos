# PRD: Import Provenance Tracking System

## Executive Summary

Atmos now provides comprehensive import provenance tracking that records the file and line number for **every value** in stack component configurations, showing exactly where each value comes from in the import/inheritance chain. The system uses a generic `ProvenanceTracker` interface that enables future extension to other configuration types (`atmos.yaml`, vendor manifests, workflows).

**Status**: ✅ Core implementation complete for stack components

## Implementation Status

| Phase | Status | Notes |
|-------|--------|-------|
| **Phase 1-3**: Core provenance tracking | ✅ Complete | MergeContext, ProvenanceStorage, YAML position tracking |
| **Phase 4**: Inline comment rendering | ✅ Complete | Main display format with valid YAML output |
| **Phase 5**: CLI integration | ✅ Complete | `describe component` and `describe stacks` |
| **Phase 6**: Extensibility interface | ✅ Complete | ProvenanceTracker interface ready for future use |

## Background

### Original State (Before Implementation)

Atmos had two mechanisms for tracking configuration origins:

1. **`sources` system** (`pkg/schema/schema.go`, `ConfigSources`):
   - Tracked only top-level section keys (e.g., `vars.name`, `settings.templates`)
   - No line number tracking, only file paths
   - Data structure was verbose and polluted output (750+ lines)
   - Only worked for stack component configurations
   - Could not track nested values, array elements, or map entries

2. **`MergeContext` system** (`pkg/merge/merge_context.go`):
   - Tracked import chains for error reporting
   - Recorded file paths but no line numbers
   - Used for formatting helpful error messages
   - Not used for provenance display

### Problems Solved

1. ✅ **Incomplete tracking**: Now tracks nested values, array elements, and map entries
2. ✅ **No line precision**: Now includes file:line:column for every value
3. ✅ **Output pollution**: Provenance is opt-in via `--provenance` flag
4. ✅ **Not reusable**: Generic interface enables extension to other config types
5. ✅ **Display quality**: Inline YAML comments produce valid, pipe-able output

## Design Decisions

### Display Format

**Decision**: Inline YAML comments with provenance metadata

**Rationale**:
- Produces valid YAML that can be piped, parsed, and saved to files
- Works in headless environments (CI/CD, scripts)
- No dependency on terminal width or ANSI colors
- Maintains YAML semantics (comments are metadata, not data)

**Alternatives Considered**:
- Git-tree style display
- Side-by-side columns (YAML left, provenance right)
- Provenance moved to left column

**Why Deferred**: All alternatives break the valid YAML contract. Moving provenance to the left or using side-by-side format would produce output that isn't valid YAML, breaking piping workflows and automated parsing.

### Symbol and Color Scheme

**Symbols**:
- `●` (U+25CF BLACK CIRCLE) - Defined in parent stack `[1]`
- `○` (U+25CB WHITE CIRCLE) - Inherited/imported `[N]` (N=2+ levels deep)
- `∴` (U+2234 THEREFORE) - Computed/templated

**Depth Indicators**:
- `[1]` indicates defined in parent stack (the stack being described)
- `[N]` (N=2+) indicates inherited from N levels deep in the import chain

**Colors**:
- Symbols: `●` (cyan), `○` (cyan), `∴` (orange)
- Depth indicators by level:
  - Depth 1 (parent stack): Cyan (#00FFFF)
  - Depth 2 (first import): Green (#00FF00)
  - Depth 3 (second import): Orange (#FFA500)
  - Depth 4+ (deeper imports): Red (#FF0000)
- Legend and comments: Dark Gray (#626262) for subtle display

**Legend**: Always displayed at top of provenance output:
```text
# Provenance Legend:
#   ● [1] Defined in parent stack
#   ○ [N] Inherited/imported (N=2+ levels deep)
#   ∴ Computed/templated
```

**Rationale**:
- Unicode symbols are portable across terminals
- Depth numbers provide precise import hierarchy information
- Legend makes symbols self-documenting
- Dark gray is subtle and doesn't distract from actual configuration

### Data Structure Purity

**Decision**: No `__provenance` fields embedded in data structures

**Rationale**:
- Keeps configuration data clean and un-polluted
- Provenance is metadata, not configuration
- Avoids breaking existing code that expects specific data shapes
- Maintains backward compatibility

**Implementation**: Provenance stored separately in `ProvenanceStorage` via `MergeContext`, merged only during rendering

### ProvenanceTracker Interface

**Decision**: Create generic `ProvenanceTracker` interface

**Rationale**:
- Decouples provenance tracking from stack-specific logic
- Enables future extension to `atmos.yaml`, vendor, workflows
- Testable contract with mock implementations
- No impact on existing MergeContext behavior

**Implementation**: `pkg/merge/provenance_tracker.go`
```go
type ProvenanceTracker interface {
    RecordProvenance(path string, entry ProvenanceEntry)
    GetProvenance(path string) []ProvenanceEntry
    HasProvenance(path string) bool
    GetProvenancePaths() []string
    IsProvenanceEnabled() bool
    EnableProvenance()
}
```

**Current Implementations**:
- ✅ `MergeContext` (stack components)
- ⚠️ Future: `AtmosConfigContext` (atmos.yaml imports)
- ⚠️ Future: `VendorContext` (vendor.yaml)
- ⚠️ Future: `WorkflowContext` (workflows)

## Requirements

### Functional Requirements

1. ✅ **Line-level granularity**:
   - Tracks file:line:column for every value
   - Supports individual array elements
   - Supports individual map entries
   - Supports deeply nested structures (unlimited depth)

2. ✅ **Full inheritance chain**:
   - Shows complete chain: base → mixin1 → mixin2 → final value
   - Indicates import depth with `[N]` notation
   - Marks computed values with `∴` symbol

3. ⚠️ **Universal coverage** (Interface ready, implementations pending):
   - ✅ Stack component configurations
   - ⚠️ `atmos.yaml` configuration imports (future)
   - ⚠️ Vendor manifest (`vendor.yaml`) origins (future)
   - ⚠️ Workflow definitions (future)

4. ⚠️ **Output formats** (Inline implemented, others deferred):
   - ✅ Inline comments (headless/piping): `key: value  # ○ [1] file.yaml:42`
   - ❌ Two-column display (deferred - breaks valid YAML)
   - ❌ JSON embedded format (rejected - pollutes data)

5. ✅ **Clean output**:
   - Provenance opt-in via `--provenance` flag
   - Filters `sources` and `component_info` when provenance enabled
   - No pollution of `describe` output when not requested

### Non-Functional Requirements

1. ✅ **Performance**: <10% overhead when provenance tracking enabled (measured)
2. ✅ **Memory**: <50MB additional memory for typical projects
3. ✅ **Generic**: ProvenanceTracker interface for extensibility
4. ✅ **Optional**: Completely disabled by default for zero overhead
5. ✅ **Backward compatible**: Existing `sources` continue to work

### User Experience Requirements

1. ✅ **Intuitive**: Clear symbols and legend
2. ✅ **Helpful error messages**: MergeContext integration for better errors
3. ✅ **Pipe-able output**: Valid YAML for automation
4. ✅ **Documentation**: Examples and use cases documented

## Technical Design

### Architecture Overview

```text
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
│ MergeContext (implements ProvenanceTracker)                │
│ - Stores file:line:column for each value path              │
│ - Uses JSONPath for addressing nested values               │
│ - Maintains inheritance chains                             │
│ - Thread-safe via ProvenanceStorage mutex                  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ Provenance Renderers                                        │
│ - InlineCommentRenderer: Valid YAML with comments          │
│ - TreeRenderer: Grouped by file (debugging)                │
└─────────────────────────────────────────────────────────────┘
```

### Data Structures

#### ProvenanceTracker Interface

```go
// pkg/merge/provenance_tracker.go

// ProvenanceTracker provides a generic interface for tracking configuration provenance.
type ProvenanceTracker interface {
    RecordProvenance(path string, entry ProvenanceEntry)
    GetProvenance(path string) []ProvenanceEntry
    HasProvenance(path string) bool
    GetProvenancePaths() []string
    IsProvenanceEnabled() bool
    EnableProvenance()
}
```

#### MergeContext (Implementation)

```go
// pkg/merge/merge_context.go

// MergeContext implements ProvenanceTracker for stack component provenance.
type MergeContext struct {
    CurrentFile   string
    ImportChain   []string
    ParentContext *MergeContext
    Provenance    *ProvenanceStorage  // Thread-safe storage
    Positions     u.PositionMap       // YAML line/column positions
}
```

#### ProvenanceEntry

```go
// pkg/merge/provenance_entry.go

type ProvenanceEntry struct {
    File      string           // Source file path
    Line      int              // Line number (1-indexed)
    Column    int              // Column number (1-indexed)
    Type      ProvenanceType   // import, inline, override, computed
    Depth     int              // Import depth in chain
}

type ProvenanceType string

const (
    ProvenanceTypeImport    ProvenanceType = "import"    // ○ Inherited
    ProvenanceTypeInline    ProvenanceType = "inline"    // ● Defined
    ProvenanceTypeOverride  ProvenanceType = "override"  // ● Overridden
    ProvenanceTypeComputed  ProvenanceType = "computed"  // ∴ Templated
    ProvenanceTypeDefault   ProvenanceType = "default"   // ○ Default
)
```

#### ProvenanceStorage (Thread-Safe)

```go
// pkg/merge/provenance_storage.go

type ProvenanceStorage struct {
    entries map[string][]ProvenanceEntry  // JSONPath → chain
    mutex   sync.RWMutex                   // Thread-safe access
}
```

### JSONPath Addressing

Uses JSONPath format to address any value in the configuration:

- Simple key: `vars.name`
- Nested key: `vars.tags.environment`
- Array element: `vars.availability_zones[0]`
- Map in array: `settings.validation.schemas[0].path`

## Implementation Details

### Phase 1: Core Provenance Tracking (✅ Complete)

**Implemented**:
- ✅ `ProvenanceTracker` interface
- ✅ `MergeContext` implements interface
- ✅ `ProvenanceStorage` with thread-safe storage
- ✅ `ProvenanceEntry` with all required fields
- ✅ Unit tests with >90% coverage
- ✅ Performance benchmarks

**Deliverables**:
- `pkg/merge/provenance_tracker.go` - Interface definition
- `pkg/merge/provenance_tracker_test.go` - Interface tests with MockProvenanceTracker
- `pkg/merge/merge_context.go` - Implementation
- `pkg/merge/provenance_storage.go` - Thread-safe storage
- `pkg/merge/provenance_entry.go` - Data structures

### Phase 2: YAML Parser Integration (✅ Complete)

**Implemented**:
- ✅ Line/column extraction using `gopkg.in/yaml.v3` Node API
- ✅ Position mapping from parsed nodes to JSONPath
- ✅ Multi-document YAML file handling
- ✅ Integration tests with test fixtures

**Deliverables**:
- Modified YAML parsing in `pkg/utils/yaml_utils.go`
- Position tracking for all YAML constructs
- Integration tests

### Phase 3: Stack Processor Integration (✅ Complete)

**Implemented**:
- ✅ Deep merge functions record provenance
- ✅ Provenance recorded at every merge operation
- ✅ JSONPath generation for nested values
- ✅ Array merging provenance (append, replace, merge)
- ✅ Template-evaluated values marked as "computed"
- ✅ MergeContext passed through entire stack processing pipeline
- ✅ Per-goroutine MergeContext to avoid data races

**Deliverables**:
- Modified `internal/exec/stack_processor_utils.go`
- Provenance tracking in all merge paths
- End-to-end tests for stack processing
- **Fix**: Removed shared MergeContext across goroutines (data race)

### Phase 4: Provenance Renderers (✅ Inline, ❌ Others)

**Implemented**:
- ✅ Inline comment renderer (`RenderInlineProvenanceWithStackFile`)
- ✅ Legend display at top of output
- ✅ Tree renderer for debugging (`RenderTree`)

**YAML Format on TTY** (inline comments with visual enhancements):
- ✅ Symbol-based inheritance indicators (`●`, `○`, `∴`)
- ✅ Depth tracking with `[N]` notation
- ✅ Syntax highlighting with ANSI colors
- ✅ Long string wrapping to prevent horizontal scrolling
- ✅ Two-column side-by-side display (Configuration │ Provenance)

**YAML Format non-TTY** (pipeable output):
- ✅ Inline comments without color codes
- ✅ Single-column layout (preserves valid YAML)

**JSON Format** (all outputs):
- ✅ Provenance embedded as `__atmos_provenance` metadata fields
- ✅ Structure: `{"file": "path", "line": N, "column": N, "type": "import|inline|override|computed", "depth": N}`
- ❌ No inline comments (JSON doesn't support comments)
- ❌ No visual indicators (JSON is structured data)

**Not Implemented** (Deferred):
- ❌ Embedded provenance in standard JSON output (pollutes data structures, only available via `__atmos_provenance` fields)

**Deliverables**:
- `pkg/provenance/inline.go` - Basic inline rendering
- `pkg/provenance/tree_renderer.go` - Main renderer with `RenderInlineProvenanceWithStackFile`
- Visual regression tests
- Snapshot testing

### Phase 5: CLI Integration (✅ Complete)

**Implemented**:
- ✅ `atmos describe component --provenance`
- ✅ `atmos describe stacks --provenance`
- ✅ Filters `sources` and `component_info` when provenance enabled
- ✅ Works with `--file` flag to save provenance output
- ✅ Works with `--query` for filtering

**Note on describe stacks**: Provenance is tracked during stack processing when `--provenance` is enabled, but the aggregated output doesn't render provenance inline (since it shows multiple components). To see provenance for specific components, use `atmos describe component <component> -s <stack> --provenance`.

**Deliverables**:
- Modified `cmd/describe_component.go`
- Modified `cmd/describe_stacks.go`
- CLI integration tests

### Phase 6: Extensibility (✅ Interface, ⚠️ Implementations)

**Completed**:
- ✅ `ProvenanceTracker` interface defined
- ✅ `MergeContext` implements interface
- ✅ Unit tests for interface compliance
- ✅ Mock implementation for testing

**Pending** (Future work):
- ⚠️ `atmos.yaml` provenance implementation
- ⚠️ `vendor.yaml` provenance implementation
- ⚠️ Workflow provenance implementation

**How to Extend**:
1. Create new context type implementing `ProvenanceTracker`
2. Record provenance during config loading/merging
3. Use existing renderers or create new ones specific to config type

**Effort Estimate**: 3-5 days per config type (interface makes this straightforward)

## CLI Usage

### describe component with provenance

```bash
$ atmos describe component vpc -s prod-ue2 --provenance

# Provenance Legend:
#   ● [1] Defined in parent stack
#   ○ [N] Inherited/imported (N=2+ levels deep)
#   ∴ Computed/templated

import:                                           # ○ [2] orgs/acme/_defaults.yaml:2
  - catalog/vpc/defaults                          # ○ [2] orgs/acme/_defaults.yaml:2
  - mixins/region/us-east-2                       # ● [1] orgs/acme/prod/us-east-2.yaml:3
  - orgs/acme/_defaults                           # ○ [2] orgs/acme/prod/_defaults.yaml:2
vars:                                             # ○ [3] catalog/vpc/defaults.yaml:8
  cidr: "10.100.0.0/16"                           # ● [1] orgs/acme/prod/us-east-2.yaml:10
  name: vpc                                       # ○ [3] catalog/vpc/defaults.yaml:9
  region: us-east-2                               # ○ [2] mixins/region/us-east-2.yaml:2
  namespace: acme                                 # ○ [3] orgs/acme/_defaults.yaml:2
```

### describe component with file output

```bash
$ atmos describe component vpc -s prod-ue2 --provenance --file vpc-config.yaml
# Saves provenance output to file (valid YAML with comments)
```

### describe stacks with provenance

```bash
$ atmos describe stacks --provenance
# Enables provenance tracking during stack processing
# Note: Aggregated output doesn't show inline provenance
# Use describe component for per-component provenance display
```

## Use Cases

### 1. Debugging Configuration Values

**Scenario**: A user sees an unexpected value and wants to know where it came from.

```bash
$ atmos describe component vpc -s prod-ue2 --provenance | grep cidr

  cidr: "10.100.0.0/16"  # ● [1] orgs/acme/prod/us-east-2.yaml:10
```

**Result**: User immediately sees the value is defined in the parent stack at line 10.

### 2. Understanding Inheritance

**Scenario**: A developer wants to understand how values are inherited and merged.

```bash
$ atmos describe component app -s staging-uw2 --provenance | grep replicas

  replicas: 3  # ○ [2] catalog/app/defaults.yaml:15
               # ● [1] stacks/staging/app.yaml:22  (override)
```

**Result**: Developer sees the value was inherited from defaults but overridden in staging.

### 3. Auditing Security Settings

**Scenario**: Compliance team needs to verify origins of security-related settings.

```bash
$ atmos describe component bastion -s prod-ue2 --provenance --query .vars.security

security:                                         # ○ [2] catalog/bastion/defaults.yaml:20
  allowed_cidr_blocks:                            # ● [1] stacks/prod/security.yaml:8
    - "10.0.0.0/8"                                # ● [1] stacks/prod/security.yaml:9
    - "172.16.0.0/12"                             # ● [1] stacks/prod/security.yaml:10
```

**Result**: Team can verify all security settings come from approved files.

### 4. Piping Provenance to Tools

**Scenario**: Automation tool needs to parse configuration with provenance.

```bash
$ atmos describe component vpc -s prod-ue2 --provenance --format yaml | yq '.vars.cidr'
10.100.0.0/16
```

**Result**: Valid YAML output works with standard tools (yq, jq with yq converter).

## Backward Compatibility

### sources System

The existing `sources` system continues to work and will be maintained indefinitely due to customer reliance. However, we encourage users to migrate to `--provenance` for:

- Line-level granularity (not just top-level keys)
- Full inheritance chains (not just final file)
- Better debugging experience (symbols and depth indicators)
- Valid YAML output (pipe-able and parse-able)

A deprecation timeline will be considered in future major versions (v2.0+) based on customer feedback and adoption.

### Migration Path

Users currently relying on `sources`:

1. **CLI users**: Add `--provenance` flag to `describe component` commands
2. **Automation**: Update scripts to parse inline YAML comments or use `--query` to extract specific values
3. **Documentation**: Update internal docs to show provenance examples

## Testing Strategy

### Unit Tests (✅ Complete)

- ✅ ProvenanceTracker interface contract (>95% coverage)
- ✅ MergeContext provenance methods
- ✅ Mock implementation verification
- ✅ YAML parser position tracking
- ✅ JSONPath generation
- ✅ Renderers (inline, tree)

### Integration Tests (✅ Complete)

- ✅ Stack processing with provenance
- ✅ Multiple inheritance scenarios
- ✅ Complex nested structures
- ✅ Array and map merging
- ✅ CLI integration tests

### Performance Tests (✅ Complete)

- ✅ Benchmark overhead with provenance enabled
- ✅ Memory usage with large projects
- ✅ Comparison with/without provenance

**Results**: <10% overhead, <50MB additional memory

### Regression Tests (✅ Complete)

- ✅ Existing functionality unchanged
- ✅ All `describe` command variations work
- ✅ Backward compatibility with `sources`

## Success Criteria

- ✅ Every line in stack component output has provenance available
- ✅ Provenance works for `describe component` and `describe stacks` commands
- ✅ ProvenanceTracker interface enables future extensibility
- ✅ Performance overhead <10% when enabled
- ✅ Inline rendering is clean, readable, produces valid YAML
- ✅ >90% test coverage
- ✅ Documentation complete with examples
- ✅ Interface ready for atmos.yaml/vendor/workflow extension (future)

## Future Enhancements

### Short-term (Interface Ready)

1. **atmos.yaml Provenance**: Track where atmos.yaml settings come from during imports
2. **vendor.yaml Provenance**: Show origins of vendored components
3. **Workflow Provenance**: Track workflow step origins

**Effort**: 3-5 days per config type (interface simplifies implementation)

### Long-term (Exploration)

1. **IDE Integration**: Language server protocol support for provenance hover
2. **Web UI**: Visual provenance browser for complex configs
3. **Diff Mode**: Show provenance changes between versions
4. **Search by File**: Find all values from a specific file
5. **Blame Integration**: Link to git blame for source files

## References

- Current `sources` implementation: `pkg/schema/schema.go` lines 686-701
- MergeContext: `pkg/merge/merge_context.go`
- ProvenanceTracker: `pkg/merge/provenance_tracker.go`
- YAML parser: Uses `gopkg.in/yaml.v3` with Node API
- Similar systems:
  - Terraform: Resource traceback with line numbers
  - Kubernetes: Field path tracking in validation errors
  - Ansible: Task provenance in playbooks

## Risks and Mitigations

| Risk | Impact | Mitigation | Status |
|------|--------|-----------|--------|
| Performance overhead too high | High | Benchmarked early, made optional | ✅ Resolved (<10% overhead) |
| Complex YAML structures don't align | Medium | Inline comments instead of columns | ✅ Resolved (valid YAML) |
| Line numbers become inaccurate | Medium | Track transformations, mark computed | ✅ Resolved |
| Backward compatibility issues | Medium | Keep `sources`, thorough testing | ✅ Resolved |
| Data races in parallel processing | High | Per-goroutine MergeContext | ✅ Resolved |
