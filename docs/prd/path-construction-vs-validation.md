# PRD: Path Construction vs Validation Architecture

## Document Control
- **PRD Number**: PRD-2025-001
- **Author**: Engineering Team
- **Date Created**: 2025-09-27
- **Last Updated**: 2025-09-27
- **Status**: Implemented
- **Related Issues**: #1512, #1535

## Executive Summary

This PRD documents the architectural decision to separate path construction from filesystem validation in Atmos. This separation improves testability, maintainability, and follows the Single Responsibility Principle while fixing a critical path duplication bug that affected GitHub Actions workflows.

## Problem Statement

### Background
When component paths were configured with absolute paths in Atmos, the `filepath.Join()` function on Unix systems incorrectly handled two absolute paths, causing path duplication. This manifested as broken GitHub Actions pipelines with duplicated paths like:
```text
/home/runner/_work/infrastructure/infrastructure/home/runner/_work/infrastructure/infrastructure/atmos/components/terraform
```

### Root Cause Analysis
The issue stemmed from mixing path construction logic with filesystem validation in the `JoinAbsolutePathWithPath` function, which:
1. Performed path string manipulation
2. Executed filesystem checks (`os.Stat`)
3. Failed in unit tests when paths didn't exist
4. Violated the Single Responsibility Principle

### Impact
- **Users Affected**: All users upgrading from v1.191.0 to v1.192.0
- **Severity**: High - Broken CI/CD pipelines
- **Platforms**: Primarily Unix/Linux, with potential Windows issues

## Requirements

### Functional Requirements

#### FR1: Path Construction
- **FR1.1**: Pure functions that manipulate path strings without I/O operations
- **FR1.2**: Handle absolute paths correctly on all platforms (Windows, Unix, macOS)
- **FR1.3**: Prevent path duplication when joining two absolute paths
- **FR1.4**: Support all path formats:
  - Unix: `/home/user/project`
  - Windows: `C:\Users\project`
  - UNC: `\\server\share\project`
  - Long paths: `\\?\C:\very\long\path`

#### FR2: Path Validation
- **FR2.1**: Separate functions for filesystem checks
- **FR2.2**: Validation only when explicitly needed
- **FR2.3**: Clear error messages when paths don't exist

### Non-Functional Requirements

#### NFR1: Testability
- **NFR1.1**: Path construction functions must be testable without filesystem mocks
- **NFR1.2**: >80% test coverage for path operations
- **NFR1.3**: Tests must run on all platforms without modification

#### NFR2: Performance
- **NFR2.1**: No unnecessary filesystem operations during path construction
- **NFR2.2**: Lazy validation - only check existence when required

#### NFR3: Maintainability
- **NFR3.1**: Clear separation of concerns
- **NFR3.2**: Self-documenting function names
- **NFR3.3**: Consistent patterns across the codebase

## Design

### Architecture Overview

```text
┌─────────────────────────────────────────┐
│           Application Layer              │
│  (Commands: terraform, helmfile, etc.)   │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│         Path Construction Layer          │
│      (Pure Functions - No I/O)           │
│  ┌─────────────────────────────────┐    │
│  │ JoinPath(base, provided)        │    │
│  │ buildComponentPath(...)         │    │
│  │ GetComponentPath(...)           │    │
│  └─────────────────────────────────┘    │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│         Path Validation Layer            │
│        (I/O Operations - Mockable)       │
│  ┌─────────────────────────────────┐    │
│  │ IsDirectory(path)               │    │
│  │ FileExists(path)                │    │
│  │ FileOrDirExists(path)          │    │
│  └─────────────────────────────────┘    │
└──────────────────────────────────────────┘
```

### Core Components

#### 1. Path Construction Functions (Pure)

```go
// JoinPath - Pure function for path manipulation
// No filesystem checks, no side effects
func JoinPath(basePath, providedPath string) string {
    if filepath.IsAbs(providedPath) {
        return providedPath
    }
    return filepath.Join(basePath, providedPath)
}
```

**Characteristics:**
- Deterministic output
- No side effects
- Platform-agnostic using `filepath` package
- Easily testable

#### 2. Path Validation Functions (I/O)

```go
// IsDirectory - I/O function that checks filesystem
func IsDirectory(path string) (bool, error) {
    fileInfo, err := os.Stat(path)
    if err != nil {
        return false, err
    }
    return fileInfo.IsDir(), nil
}
```

**Characteristics:**
- Performs actual filesystem operations
- Returns errors for missing paths
- Used at application boundaries
- Can be mocked if needed

### Usage Pattern

```go
// Step 1: Construct the path (pure, no I/O)
componentPath := GetComponentPath(config, "terraform", prefix, component)

// Step 2: Validate when needed (I/O operation)
if exists, err := IsDirectory(componentPath); !exists {
    return fmt.Errorf("component not found: %s", componentPath)
}

// Step 3: Use the validated path
return ExecuteTerraformCommand(componentPath, args)
```

## Implementation

### Phase 1: Core Functions (Completed)
- [x] Implement `JoinPath` utility function
- [x] Refactor `buildComponentPath` to use `JoinPath`
- [x] Update `atmosConfigAbsolutePaths` to use `JoinPath`

### Phase 2: Testing (Completed)
- [x] Unit tests for path construction (no mocks needed)
- [x] Cross-platform path handling tests (65+ scenarios)
- [x] Windows and Unix edge case tests (comprehensive coverage)
- [x] Integration tests with real filesystem

### Phase 3: Migration (Completed)
- [x] Update all path joining to use new utilities
- [x] Remove duplicate path logic
- [x] Ensure backward compatibility

## Testing Strategy

### Unit Tests
- Test path construction with various inputs
- No filesystem mocks required
- Cross-platform test cases

### Integration Tests
- Test full command flow with real filesystem
- Use test fixtures in `tests/` directory
- Validate actual file operations

### Test Coverage Matrix

| Scenario | Windows | Unix | macOS |
|----------|---------|------|-------|
| Absolute paths | ✅ | ✅ | ✅ |
| Relative paths | ✅ | ✅ | ✅ |
| UNC paths | ✅ | N/A | N/A |
| Drive letters | ✅ | N/A | N/A |
| Hidden files | ✅ | ✅ | ✅ |
| Special chars | ✅ | ✅ | ✅ |
| Path navigation | ✅ | ✅ | ✅ |

## Alternatives Considered

### Alternative 1: Filesystem Interface with Dependency Injection

```go
type FileSystem interface {
    Stat(path string) (os.FileInfo, error)
    ReadFile(path string) ([]byte, error)
}
```

**Pros:**
- Full mockability
- Complete control in tests

**Cons:**
- Over-engineering for current needs
- Increases complexity
- Requires refactoring all file operations

**Decision:** Rejected - Current approach provides sufficient testability

### Alternative 2: Keep Mixed Responsibilities

**Pros:**
- No refactoring needed
- Single function call

**Cons:**
- Poor testability
- Violates SRP
- Continued test failures

**Decision:** Rejected - Causes test failures and maintenance issues

## Success Metrics

1. **Test Coverage**: >80% coverage for path operations ✅
2. **Bug Resolution**: No path duplication in any scenario ✅
3. **Cross-Platform**: Tests pass on Windows, Linux, macOS ✅
4. **Performance**: No unnecessary filesystem operations ✅
5. **Maintainability**: Clear separation of concerns ✅

## Security Considerations

1. **Path Traversal**: Functions don't prevent `../` navigation - this is intentional as some workflows require it
2. **Symbolic Links**: Not resolved during path construction - resolution happens at validation
3. **Permissions**: Validation functions return appropriate errors for permission issues

## Migration Guide

### For Atmos Core Team

1. Use `JoinPath` for all new path joining operations
2. Call validation functions only at command boundaries
3. Keep path construction and validation separate

### For Plugin Developers

No changes required - external API remains the same

## Rollout Plan

1. **v1.193.0**: Include fix with separated path logic
2. **Documentation**: Update contributor guidelines
3. **Monitoring**: Watch for path-related issues in GitHub Issues

## Appendix

### A. Function Inventory

#### Path Construction (Pure)
- `JoinPath(basePath, providedPath string) string`
- `buildComponentPath(basePath, folderPrefix, component string) string`
- `GetComponentPath(config, type, prefix, component) (string, error)`

#### Path Validation (I/O)
- `IsDirectory(path string) (bool, error)`
- `FileExists(path string) bool`
- `FileOrDirExists(path string) bool`

### B. References

- PR #1512: Introduction of regression
- PR #1535: Implementation of fix
- Go filepath package documentation
- Single Responsibility Principle

### C. Glossary

- **Pure Function**: Function with no side effects, deterministic output
- **I/O Operation**: Operation that interacts with filesystem
- **Path Duplication**: Bug where absolute paths were incorrectly concatenated
- **SRP**: Single Responsibility Principle
