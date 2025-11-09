# Component Resolution Architecture Refactor

## Problem
Original circular dependency:
```
pkg/component (needs FindStacksMap)
  └─> internal/exec (has FindStacksMap)
        └─> pkg/component (for IsExplicitComponentPath)
           └─> CIRCULAR DEPENDENCY!
```

## Solution: Dependency Injection

### New Architecture
```
pkg/component/
├── path.go              # IsExplicitComponentPath() - path detection helper
└── resolver.go          # Resolver with StackLoader interface for DI

internal/exec/
├── stack_loader.go      # ExecStackLoader implements StackLoader interface
├── component_resolver.go # Thin wrappers around pkg/component.Resolver
└── utils.go            # FindStacksMap() stays here
```

### Dependency Flow (NO CYCLES!)
```
pkg/component
  ├─> pkg/utils (ExtractComponentInfoFromPath)
  ├─> pkg/schema
  ├─> pkg/logger
  └─> pkg/perf

internal/exec
  ├─> pkg/component (imports Resolver, StackLoader interface)
  └─> (provides StackLoader implementation)
```

## Key Design Patterns

### 1. Dependency Injection via Interface
```go
// pkg/component/resolver.go
type StackLoader interface {
    FindStacksMap(*schema.AtmosConfiguration, bool) (map[string]any, map[string]map[string]any, error)
}

type Resolver struct {
    stackLoader StackLoader  // Injected dependency
}
```

### 2. Implementation in internal/exec
```go
// internal/exec/stack_loader.go
type ExecStackLoader struct{}

func (l *ExecStackLoader) FindStacksMap(...) (...) {
    return FindStacksMap(...)  // Calls existing function
}
```

### 3. Backwards-Compatible Wrappers
```go
// internal/exec/component_resolver.go
var componentResolver = comp.NewResolver(NewStackLoader())

func ResolveComponentFromPath(...) (...) {
    return componentResolver.ResolveComponentFromPath(...)
}
```

## Benefits

1. **No Circular Dependencies**: `pkg/component` doesn't import `internal/exec`
2. **Testable**: Can mock `StackLoader` interface for unit tests
3. **Reusable**: `pkg/component` can be used by other packages
4. **Backwards Compatible**: Existing callers don't need changes
5. **Clean Architecture**: Business logic in `pkg`, orchestration in `internal`

## Files Created/Modified

### Created
- `pkg/component/resolver.go` - Main component resolution logic with DI
- `internal/exec/stack_loader.go` - StackLoader implementation

### Modified
- `internal/exec/component_resolver.go` - Now thin wrappers around pkg/component

### Unchanged
- `pkg/component/path.go` - Path detection helper (already existed)
- `internal/exec/utils.go` - FindStacksMap stays here (too many dependencies)

## Migration Path

No migration needed! All existing code continues to work:
```go
// Existing code (still works)
component, err := exec.ResolveComponentFromPath(cfg, path, stack, "terraform")

// New code (optional, for testing)
resolver := component.NewResolver(customStackLoader)
component, err := resolver.ResolveComponentFromPath(cfg, path, stack, "terraform")
```

## Testing Strategy

Unit tests can now mock the stack loader:
```go
type mockStackLoader struct{}
func (m *mockStackLoader) FindStacksMap(...) (...) {
    return mockData, nil
}

resolver := component.NewResolver(&mockStackLoader{})
// Test without needing full stack processing!
```
