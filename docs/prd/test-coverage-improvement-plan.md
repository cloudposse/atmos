# Test Coverage Improvement Plan

## Executive Summary

**Current State**: Project coverage is 71.00%, with patch coverage at 36.39% (346 lines missing in recent changes).

**Goal**: Increase patch coverage from 36.39% to **80%+** (a 43.61 percentage point improvement) and overall project coverage to **80%+** (9 percentage point improvement).

**Strategy**: Focus on the 10 files in `cmd/list/` with lowest coverage, prioritizing high-value unit tests that cover edge cases, error paths, and integration points.

## Coverage Analysis

### Files Requiring Attention (from Codecov Report)

Based on the Codecov report, these `cmd/list/` files have critical coverage gaps:

| File | Current Coverage | Missing Lines | Priority |
|------|-----------------|---------------|----------|
| `cmd/list/stacks.go` | 17.14% | 28 + 1 partial | **HIGH** |
| `cmd/list/instances.go` | 20.00% | 23 + 1 partial | **HIGH** |
| `cmd/list/cmd_utils.go` | 25.00% | 16 + 2 partials | **HIGH** |
| `cmd/list/components.go` | 25.71% | 25 + 1 partial | **HIGH** |
| `cmd/list/values.go` | 26.37% | 63 + 4 partials | **CRITICAL** |
| `cmd/list/settings.go` | 28.57% | 54 + 1 partial | **CRITICAL** |
| `cmd/list/metadata.go` | 31.64% | 53 + 1 partial | **HIGH** |
| `cmd/list/themes.go` | 33.33% | 14 + 2 partials | **MEDIUM** |
| `cmd/list/utils.go` | 38.59% | 33 + 2 partials | **MEDIUM** |
| `cmd/list/vendor.go` | 74.35% | 6 + 4 partials | **LOW** |

### Current Test Coverage Assessment

**Existing tests focus on**:
- Flag validation (structure, defaults, shorthand)
- Argument validation (ExactArgs, MaximumNArgs, NoArgs)
- Command structure (Use, Short, Long, Examples)
- Simple option structure tests

**Missing coverage**:
- **Error handling paths** - Component not found, invalid config, describe stacks failures
- **Edge cases** - Empty results, nil values, malformed input
- **Helper functions** - setupXOptions, logNoXFoundMessage, getFilterOptionsFromValues
- **Integration points** - Viper flag binding, environment variable precedence
- **Output formatting** - CSV delimiter defaults, query transformations
- **Stack completion** - Shell completion logic, component filtering

## Implementation Plan

### Phase 1: Critical Coverage (Files with <30% coverage)

#### 1.1 `cmd/list/values.go` (26.37% → 80%+)

**Target**: Add 63 lines of test coverage

**Test scenarios to add**:
```go
// Helper function tests
func TestGetBoolFlagWithDefault(t *testing.T) {
    // Test cases:
    // - Flag exists and returns value
    // - Flag doesn't exist, returns default
    // - Flag parsing error, returns default with warning
}

func TestGetFilterOptionsFromValues(t *testing.T) {
    // Test cases:
    // - Vars flag true → query set to ".vars"
    // - Query provided → query preserved
    // - CSV format with TSV delimiter → delimiter changed to comma
    // - TSV format → delimiter preserved
    // - All option combinations
}

func TestLogNoValuesFoundMessage(t *testing.T) {
    // Test cases:
    // - Query is ".vars" → logs "No vars found"
    // - Other query → logs "No values found"
    // - Component name included in log
}

func TestPrepareListValuesOptions(t *testing.T) {
    // Test cases:
    // - Vars query → Component field cleared, ComponentFilter set
    // - Non-vars query → Both Component and ComponentFilter set
    // - Empty query → proper defaults
}

// Error handling tests
func TestListValuesWithOptions_ErrorHandling(t *testing.T) {
    // Test cases:
    // - No component name → ErrComponentNameRequired
    // - InitCliConfig fails → wrapped error
    // - Component doesn't exist → ComponentDefinitionNotFoundError
    // - ExecuteDescribeStacks fails → wrapped error
    // - NoValuesFoundError → logs info, returns empty string
}

// Integration tests with mocks
func TestVarsCmd_ComponentVarsNotFoundError(t *testing.T) {
    // Test graceful handling when component has no vars
}

func TestVarsCmd_NoValuesFoundError(t *testing.T) {
    // Test graceful handling when no values match query
}
```

**Files to create/modify**:
- `cmd/list/values_test.go` - Add ~150 lines of tests

**Estimated impact**: +37 percentage points

#### 1.2 `cmd/list/settings.go` (28.57% → 80%+)

**Target**: Add 54 lines of test coverage

**Test scenarios to add**:
```go
func TestLogNoSettingsFoundMessage(t *testing.T) {
    // Test cases:
    // - With component filter → logs "No settings found" with component
    // - Without component filter → logs generic message
}

func TestListSettingsWithOptions_ErrorHandling(t *testing.T) {
    // Test cases:
    // - InitConfigError → returns wrapped error
    // - Component doesn't exist → ComponentDefinitionNotFoundError
    // - DescribeStacksError → returns wrapped error
    // - NoValuesFoundError → logs info, returns empty string
}

func TestListSettingsWithOptions_CSVDelimiter(t *testing.T) {
    // Test cases:
    // - CSV format with TSV delimiter → changes to comma
    // - CSV format with custom delimiter → preserves delimiter
    // - Other formats → preserves delimiter
}

func TestListSettingsWithOptions_ComponentFilter(t *testing.T) {
    // Test cases:
    // - Empty args → no component filter
    // - Args[0] present → component filter set
    // - Component exists → proceeds normally
    // - Component doesn't exist → returns error
}
```

**Files to create/modify**:
- `cmd/list/settings_test.go` - Add ~120 lines of tests

**Estimated impact**: +26 percentage points

#### 1.3 `cmd/list/metadata.go` (31.64% → 80%+)

**Target**: Add 53 lines of test coverage

**Test scenarios to add**:
```go
func TestSetupMetadataOptions(t *testing.T) {
    // Test cases:
    // - Empty query → defaults to ".metadata"
    // - Custom query → preserves query
    // - Component filter → sets ComponentFilter correctly
    // - All option combinations
}

func TestLogNoMetadataFoundMessage(t *testing.T) {
    // Test cases:
    // - With component filter
    // - Without component filter
}

func TestListMetadataWithOptions_ErrorHandling(t *testing.T) {
    // Test cases:
    // - CSV delimiter defaults
    // - InitConfigError handling
    // - Component existence check
    // - DescribeStacksError handling
    // - NoValuesFoundError handling
}

func TestListMetadataWithOptions_QueryDefault(t *testing.T) {
    // Test that empty query defaults to ".metadata"
}
```

**Files to create/modify**:
- `cmd/list/metadata_test.go` - Add ~110 lines of tests

**Estimated impact**: +21 percentage points

### Phase 2: High Priority Coverage (Files with 17-26% coverage)

#### 2.1 `cmd/list/stacks.go` (17.14% → 80%+)

**Target**: Add 28 lines of test coverage

**Test scenarios to add**:
```go
func TestListStacksWithOptions_ErrorHandling(t *testing.T) {
    // Test cases:
    // - InitCliConfig fails → returns formatted error
    // - ExecuteDescribeStacks fails → returns formatted error
    // - FilterAndListStacks error → propagates error
}

func TestListStacksWithOptions_ComponentFilter(t *testing.T) {
    // Test cases:
    // - Empty component → returns all stacks
    // - With component → filters by component
}

func TestStacksCmd_EmptyResults(t *testing.T) {
    // Test cases:
    // - No stacks found → displays "No stacks found" info message
    // - Stacks found → displays colored output
}

func TestStacksCmd_OutputFormatting(t *testing.T) {
    // Test that output uses theme.Colors.Success
    // Test newline handling
}
```

**Files to create/modify**:
- `cmd/list/stacks_test.go` - Add ~90 lines of tests

**Estimated impact**: +33 percentage points

#### 2.2 `cmd/list/instances.go` (20.00% → 80%+)

**Target**: Add 23 lines of test coverage

**Test scenarios to add**:
```go
func TestListInstancesWithOptions_ErrorHandling(t *testing.T) {
    // Test cases:
    // - InitCliConfig fails
    // - ExecuteDescribeStacks fails
    // - FilterAndListInstances fails
}

func TestListInstancesWithOptions_StackPattern(t *testing.T) {
    // Test cases:
    // - Empty pattern → all instances
    // - With pattern → filtered instances
}

func TestListInstancesWithOptions_Upload(t *testing.T) {
    // Test cases:
    // - Upload flag true → calls UploadInstances
    // - Upload fails → returns error
    // - Upload succeeds → returns success message
}
```

**Files to create/modify**:
- `cmd/list/instances_test.go` - Add ~85 lines of tests

**Estimated impact**: +30 percentage points

#### 2.3 `cmd/list/cmd_utils.go` (25.00% → 80%+)

**Target**: Add 16 lines of test coverage

**Note**: This file doesn't exist in the current directory listing. Need to verify file name.

#### 2.4 `cmd/list/components.go` (25.71% → 80%+)

**Target**: Add 25 lines of test coverage

**Test scenarios to add**:
```go
func TestListComponentsWithOptions_ErrorHandling(t *testing.T) {
    // Test cases:
    // - InitCliConfig fails
    // - ExecuteDescribeStacks fails
    // - FilterAndListComponents fails
}

func TestListComponentsWithOptions_StackPattern(t *testing.T) {
    // Test cases:
    // - Empty pattern → all components
    // - With pattern → filtered components
}

func TestListComponentsWithOptions_OutputFormatting(t *testing.T) {
    // Test cases:
    // - Empty result → "No components found"
    // - Components found → colored output with theme
}
```

**Files to create/modify**:
- `cmd/list/components_test.go` - Add ~80 lines of tests

**Estimated impact**: +29 percentage points

### Phase 3: Medium Priority Coverage (Files with 33-39% coverage)

#### 3.1 `cmd/list/utils.go` (38.59% → 80%+)

**Target**: Add 33 lines of test coverage

**Test scenarios to add**:
```go
func TestCheckAtmosConfig(t *testing.T) {
    // Test cases:
    // - InitCliConfig fails → returns error
    // - skipStackCheck true → skips validation
    // - skipStackCheck false + stacks dir exists → succeeds
    // - skipStackCheck false + stacks dir missing → returns error
}

func TestStackFlagCompletion(t *testing.T) {
    // Test cases:
    // - Component arg provided → filters stacks by component
    // - No component arg → returns all stacks
    // - listStacksForComponent fails → returns empty with directive
    // - listAllStacks fails → returns empty with directive
}

func TestListStacksForComponent(t *testing.T) {
    // Test cases:
    // - InitCliConfig fails → returns error
    // - ExecuteDescribeStacks fails → returns error
    // - FilterAndListStacks succeeds → returns filtered stacks
}

func TestListAllStacks(t *testing.T) {
    // Test cases:
    // - InitCliConfig fails → returns formatted error
    // - ExecuteDescribeStacks fails → returns formatted error
    // - FilterAndListStacks succeeds → returns all stacks
}

func TestNewCommonListParser(t *testing.T) {
    // Test cases:
    // - No additional options → creates parser with base flags
    // - With additional options → merges options correctly
    // - Verify all common flags are present (format, max-columns, delimiter, stack, query)
    // - Verify environment variable bindings
}
```

**Files to create/modify**:
- Create `cmd/list/utils_test.go` - ~140 lines

**Estimated impact**: +20 percentage points

#### 3.2 `cmd/list/themes.go` (33.33% → 80%+)

**Target**: Add 14 lines of test coverage

**Test scenarios to add**:
```go
func TestFilterRecommendedThemes(t *testing.T) {
    // Test cases:
    // - Active theme is recommended → included once
    // - Active theme not recommended → added to list
    // - No active theme → only recommended themes
    // - Themes sorted by name
}

func TestFormatSimpleOutput(t *testing.T) {
    // Test non-TTY output formatting
    // - Header formatting
    // - Theme row formatting
    // - Footer with counts
    // - Active theme indicator
}

func TestGetThemeType(t *testing.T) {
    // Test cases:
    // - IsDark true → returns "Dark"
    // - IsDark false → returns "Light"
}

func TestGetThemeSource(t *testing.T) {
    // Test cases:
    // - Credits with link → returns link
    // - Credits with name only → returns name
    // - No credits → returns empty string
}
```

**Files to create/modify**:
- `cmd/list/themes_test.go` - Add ~90 lines

**Estimated impact**: +23 percentage points

### Phase 4: Final Improvements

#### 4.1 `cmd/list/vendor.go` (74.35% → 80%+)

**Target**: Add 6 lines of test coverage

**Test scenarios to add**:
```go
func TestListVendorWithOptions(t *testing.T) {
    // Test cases:
    // - InitCliConfig fails → returns error
    // - FilterAndListVendor succeeds → returns output
}

func TestObfuscateHomeDirInOutput_EdgeCases(t *testing.T) {
    // Additional test cases:
    // - Empty home dir → returns output unchanged
    // - GetHomeDir error → returns output unchanged
    // - Unicode characters in paths
}

func TestVendorCmd_OutputObfuscation(t *testing.T) {
    // Integration test:
    // - Verify home directory is obfuscated in actual command output
}
```

**Files to create/modify**:
- `cmd/list/vendor_test.go` - Add ~40 lines

**Estimated impact**: +6 percentage points

## Testing Strategy

### Test Quality Guidelines

All new tests MUST follow these principles (from CLAUDE.md):

1. **Test behavior, not implementation** - Verify inputs/outputs, not internal state
2. **No stub/tautological tests** - Either implement the function or remove the test
3. **Use dependency injection** - Avoid hard dependencies on `os.Exit`, `CheckErrorPrintAndExit`
4. **Real scenarios only** - Use production-like inputs, not contrived data
5. **Use `errors.Is()` for error checking** - Use `assert.ErrorIs(err, ErrSentinel)` for Atmos errors and stdlib errors
6. **String matching only for third-party errors** - Or testing specific message formatting

### Test Organization

Each test file should:
- Use table-driven tests for comprehensive coverage
- Group tests by function/feature
- Include both positive and negative test cases
- Test error paths explicitly
- Use descriptive test names (e.g., `TestListValues_ComponentNotFound`)

### Mocking Strategy

- Mock external dependencies (config loading, stack processing)
- Use interfaces for testability
- Generate mocks with `go.uber.org/mock/mockgen`
- Keep mocks in test files or separate `_test.go` files

### Execution Plan

**Phase 1** (Week 1): Critical files - values.go, settings.go, metadata.go
- Estimated: +84 percentage points across 3 files
- Impact: Patch coverage 36% → ~62%

**Phase 2** (Week 2): High priority - stacks.go, instances.go, components.go
- Estimated: +92 percentage points across 3 files
- Impact: Patch coverage ~62% → ~78%

**Phase 3** (Week 3): Medium priority - utils.go, themes.go
- Estimated: +43 percentage points across 2 files
- Impact: Patch coverage ~78% → ~84%

**Phase 4** (Week 4): Final improvements - vendor.go, cleanup
- Estimated: +6 percentage points
- Impact: Patch coverage ~84% → **85%+**

## Success Metrics

### Coverage Targets

- **Overall project coverage**: 71.00% → **80%+** (9 percentage point increase)
- **Patch coverage**: 36.39% → **80%+** (43.61 percentage point increase)
- **cmd/list package coverage**: ~45% average → **80%+** average

### File-Specific Targets

All files in `cmd/list/` should achieve:
- **Minimum**: 75% coverage
- **Target**: 80%+ coverage
- **Ideal**: 85%+ coverage

### Quality Metrics

- **Zero tautological tests** - All tests validate real behavior
- **All error paths covered** - Every error return has a test
- **Edge cases documented** - Empty values, nil inputs, malformed data tested
- **Integration points verified** - Viper binding, env vars, flag precedence

## Risk Mitigation

### Challenges

1. **Integration test dependencies** - Some functions require full Atmos config
   - **Mitigation**: Use mocks and dependency injection
   - **Mitigation**: Create minimal test fixtures

2. **External dependencies** - ExecuteDescribeStacks, InitCliConfig
   - **Mitigation**: Mock these interfaces
   - **Mitigation**: Create test helpers that provide fake configs

3. **Time constraints** - 4-week timeline is aggressive
   - **Mitigation**: Prioritize critical files first
   - **Mitigation**: Parallelize work if multiple contributors

### Validation Strategy

After each phase:
1. Run `make testacc-cover` to verify coverage improvements
2. Review CodeCov reports to identify remaining gaps
3. Ensure no existing tests are broken
4. Verify test quality (no stubs/tautological tests)

## Appendix: Example Tests

### Example: High-Quality Error Handling Test

```go
func TestListValuesWithOptions_ComponentNotFound(t *testing.T) {
    opts := &ValuesOptions{
        Format:           "json",
        ProcessTemplates: true,
        ProcessFunctions: true,
    }
    args := []string{"nonexistent-component"}

    output, err := listValuesWithOptions(opts, args)

    assert.Error(t, err)
    assert.Empty(t, output)

    var notFoundErr *listerrors.ComponentDefinitionNotFoundError
    assert.ErrorAs(t, err, &notFoundErr)
    assert.Equal(t, "nonexistent-component", notFoundErr.Component)
}
```

### Example: Table-Driven Test

```go
func TestGetFilterOptionsFromValues(t *testing.T) {
    testCases := []struct {
        name              string
        opts              *ValuesOptions
        expectedQuery     string
        expectedDelimiter string
    }{
        {
            name: "vars flag sets query to .vars",
            opts: &ValuesOptions{
                Vars:      true,
                Query:     ".custom",
                Format:    "json",
                Delimiter: "\t",
            },
            expectedQuery:     ".vars",
            expectedDelimiter: "\t",
        },
        {
            name: "CSV format changes TSV delimiter to comma",
            opts: &ValuesOptions{
                Format:    "csv",
                Delimiter: "\t", // TSV default
            },
            expectedQuery:     "",
            expectedDelimiter: ",",
        },
        {
            name: "custom query preserved when vars false",
            opts: &ValuesOptions{
                Query:  ".vars.region",
                Format: "yaml",
            },
            expectedQuery:     ".vars.region",
            expectedDelimiter: "",
        },
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            filterOpts := getFilterOptionsFromValues(tc.opts)

            assert.Equal(t, tc.expectedQuery, filterOpts.Query)
            assert.Equal(t, tc.expectedDelimiter, filterOpts.Delimiter)
        })
    }
}
```

## Conclusion

This plan provides a systematic approach to increasing test coverage from 36.39% (patch) and 71% (project) to **80%+** for both metrics. By focusing on high-value tests that cover error paths, edge cases, and integration points, we ensure both coverage metrics and code quality improve.

The phased approach allows for incremental progress validation and adjustment based on actual implementation experience. Success depends on maintaining test quality standards and avoiding low-value tautological tests.
