# Vendor Package Refactoring Plan

## Current State
- **30 files** in `pkg/vendor/`
- **51.5% test coverage** (target: 80-90%)
- Files range from 1K to 25K bytes
- No internal import cycles (all files in flat structure)

## Goals
1. Split `pkg/vendor/` into focused sub-packages
2. Increase test coverage to 80%+
3. Eliminate circular dependency risk
4. Make code more maintainable and testable

---

## Phase 1: Create Sub-Package Structure (No Breaking Changes)

### Step 1.1: Create `pkg/vendor/uri` package
**Files to move:**
- `uri_helpers.go` → `pkg/vendor/uri/helpers.go`
- `uri_helpers_test.go` → `pkg/vendor/uri/helpers_test.go`

**Exports:** All URI helper functions (already 100% covered)

**Why first:** Leaf package with no internal dependencies, already well-tested.

### Step 1.2: Create `pkg/vendor/version` package
**Files to move:**
- `version_check.go` → `pkg/vendor/version/check.go`
- `version_check_test.go` → `pkg/vendor/version/check_test.go`
- `version_constraints.go` → `pkg/vendor/version/constraints.go`
- `version_constraints_test.go` → `pkg/vendor/version/constraints_test.go`

**Exports:** Version checking and constraint functions

**Why second:** Leaf package, mostly well-tested (100% on constraints).

### Step 1.3: Create `pkg/vendor/gitops` package
**Files to move:**
- `git_interface.go` → `pkg/vendor/gitops/interface.go`
- `git_diff.go` → `pkg/vendor/gitops/diff.go`
- `git_diff_test.go` → `pkg/vendor/gitops/diff_test.go`
- `mock_git_interface.go` → `pkg/vendor/gitops/mock.go`

**Exports:** GitOperations interface, diff helpers

**Why:** Clean separation of git operations, enables better mocking.

### Step 1.4: Create `pkg/vendor/source` package
**Files to move:**
- `source_provider.go` → `pkg/vendor/source/provider.go`
- `source_provider_git.go` → `pkg/vendor/source/git.go`
- `source_provider_github.go` → `pkg/vendor/source/github.go`
- `source_provider_unsupported.go` → `pkg/vendor/source/unsupported.go`
- `source_provider_test.go` → `pkg/vendor/source/provider_test.go`

**Depends on:** `pkg/vendor/gitops`, `pkg/vendor/version`

**Exports:** VendorSourceProvider interface, GetProviderForSource

### Step 1.5: Create `pkg/vendor/yaml` package
**Files to move:**
- `yaml_updater.go` → `pkg/vendor/yaml/updater.go`
- `yaml_updater_test.go` → `pkg/vendor/yaml/updater_test.go`

**Exports:** YAML version update functions

---

## Phase 2: Refactor Core Files (Careful Dependencies)

### Step 2.1: Keep in `pkg/vendor/` (main package)
These files stay as the public API and orchestration layer:
- `pull.go` - Public Pull() function
- `diff.go` - Public Diff() function
- `update.go` - Public Update() function
- `params.go` - Public param structs
- `utils.go` - Internal utilities (may split further)
- `component_utils.go` - Component vendor logic
- `model.go` - TUI model (may move to `pkg/vendor/tui/`)

### Step 2.2: Create backward-compatible re-exports
In `pkg/vendor/vendor.go`, re-export moved functions to avoid breaking external consumers:
```go
// Re-exports for backward compatibility
var (
    GetProviderForSource = source.GetProviderForSource
    // etc.
)
```

---

## Phase 3: Increase Test Coverage

### Priority 1: Zero-coverage functions (immediate impact)
| Function | File | Action |
|----------|------|--------|
| `Update` | update.go | Add unit tests with mocked config |
| `Diff` | diff.go | Add unit tests with mocked git ops |
| `executeVendorUpdate` | update.go | Add unit tests |
| `executeComponentVendorUpdate` | update.go | Add unit tests |
| `handleComponentVendor` | pull.go | Add unit tests |
| `ExecuteStackVendorInternal` | component_utils.go | Add stub test (returns ErrNotSupported) |

### Priority 2: Low-coverage functions (38-70%)
| Function | Coverage | Action |
|----------|----------|--------|
| `handleVendorConfig` | 38.5% | Add edge case tests |
| `validateVendorFlags` | 57.1% | Add all flag combination tests |
| `getConfigFiles` | 21.1% | Add directory/permission tests |
| `processVendorImports` | 15.8% | Add import chain tests |

### Priority 3: TUI model testing
- Mock `downloadAndInstall` for unit tests
- Test state transitions in `Update` method
- Test `View` rendering

---

## Phase 4: File Moves Summary

```
pkg/vendor/
├── uri/
│   ├── helpers.go          (from uri_helpers.go)
│   └── helpers_test.go     (from uri_helpers_test.go)
├── version/
│   ├── check.go            (from version_check.go)
│   ├── check_test.go       (from version_check_test.go)
│   ├── constraints.go      (from version_constraints.go)
│   └── constraints_test.go (from version_constraints_test.go)
├── gitops/
│   ├── interface.go        (from git_interface.go)
│   ├── diff.go             (from git_diff.go)
│   ├── diff_test.go        (from git_diff_test.go)
│   └── mock.go             (from mock_git_interface.go)
├── source/
│   ├── provider.go         (from source_provider.go)
│   ├── git.go              (from source_provider_git.go)
│   ├── github.go           (from source_provider_github.go)
│   ├── unsupported.go      (from source_provider_unsupported.go)
│   └── provider_test.go    (from source_provider_test.go)
├── yaml/
│   ├── updater.go          (from yaml_updater.go)
│   └── updater_test.go     (from yaml_updater_test.go)
├── pull.go                 (stays - public API)
├── diff.go                 (stays - public API)
├── update.go               (stays - public API)
├── params.go               (stays - public param types)
├── utils.go                (stays - internal utilities)
├── component_utils.go      (stays - component logic)
├── model.go                (stays - TUI)
└── *_test.go               (integration tests stay)
```

---

## Dependency Graph (No Cycles)

```
pkg/vendor (main)
    ├── pkg/vendor/uri         (leaf - no deps)
    ├── pkg/vendor/version     (leaf - no deps)
    ├── pkg/vendor/gitops      (leaf - no deps)
    ├── pkg/vendor/yaml        (leaf - no deps)
    └── pkg/vendor/source
            ├── pkg/vendor/gitops
            └── pkg/vendor/version
```

---

## Implementation Order

1. **Phase 1.1**: Move `uri/` - verify tests pass
2. **Phase 1.2**: Move `version/` - verify tests pass
3. **Phase 1.3**: Move `gitops/` - verify tests pass
4. **Phase 1.4**: Move `source/` - update imports, verify tests
5. **Phase 1.5**: Move `yaml/` - verify tests pass
6. **Phase 2**: Add re-exports, update main package imports
7. **Phase 3**: Add tests for zero-coverage functions
8. **Build & Test**: `go build ./... && go test ./pkg/vendor/...`

---

## Success Criteria

- [ ] All tests pass after each phase
- [ ] No import cycles (`go build ./...` succeeds)
- [ ] Coverage increases to 70%+ after Phase 3.1
- [ ] Coverage reaches 80%+ after Phase 3.2
- [ ] Public API unchanged (Pull, Diff, Update functions)
