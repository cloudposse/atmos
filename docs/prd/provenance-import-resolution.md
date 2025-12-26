# Atmos Provenance System Investigation

## Executive Summary

Atmos provides a sophisticated provenance tracking system that records the file, line number, and import chain for every configuration value. The system uses a **three-layer architecture** to properly track where stacks are defined and how they're imported.

## Key Architecture Components

### 1. MergeContext - The Import Chain Tracker

**Location**: `/pkg/merge/merge_context.go`

The `MergeContext` struct tracks the complete import chain during configuration merging:

```go
type MergeContext struct {
    // CurrentFile is the file currently being processed
    CurrentFile string

    // ImportChain tracks the chain of imports leading to the current file
    // First element is the root file, last is the current file
    ImportChain []string

    // ParentContext for nested operations
    ParentContext *MergeContext

    // Provenance stores optional provenance information
    Provenance *ProvenanceStorage

    // Positions stores YAML position information (line:column)
    Positions u.PositionMap
}
```

**Key Methods**:
- `WithFile(filePath)` - Creates a new context for a file, appending to ImportChain
- `GetImportChainString()` - Returns formatted import chain (e.g., "file1 → file2 → file3")
- `GetDepth()` - Returns the depth of the import chain
- `HasFile(filePath)` - Detects circular imports

### 2. ProvenanceEntry - The Value Source Record

**Location**: `/pkg/merge/provenance_entry.go`

Each value in the configuration has provenance metadata:

```go
type ProvenanceEntry struct {
    File      string           // Source file path
    Line      int              // Line number (1-indexed)
    Column    int              // Column number (1-indexed)
    Type      ProvenanceType   // import, inline, override, computed
    ValueHash string           // Hash of value for change detection
    Depth     int              // Import depth: 0=parent, 1=direct import, 2+=nested
}

type ProvenanceType string
const (
    ProvenanceTypeImport    = "import"    // ○ Inherited
    ProvenanceTypeInline    = "inline"    // ● Defined
    ProvenanceTypeOverride  = "override"  // ● Overridden
    ProvenanceTypeComputed  = "computed"  // ∴ Templated
    ProvenanceTypeDefault   = "default"   // ○ Default
)
```

### 3. ProvenanceStorage - Thread-Safe Value Tracking

**Location**: `/pkg/merge/provenance_storage.go`

Stores provenance chains keyed by JSONPath:

```go
type ProvenanceStorage struct {
    // entries maps JSONPath to a chain of provenance entries
    // e.g., "vars.cidr" -> [entry1, entry2, entry3]
    // Chain is ordered base → override
    entries map[string][]ProvenanceEntry

    // Thread-safe access
    mutex sync.RWMutex
}
```

**Usage Examples**:
```go
// Record provenance for a nested value
entry := ProvenanceEntry{
    File:      "stacks/prod/us-east-2.yaml",
    Line:      10,
    Column:    5,
    Type:      ProvenanceTypeInline,
    Depth:     0,  // Parent stack
}
storage.Record("vars.cidr", entry)

// Get the inheritance chain for a value
chain := storage.Get("vars.cidr")
// Returns all values this variable had through the inheritance chain

// Get only the final value
latest := storage.GetLatest("vars.cidr")
```

## How Provenance Tracks Import Chains

### The Flow (in ProcessYAMLConfigFileWithContext)

1. **Initialize MergeContext** (line 590-597):
```go
if mergeContext == nil {
    mergeContext = m.NewMergeContext()
    if atmosConfig != nil && atmosConfig.TrackProvenance {
        mergeContext.EnableProvenance()
    }
}
mergeContext = mergeContext.WithFile(relativeFilePath)
```
Each file call creates a **new context** with updated ImportChain.

2. **Extract YAML Positions** (line 657):
```go
stackConfigMap, positions, err := u.UnmarshalYAMLFromFileWithPositions[...](
    atmosConfig, stackManifestTemplatesProcessed, filePath)
```
The positions map tracks where each value appears in the YAML.

3. **Enable Provenance Storage** (line 676-679):
```go
if atmosConfig.TrackProvenance && mergeContext != nil && len(positions) > 0 {
    mergeContext.EnableProvenance()
    mergeContext.Positions = positions
}
```

4. **Recursive Merge with Provenance** (in MergeWithProvenance):
```go
// MergeWithProvenance calls standard Merge, then records provenance
recordProvenanceRecursive(provenanceRecursiveParams{
    data:        result,
    currentPath: "",
    ctx:         ctx,           // Has ImportChain + Provenance
    positions:   positions,     // YAML line:column info
    currentFile: ctx.CurrentFile,
    depth:       ctx.GetImportDepth(),
})
```

The `depth` is calculated from ImportChain length:
```go
func (c *MergeContext) GetImportDepth() int {
    depth := 0
    current := c
    for current != nil && current.ParentContext != nil {
        depth++
        current = current.ParentContext
    }
    return depth
}
```

## How Stacks Are Mapped to Files

### Two-Part System

**Part 1: Stack Name to File Mapping** (ProcessYAMLConfigFiles)

In `stack_processor_utils.go` (lines 314-328):
```go
for i, filePath := range filePaths {
    go func(i int, p string) {
        // Derive stack name from file path
        stackFileName := strings.TrimSuffix(
            strings.TrimSuffix(
                u.TrimBasePathFromPath(stackBasePath+"/", p),
                u.DefaultStackConfigFileExtension),
            ".yml",
        )

        // Example: "stacks/orgs/prod/us-east-2.yaml" → "orgs/prod/us-east-2"

        // ... process file ...

        // Store result by stack name
        results <- stackProcessResult{
            stackFileName: stackFileName,
            // ...
        }
    }(i, filePath)
}
```

**Part 2: MergeContext Storage** (lines 424-428):
```go
// Store merge context for this stack file if provenance tracking is enabled
if atmosConfig != nil && atmosConfig.TrackProvenance &&
   result.mergeContext != nil && result.mergeContext.IsProvenanceEnabled() {

    // Key: stack name (derived from file path)
    SetMergeContextForStack(result.stackFileName, result.mergeContext)
    SetLastMergeContext(result.mergeContext)  // For backward compat
}
```

### Retrieving Stack File Information

**For a specific component in a stack** (describe_component.go):
```go
// Get the stack file (from ProcessComponentConfig)
stackFile = result.StackFile  // e.g., "prod/us-east-2"

// Get the merge context with import chain
mergeContext = GetMergeContextForStack(configAndStacksInfo.StackFile)

// Now you can:
// 1. Get the import chain
importChain := mergeContext.ImportChain
// Returns: ["prod/us-east-2.yaml", "catalog/vpc/defaults.yaml", "orgs/acme/_defaults.yaml"]

// 2. Get provenance for a specific value
provenance := mergeContext.GetProvenance("vars.cidr")
// Returns: [
//   {File: "catalog/vpc/defaults.yaml", Line: 8, Depth: 2},
//   {File: "prod/us-east-2.yaml", Line: 10, Depth: 0, Type: override},
// ]

// 3. Get the final value's source
latest := mergeContext.GetProvenance("vars.cidr")[len(...)-1]
// Returns: {File: "prod/us-east-2.yaml", Line: 10, Depth: 0}
```

## Correct Way to Resolve Stack Files

### ✅ DO: Use ExecuteDescribeStacks + MergeContext

This is the **authoritative** method used by describe component:

```go
// 1. Get the stacks map (maps stack name → config)
stacksMap, _, err := FindStacksMap(atmosConfig, false)

// 2. Process a specific component
err = ProcessComponentConfig(
    &configAndStacksInfo,
    stackName,
    stacksMap,
    componentType,
    component,
    authManager,
)

// 3. Get the stack file (this is accurate)
stackFile := configAndStacksInfo.StackFile

// 4. Get the merge context with import chain
mergeContext := GetMergeContextForStack(stackFile)

// 5. Use import chain from mergeContext
for i, file := range mergeContext.ImportChain {
    depth := i  // 0 = parent stack, 1+ = imports
    fmt.Printf("Level %d: %s\n", depth, file)
}
```

### ❌ DON'T: Use Heuristic Path Guessing

The old `import_resolver.go` uses unreliable heuristics:

```go
// BAD: Tries to guess the file path from stack name
possiblePaths := []string{
    filepath.Join(stacksBasePath, "orgs", stackName+".yaml"),  // Assumes pattern!
    filepath.Join(stacksBasePath, stackName+".yaml"),
    filepath.Join(stacksBasePath, strings.ReplaceAll(stackName, "-", "/"), ".yaml"),
}

for _, path := range possiblePaths {
    if u.FileExists(path) {
        return path, nil
    }
}
```

**Problems**:
- Assumes stack names follow a specific pattern
- Fails for complex directory structures
- Doesn't work with symlinks or aliased imports
- Can return wrong file if multiple matches exist

## Data Flow for Tree View Implementation

### Recommended Architecture

```
┌─────────────────────────────────────────────────┐
│ ExecuteDescribeStacks() or describe component    │
└─────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────┐
│ FindStacksMap()                                 │
│ Returns: stacksMap[stackName] = config          │
└─────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────┐
│ ProcessComponentConfig()                        │
│ Sets configAndStacksInfo.StackFile              │
└─────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────┐
│ GetMergeContextForStack(stackFile)              │
│ Returns: MergeContext with ImportChain          │
└─────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────┐
│ mergeContext.ImportChain                        │
│ [0] = parent stack file path                    │
│ [1..N] = imported files in order                │
└─────────────────────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────┐
│ Build Tree View                                 │
│ - Root: parent stack file                       │
│ - Children: imported files at each level        │
│ - Depth indicators show inheritance chain       │
└─────────────────────────────────────────────────┘
```

## Code Examples for Tree View Implementation

### Example 1: Get Accurate Import Chain for Stack

```go
package exec

import (
    m "github.com/cloudposse/atmos/pkg/merge"
)

// GetStackImportChain returns the import chain for a specific stack
// This is the CORRECT way to get the actual files
func GetStackImportChain(stackFileName string) []string {
    mergeContext := GetMergeContextForStack(stackFileName)
    if mergeContext == nil {
        return []string{}
    }

    // ImportChain contains the actual file paths
    // Index 0 = parent stack
    // Index 1..N = imported files
    return mergeContext.ImportChain
}

// GetStackFileDepth returns how many levels of imports a stack has
func GetStackFileDepth(stackFileName string) int {
    mergeContext := GetMergeContextForStack(stackFileName)
    if mergeContext == nil {
        return 0
    }

    // Depth = number of imports
    return len(mergeContext.ImportChain) - 1
}
```

### Example 2: Get Provenance for a Specific Value

```go
// GetValueProvenance returns where a value was defined
func GetValueProvenance(stackFileName string, jsonPath string) *m.ProvenanceEntry {
    mergeContext := GetMergeContextForStack(stackFileName)
    if mergeContext == nil {
        return nil
    }

    chain := mergeContext.GetProvenance(jsonPath)
    if len(chain) == 0 {
        return nil
    }

    // Last entry is the final value
    latest := chain[len(chain)-1]
    return &latest
}

// GetValueInheritanceChain returns all overrides for a value
func GetValueInheritanceChain(stackFileName string, jsonPath string) []*m.ProvenanceEntry {
    mergeContext := GetMergeContextForStack(stackFileName)
    if mergeContext == nil {
        return nil
    }

    chain := mergeContext.GetProvenance(jsonPath)
    result := make([]*m.ProvenanceEntry, len(chain))
    for i := range chain {
        result[i] = &chain[i]
    }
    return result
}
```

### Example 3: Build a Tree of Stacks and Their Imports

```go
// StackNode represents a node in the stack import tree
type StackNode struct {
    Name            string
    StackFile       string
    ImportedFrom    []string
    Depth           int
}

// BuildStackImportTree builds a tree showing import relationships
func BuildStackImportTree(atmosConfig *schema.AtmosConfiguration) (map[string]*StackNode, error) {
    stacksMap, _, err := FindStacksMap(atmosConfig, false)
    if err != nil {
        return nil, err
    }

    nodes := make(map[string]*StackNode)

    for stackName := range stacksMap {
        mergeContext := GetMergeContextForStack(stackName)
        if mergeContext == nil {
            continue
        }

        node := &StackNode{
            Name:      stackName,
            StackFile: stackName,
            Depth:     len(mergeContext.ImportChain) - 1,
        }

        // ImportChain[0] is the parent, ImportChain[1:] are imports
        if len(mergeContext.ImportChain) > 1 {
            node.ImportedFrom = mergeContext.ImportChain[1:]
        }

        nodes[stackName] = node
    }

    return nodes, nil
}
```

## Key Insights for Tree View Implementation

### 1. Stack File Resolution is Automatic
- Don't use heuristics or pattern matching
- Use `ProcessComponentConfig()` which sets `StackFile` correctly
- Or retrieve via `GetMergeContextForStack(stackName)`

### 2. Import Chain is Complete and Ordered
- `ImportChain[0]` = parent stack file (the one being described)
- `ImportChain[1..N]` = imported files in merge order
- This is the **complete and accurate** import path

### 3. Depth Tracking is Built-In
- `ImportChain.length - 1` = depth of imports
- Used to show indentation and visual hierarchy
- Depth 0 = parent stack, Depth 1 = direct import, Depth 2+ = nested imports

### 4. Line Numbers Are Available
- Use `mergeContext.GetProvenance(jsonPath)` for value-level provenance
- Each entry has `File`, `Line`, `Column`, and `Depth`
- This enables accurate "go to line" functionality

### 5. Thread Safety
- `MergeContext` uses `sync.RWMutex` in `ProvenanceStorage`
- `GetMergeContextForStack()` uses locks for safe access
- Safe to call from multiple goroutines

## Deprecation Note

The old system with `import_resolver.go` (using heuristic path guessing) is **outdated and inaccurate**. Always use the merge context system for reliable stack-to-file mapping.
