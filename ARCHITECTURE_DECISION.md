# Architecture Decision: Path Construction vs Validation

## Context
When fixing the path duplication bug, we discovered that `JoinAbsolutePathWithPath` was doing both path construction AND filesystem validation (os.Stat checks). This caused issues in unit tests and violated the Single Responsibility Principle.

## Decision
We separated path construction from path validation:

### 1. Path Construction (Pure Functions)
```go
// JoinPath - Pure function for path manipulation
func JoinPath(basePath, providedPath string) string {
    if filepath.IsAbs(providedPath) {
        return providedPath
    }
    return filepath.Join(basePath, providedPath)
}
```

### 2. Path Validation (I/O Functions)
```go
// IsDirectory - I/O function that checks filesystem
func IsDirectory(path string) (bool, error) {
    fileInfo, err := os.Stat(path)
    // ...
}
```

### 3. Usage Pattern
```go
// 1. Construct the path (pure, testable)
componentPath := GetComponentPath(config, "terraform", prefix, component)

// 2. Validate when needed (I/O, mockable)
if exists, err := IsDirectory(componentPath); !exists {
    return fmt.Errorf("component not found: %s", componentPath)
}
```

## Benefits

1. **Testability**: Pure functions can be tested without filesystem mocks
2. **Separation of Concerns**: Path logic separate from I/O operations
3. **Flexibility**: Callers decide when validation is needed
4. **Performance**: Avoid unnecessary stat calls during path construction
5. **Mockability**: I/O operations are isolated and can be mocked via interfaces

## Alternative Considered: Filesystem Interface

We could have created a filesystem interface for mockability:

```go
type FileSystem interface {
    Stat(path string) (os.FileInfo, error)
    ReadFile(path string) ([]byte, error)
    // etc...
}
```

However, this was deemed unnecessary because:
- Current separation is sufficient
- Validation happens at clear boundaries (command execution)
- Tests can use the pure path functions without mocks
- Integration tests handle the full flow with real filesystem

## Validation Points

Filesystem validation happens at these key points:
1. **Command Execution**: Before running terraform/helmfile/packer commands
2. **Component Discovery**: When listing available components
3. **Stack Processing**: When validating stack configuration files exist
4. **Vendor Operations**: When pulling remote components

Path construction remains pure and happens throughout the codebase without I/O.

## Testing Strategy

1. **Unit Tests**: Test path construction logic with various inputs (absolute, relative, Windows, Unix)
2. **Integration Tests**: Test full flow with real filesystem in test fixtures
3. **No Mocks Needed**: Pure functions don't need mocks; I/O is tested in integration tests

This approach follows Go best practices and maintains clean architecture boundaries.
