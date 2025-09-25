# Product Requirements Document: Terraform Dependency Order Execution

## Executive Summary

This PRD outlines the implementation of dependency-ordered execution for Terraform operations in Atmos, specifically for the `--all` flag. The solution will generalize the existing dependency graph logic used by `--affected` to create a reusable dependency management system that ensures components are processed in the correct order based on their dependencies.

## Problem Statement

### Current State
- The `--affected` flag already implements dependency-ordered execution for affected components
- The `--all` flag processes all components but does not respect dependency order
- This can lead to deployment failures when dependent components are processed before their dependencies
- The dependency logic is tightly coupled with the affected components logic

### Business Impact
- Risk of deployment failures due to incorrect execution order
- Manual intervention required to run components in correct order
- Increased deployment time and operational overhead
- Potential for infrastructure inconsistencies

## Goals and Objectives

### Primary Goals
1. Implement dependency-ordered execution for `atmos terraform apply --all`
2. Create a reusable dependency graph system for both `--all` and `--affected`
3. Maintain backward compatibility with existing functionality
4. Improve code maintainability through proper separation of concerns

### Success Criteria
- `--all` flag respects component dependencies
- No regression in `--affected` functionality
- Successful detection and prevention of circular dependencies
- Clear error messages for dependency issues
- Performance remains acceptable for large infrastructures

## Solution Overview

### High-Level Approach
Create a generalized dependency graph package that can:
1. Build a complete dependency graph from Atmos stack configurations
2. Perform topological sorting to determine execution order
3. Detect circular dependencies
4. Filter graphs for specific use cases (affected components, queries, etc.)

### Architecture

```text
pkg/dependency/          # Reusable dependency graph logic
├── types.go            # Core types and interfaces
├── graph.go            # Graph structure and operations
├── builder.go          # Graph construction
├── sort.go             # Topological sort implementation
└── filter.go           # Graph filtering operations

internal/exec/           # Atmos-specific implementation
├── terraform_all.go    # --all flag implementation
└── terraform_executor.go # Shared execution logic
```

## Functional Requirements

### 1. Dependency Graph Construction

#### FR-1.1: Component Discovery
- System SHALL discover all Terraform components across all stacks
- System SHALL extract component metadata including dependencies
- System SHALL handle abstract and disabled components appropriately

#### FR-1.2: Dependency Mapping
- System SHALL parse `settings.depends_on` configuration from stack files
- System SHALL build bidirectional dependency relationships
- System SHALL support cross-stack dependencies

### 2. Dependency Resolution

#### FR-2.1: Topological Sorting
- System SHALL implement topological sort algorithm (Kahn's or DFS-based)
- System SHALL determine correct execution order
- System SHALL handle components with no dependencies (roots)

#### FR-2.2: Cycle Detection
- System SHALL detect circular dependencies
- System SHALL provide clear error messages with cycle path
- System SHALL fail fast when cycles are detected

### 3. Execution Control

#### FR-3.1: Ordered Execution
- System SHALL execute components in dependency order
- System SHALL respect dry-run mode
- System SHALL support all Terraform subcommands (apply, destroy, plan)

#### FR-3.2: Error Handling
- System SHALL handle missing dependencies gracefully
- System SHALL provide option to continue or fail on errors
- System SHALL log execution progress clearly

### 4. Filtering and Selection

#### FR-4.1: Component Filtering
- System SHALL support filtering by stack
- System SHALL support filtering by component
- System SHALL support YQ query expressions

#### FR-4.2: Affected Components
- System SHALL maintain existing `--affected` functionality
- System SHALL filter graph to affected components and dependencies

## Non-Functional Requirements

### NFR-1: Performance
- Dependency graph construction SHALL complete in < 5 seconds for 1000 components
- Topological sort SHALL complete in O(V + E) time complexity
- Memory usage SHALL scale linearly with number of components

### NFR-2: Maintainability
- Code SHALL follow Go best practices and idioms
- Package structure SHALL separate concerns appropriately
- Functions SHALL be small and focused (< 150 lines)

### NFR-3: Testability
- Core logic SHALL have > 80% test coverage
- Dependency logic SHALL be testable without external dependencies
- Integration tests SHALL cover complex scenarios

### NFR-4: Usability
- Error messages SHALL be clear and actionable
- Progress indicators SHALL show current component being processed
- Dry-run mode SHALL show planned execution order

## Technical Specifications

### Data Structures

```go
// Core node structure
type Node struct {
    ID           string   // Unique identifier (component-stack)
    Component    string   // Component name
    Stack        string   // Stack name
    Dependencies []string // Node IDs this depends on
    Dependents   []string // Node IDs that depend on this
    Metadata     map[string]any
}

// Dependency graph
type Graph struct {
    Nodes map[string]*Node
    Roots []string // Nodes with no dependencies
}
```

### Key Algorithms

#### Topological Sort (Kahn's Algorithm)
1. Calculate in-degree for all nodes
2. Queue all nodes with in-degree 0
3. Process queue:
   - Remove node from queue
   - Add to result
   - Reduce in-degree of dependents
   - Queue dependents with in-degree 0
4. Check if all nodes processed (cycle detection)

#### Cycle Detection (DFS)
1. Maintain visited set and recursion stack
2. For each unvisited node:
   - Mark as visited and in recursion stack
   - Visit all dependencies
   - If dependency in recursion stack, cycle found
   - Remove from recursion stack

### API Design

```go
// Build dependency graph
graph, err := dependency.BuildGraph(components)

// Get execution order
order, err := graph.TopologicalSort()

// Filter for affected components
filtered := graph.Filter(affectedIDs, includeDependencies)

// Detect cycles
hasCycle, cyclePath := graph.DetectCycles()
```

## Implementation Plan

### Phase 1: Core Dependency Package (Week 1)
1. Create `pkg/dependency` package structure
2. Implement graph data structures
3. Implement topological sort
4. Implement cycle detection
5. Write unit tests

### Phase 2: Terraform Integration (Week 1-2)
1. Create `ExecuteTerraformAll` function
2. Integrate dependency graph with stack processing
3. Update command routing for `--all` flag
4. Refactor `--affected` to use new graph

### Phase 3: Testing and Documentation (Week 2)
1. Create integration tests
2. Test with complex dependency scenarios
3. Update user documentation
4. Create example configurations

## Testing Strategy

### Unit Tests
- Graph construction and manipulation
- Topological sort correctness
- Cycle detection accuracy
- Filter operations

### Integration Tests
- End-to-end `--all` execution
- Cross-stack dependencies
- Error scenarios
- Performance with large graphs

### Test Scenarios
```yaml
# Simple chain: A -> B -> C
# Diamond: A -> B,C -> D
# Complex: Multiple roots, shared dependencies
# Circular: A -> B -> C -> A (should fail)
```

## Risk Assessment

### Technical Risks
- **Risk**: Performance degradation with large infrastructures
- **Mitigation**: Optimize algorithms, add caching, benchmark regularly

- **Risk**: Breaking existing `--affected` functionality
- **Mitigation**: Comprehensive testing, gradual refactoring

### Operational Risks
- **Risk**: Incorrect dependency configuration causes failures
- **Mitigation**: Clear documentation, validation tools, dry-run mode

## Success Metrics

1. **Correctness**: Zero dependency-related deployment failures
2. **Performance**: < 5 second overhead for dependency resolution
3. **Adoption**: 90% of users utilize `--all` with dependencies
4. **Quality**: Zero critical bugs in first month post-release
5. **Maintainability**: Reduced code complexity metrics

## Migration Strategy

### Backward Compatibility
- Existing commands continue to work unchanged
- `--affected` maintains current behavior
- New functionality is opt-in via flags

### Rollout Plan
1. Alpha: Internal testing with test environments
2. Beta: Limited release to power users
3. GA: Full release with documentation

## Documentation Requirements

### User Documentation
- Usage examples for `--all` with dependencies
- Dependency configuration guide
- Troubleshooting circular dependencies
- Migration guide from manual ordering

### Developer Documentation
- Architecture overview
- API reference
- Extension guide
- Contributing guidelines

## Open Questions

1. Should we support parallel execution of independent components?
2. How should we handle optional dependencies?
3. Should dependency order be configurable per command?
4. What level of progress reporting is needed?

## Appendix

### Example Configuration

```yaml
# stacks/prod.yaml
components:
  terraform:
    vpc:
      vars:
        cidr: "10.0.0.0/16"

    database:
      settings:
        depends_on:
          - component: vpc
      vars:
        instance_class: "db.t3.medium"

    application:
      settings:
        depends_on:
          - component: database
          - component: vpc
      vars:
        replicas: 3
```

### Expected Execution Order
1. vpc (no dependencies)
2. database (depends on vpc)
3. application (depends on vpc and database)

## Approval

- **Author**: AI Assistant
- **Date**: 2024-09-25
- **Status**: Draft
- **Reviewers**: TBD
