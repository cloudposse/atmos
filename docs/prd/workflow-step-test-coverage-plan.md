# Workflow Step Test Coverage Improvement Plan

## Executive Summary

**Current State**: `pkg/workflow/step/` package has 26.3% coverage.

**Goal**: Increase coverage to **80%+** (53.7 percentage point improvement).

**Strategy**: Focus on Execute() methods across all step handlers, leveraging existing patterns from test files and using mocks/dependency injection for testability.

## Coverage Analysis

### Current Coverage by File

Based on fresh coverage analysis, here are the files sorted by priority:

| File | Functions | Covered | Coverage | Priority |
|------|-----------|---------|----------|----------|
| `output_mode.go` | 10 | 1 | ~10% | **CRITICAL** |
| `shell.go` | 3 | 1 | ~33% | **CRITICAL** |
| `atmos.go` | 11 | 3 | ~27% | **CRITICAL** |
| `spin.go` | 7 | 2 | ~29% | **HIGH** |
| `table.go` | 8 | 2 | ~25% | **HIGH** |
| `style.go` | 6 | 2 | ~33% | **HIGH** |
| `stage.go` | 5 | 1 | ~20% | **HIGH** |
| `file.go` | 6 | 2 | ~33% | **HIGH** |
| `filter.go` | 7 | 2 | ~29% | **HIGH** |
| `choose.go` | 7 | 2 | ~29% | **MEDIUM** |
| `input.go` | 3 | 2 | ~67% | **MEDIUM** |
| `confirm.go` | 3 | 2 | ~67% | **MEDIUM** |
| `log.go` | 5 | 1 | ~20% | **MEDIUM** |
| `pager.go` | ? | ? | Low | **MEDIUM** |
| `write.go` | 3 | 2 | ~67% | **LOW** |
| `sleep.go` | 3 | 1 | ~33% | **LOW** |
| `exit.go` | 3 | 2 | ~67% | **LOW** |
| `clear.go` | 3 | 2 | ~67% | **LOW** |
| `env.go` | 3 | 3 | ~100% | **DONE** |
| `title.go` | 3 | 2 | ~67% | **LOW** |
| `toast.go` | 3 | 3 | ~100% | **DONE** |
| `linebreak.go` | 3 | 1 | ~33% | **LOW** |
| `alert.go` | 3 | 2 | ~67% | **LOW** |
| `format.go` | 3 | 2 | ~67% | **LOW** |
| `markdown.go` | 3 | 3 | ~100% | **DONE** |

### Files with Full Coverage (No Changes Needed)

- `executor.go` - Well tested (existing executor_test.go)
- `types.go` - Full coverage
- `variables.go` - Nearly full coverage (88.9%)
- `registry.go` - Implicitly tested through handler registration
- `handler_base.go` - Partially covered (NewBaseHandler, GetName, GetCategory, RequiresTTY, ValidateRequired, ResolveContent, ResolvePrompt, ResolveCommand)
- `show_config.go` - Full coverage

### Existing Test Files

| Test File | Covers |
|-----------|--------|
| `executor_test.go` | Core executor, variables, result chaining |
| `variables_test.go` | Variable management, template resolution |
| `output_mode_test.go` | OutputMode enum validation |
| `show_config_test.go` | ShowConfig handler |
| `command_handlers_test.go` | Registration + validation for atmos/shell |
| `interactive_handlers_test.go` | Registration + validation for input/confirm/choose/filter/file/write |
| `output_handlers_test.go` | Registration + validation + some execution for spin/table/pager/format/join/style |
| `ui_handlers_test.go` | Registration + validation for toast/markdown/alert/title/log/linebreak/clear/env/exit/stage/sleep |

## Testing Challenges

### Challenge 1: TTY-Dependent Interactive Handlers

Many handlers require TTY for user input (input, confirm, choose, filter, file, write).

**Solution**:
- Test validation paths (already covered)
- Test CheckTTY error path when TTY not available
- Test helper methods (resolveOptions, resolveDefault, etc.) in isolation
- Mock the `huh` form library or test with non-interactive fallbacks

### Challenge 2: Command Execution (shell, atmos, spin)

These handlers execute actual shell commands.

**Solution**:
- Create mock command execution interfaces
- Test with simple commands like `echo` or `true`
- Test error paths with commands that fail
- Use short timeouts in tests

### Challenge 3: Output Writers

OutputModeWriter executes commands and handles output modes.

**Solution**:
- Mock the command execution
- Test each output mode independently
- Test viewport, raw, log, and none modes

## Implementation Plan

### Phase 1: Critical Coverage (output_mode.go, shell.go, atmos.go)

These files have the most uncovered code and are foundational.

#### 1.1 `output_mode.go` (10% → 80%+)

**Functions to test:**
- `Execute()` - Main execution dispatch
- `executeViewport()` - Viewport output mode
- `executeRaw()` - Raw output mode
- `executeLog()` - Log output mode
- `executeNone()` - None output mode
- `fallbackToLog()` - Fallback handling
- `formatStepFooter()`, `formatFailedFooter()`, `formatSuccessFooter()`

**Test scenarios:**
```go
func TestOutputModeWriter_Execute(t *testing.T) {
    // Test each output mode
    tests := []struct {
        name       string
        mode       OutputMode
        expectCall string
    }{
        {"viewport mode", OutputModeViewport, "executeViewport"},
        {"raw mode", OutputModeRaw, "executeRaw"},
        {"log mode", OutputModeLog, "executeLog"},
        {"none mode", OutputModeNone, "executeNone"},
    }
}

func TestOutputModeWriter_ExecuteLog(t *testing.T) {
    // Test successful command
    // Test failed command
    // Test command output capture
}

func TestOutputModeWriter_ExecuteRaw(t *testing.T) {
    // Test stdout/stderr separation
    // Test command failure handling
}

func TestOutputModeWriter_Formatters(t *testing.T) {
    // Test formatStepFooter
    // Test formatFailedFooter
    // Test formatSuccessFooter
}
```

**Estimated impact**: +70% coverage for this file

#### 1.2 `shell.go` (33% → 80%+)

**Functions to test:**
- `Execute()` - Main execution
- `ExecuteWithWorkflow()` - Workflow-aware execution
- `getExitCode()` - Exit code extraction

**Test scenarios:**
```go
func TestShellHandler_Execute(t *testing.T) {
    tests := []struct {
        name        string
        command     string
        expectError bool
        exitCode    int
    }{
        {"successful echo", "echo hello", false, 0},
        {"command with output", "echo -n test", false, 0},
        {"failing command", "exit 1", true, 1},
        {"command not found", "nonexistent_cmd_12345", true, 127},
    }
}

func TestShellHandler_Execute_WorkingDirectory(t *testing.T) {
    // Test command runs in specified directory
}

func TestShellHandler_Execute_Environment(t *testing.T) {
    // Test environment variables are passed
    // Test env template resolution
}

func TestShellHandler_ExecuteWithWorkflow(t *testing.T) {
    // Test output mode from workflow
    // Test viewport config from workflow
}

func TestGetExitCode(t *testing.T) {
    // Test exec.ExitError extraction
    // Test non-ExitError defaults to 1
}
```

**Estimated impact**: +47% coverage for this file

#### 1.3 `atmos.go` (27% → 80%+)

**Functions to test:**
- `Execute()` - Main execution
- `prepareExecution()` - Prepare command args
- `resolveStack()` - Stack resolution
- `resolveWorkDir()` - Working directory resolution
- `resolveEnvVars()` - Environment variable resolution
- `runAtmosCommand()` - Atmos command execution
- `buildAtmosResult()` - Result building
- `ExecuteWithWorkflow()` - Workflow execution
- `containsStackFlag()` - Already covered (100%)

**Test scenarios:**
```go
func TestAtmosHandler_PrepareExecution(t *testing.T) {
    // Test command parsing
    // Test stack injection
    // Test working directory resolution
}

func TestAtmosHandler_ResolveStack(t *testing.T) {
    // Test explicit stack
    // Test --stack flag detection
    // Test default stack
}

func TestAtmosHandler_ResolveEnvVars(t *testing.T) {
    // Test env map resolution
    // Test template in env values
}

func TestAtmosHandler_BuildAtmosResult(t *testing.T) {
    // Test success result
    // Test error result
    // Test metadata population
}
```

**Estimated impact**: +53% coverage for this file

### Phase 2: High Priority (spin.go, table.go, style.go, stage.go)

#### 2.1 `spin.go` (29% → 80%+)

**Functions to test:**
- `Execute()` - Spinner execution
- `prepareExecution()` - Command preparation
- `createExecContext()` - Context creation with timeout
- `runCommand()` - Command execution
- `buildResult()` - Result building

**Test scenarios:**
```go
func TestSpinHandler_Execute(t *testing.T) {
    // Test successful spin execution
    // Test timeout handling
    // Test command failure
}

func TestSpinHandler_PrepareExecution(t *testing.T) {
    // Test command resolution
    // Test working directory
    // Test environment
}

func TestSpinHandler_CreateExecContext(t *testing.T) {
    // Test default timeout
    // Test custom timeout
    // Test invalid timeout format
}
```

**Estimated impact**: +51% coverage for this file

#### 2.2 `table.go` (25% → 80%+)

**Functions to test:**
- `Execute()` - Table rendering
- `executeContentTable()` - Content-based table
- `executeDataTable()` - Data-based table
- `determineColumns()` - Column detection
- `buildHeader()` - Header row
- `buildRows()` - Data rows
- `addTitle()` - Title addition

**Test scenarios:**
```go
func TestTableHandler_ExecuteContent(t *testing.T) {
    // Test pre-formatted content display
}

func TestTableHandler_ExecuteData(t *testing.T) {
    // Test data table rendering
    // Test column detection
    // Test custom columns
}

func TestTableHandler_DetermineColumns(t *testing.T) {
    // Test auto column detection from data
    // Test explicit columns override
}

func TestTableHandler_BuildRows(t *testing.T) {
    // Test row building with various data types
    // Test missing column handling
}
```

**Estimated impact**: +55% coverage for this file

#### 2.3 `style.go` (33% → 80%+)

**Functions to test:**
- `Execute()` - Style rendering
- `renderMarkdown()` - Markdown rendering
- `buildStyle()` - Style construction
- `getBorderStyle()` - Border style mapping
- `parseSpacing()` - Spacing parser

**Test scenarios:**
```go
func TestStyleHandler_Execute(t *testing.T) {
    // Test basic styling
    // Test markdown rendering
    // Test border styles
}

func TestStyleHandler_BuildStyle(t *testing.T) {
    // Test foreground/background
    // Test bold/italic/underline
    // Test padding/margin
}

func TestStyleHandler_GetBorderStyle(t *testing.T) {
    // Test all border types: normal, rounded, double, thick, hidden
}

func TestStyleHandler_ParseSpacing(t *testing.T) {
    // Test single value
    // Test two values (vertical/horizontal)
    // Test four values (top/right/bottom/left)
}
```

**Estimated impact**: +47% coverage for this file

#### 2.4 `stage.go` (20% → 80%+)

**Functions to test:**
- `Validate()` - Stage validation
- `Execute()` - Stage execution
- `formatStageOutput()` - Output formatting
- `CountStages()` - Stage counting

**Test scenarios:**
```go
func TestStageHandler_Validate(t *testing.T) {
    // Test title required
    // Test valid stage
}

func TestStageHandler_Execute(t *testing.T) {
    // Test stage display
    // Test stage counter update
}

func TestStageHandler_FormatStageOutput(t *testing.T) {
    // Test format with counter
    // Test format without counter
}

func TestCountStages(t *testing.T) {
    // Test counting stage steps in workflow
    // Test empty workflow
}
```

**Estimated impact**: +60% coverage for this file

### Phase 3: Medium Priority (file.go, filter.go, choose.go, input.go, confirm.go, log.go, pager.go)

#### 3.1 Interactive Handlers (file, filter, choose, input, confirm)

Since these require TTY, focus on:
- Testing validation (already partially covered)
- Testing helper methods
- Testing CheckTTY error path

```go
func TestInputHandler_CheckTTY_NoTTY(t *testing.T) {
    // Test error when TTY not available
    // Verify error is ErrStepTTYRequired
}

func TestChooseHandler_ResolveOptions(t *testing.T) {
    // Test options resolution from step
    // Test template expansion in options
}

func TestFilterHandler_ResolveOptions(t *testing.T) {
    // Test filtering options
    // Test multi-select vs single-select setup
}

func TestFileHandler_ResolveStartPath(t *testing.T) {
    // Test path resolution
    // Test default to current directory
}

func TestFileHandler_MatchesExtensions(t *testing.T) {
    // Test extension filtering
    // Test case sensitivity
}
```

#### 3.2 `log.go` (20% → 80%+)

**Functions to test:**
- `Validate()` - Message validation
- `Execute()` - Log output
- `buildKeyvals()` - Key-value builder
- `getLogLevel()` - Level mapping

```go
func TestLogHandler_Execute(t *testing.T) {
    // Test all log levels: debug, info, warn, error
    // Test with keyvals
    // Test without message (just keyvals)
}

func TestLogHandler_BuildKeyvals(t *testing.T) {
    // Test keyval construction
    // Test template resolution in keyvals
}

func TestLogHandler_GetLogLevel(t *testing.T) {
    // Test level mapping
    // Test default level
}
```

### Phase 4: Low Priority (Simple Handlers)

These handlers are simple and require minimal testing beyond what exists:

#### Simple Handler Tests

```go
func TestAlertHandler_Execute(t *testing.T) {
    // Test all alert levels: info, warn, error, success
}

func TestTitleHandler_Execute(t *testing.T) {
    // Test title display
}

func TestClearHandler_Execute(t *testing.T) {
    // Test screen clear
}

func TestSleepHandler_Execute(t *testing.T) {
    // Test sleep duration
    // Test invalid duration
}

func TestExitHandler_Execute(t *testing.T) {
    // Test exit with code 0
    // Test exit with custom code
}

func TestLinebreakHandler_Execute(t *testing.T) {
    // Test default count
    // Test custom count
}

func TestWriteHandler_Execute(t *testing.T) {
    // Test file writing
    // Test path resolution
    // Test permission handling
}
```

## Test Organization

### New Test Files to Create

1. **`output_mode_execution_test.go`** - Tests for OutputModeWriter execution
2. **`shell_execution_test.go`** - Tests for shell command execution
3. **`atmos_execution_test.go`** - Tests for atmos command execution
4. **`handler_base_test.go`** - Tests for CheckTTY and base handler methods

### Existing Test Files to Extend

1. **`output_handlers_test.go`** - Add Execute() tests for spin, table, format, join, style
2. **`ui_handlers_test.go`** - Add Execute() tests for toast, markdown, alert, title, log, linebreak, clear, env, exit, stage, sleep
3. **`interactive_handlers_test.go`** - Add helper method tests for input, confirm, choose, filter, file, write
4. **`command_handlers_test.go`** - Add Execute() tests (with mocks)

## Testing Patterns to Follow

### Pattern 1: Table-Driven Tests (from existing tests)

```go
func TestHandlerValidation(t *testing.T) {
    handler, ok := Get("handler_name")
    require.True(t, ok)

    tests := []struct {
        name      string
        step      *schema.WorkflowStep
        expectErr bool
    }{
        {"valid case", &schema.WorkflowStep{...}, false},
        {"missing required", &schema.WorkflowStep{...}, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := handler.Validate(tt.step)
            if tt.expectErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Pattern 2: Execute Tests with Variables

```go
func TestHandlerExecution(t *testing.T) {
    handler, ok := Get("handler_name")
    require.True(t, ok)

    vars := NewVariables()
    vars.Set("prev_step", NewStepResult("value"))

    step := &schema.WorkflowStep{
        Name:    "test",
        Type:    "handler_name",
        Content: "{{ .steps.prev_step.value }}",
    }

    result, err := handler.Execute(context.Background(), step, vars)
    require.NoError(t, err)
    assert.Equal(t, "expected_value", result.Value)
}
```

### Pattern 3: Error Testing with Sentinel Errors

```go
func TestHandlerError(t *testing.T) {
    handler, ok := Get("handler_name")
    require.True(t, ok)

    step := &schema.WorkflowStep{...}
    vars := NewVariables()

    _, err := handler.Execute(context.Background(), step, vars)
    assert.Error(t, err)
    assert.True(t, errors.Is(err, errUtils.ErrExpectedSentinel))
}
```

## Execution Timeline

**Phase 1** (Critical): output_mode.go, shell.go, atmos.go
- Focus: Command execution and output handling
- Estimated tests: ~200 lines
- Impact: Coverage 26% → ~50%

**Phase 2** (High): spin.go, table.go, style.go, stage.go
- Focus: Complex output handlers
- Estimated tests: ~250 lines
- Impact: Coverage ~50% → ~65%

**Phase 3** (Medium): file.go, filter.go, choose.go, input.go, confirm.go, log.go, pager.go
- Focus: Interactive and logging handlers
- Estimated tests: ~200 lines
- Impact: Coverage ~65% → ~75%

**Phase 4** (Low): Simple handlers (alert, title, clear, sleep, exit, linebreak, write)
- Focus: Complete coverage
- Estimated tests: ~100 lines
- Impact: Coverage ~75% → **80%+**

## Success Metrics

### Coverage Targets

- **Package overall**: 26.3% → **80%+**
- **Critical files (output_mode, shell, atmos)**: <30% → **80%+**
- **High priority files**: <35% → **80%+**

### Quality Metrics

- All error paths tested with `errors.Is()`
- No tautological tests
- All Execute() methods have at least one test
- Template resolution tested
- Edge cases documented and tested

## Risk Mitigation

### Challenge: Command Execution in Tests

**Risk**: Tests that execute real commands may be slow or flaky.

**Mitigation**:
- Use fast commands (`echo`, `true`, `false`)
- Set short timeouts
- Use context cancellation for control
- Consider mocking exec.Command for complex scenarios

### Challenge: TTY-Dependent Code

**Risk**: Interactive handlers can't be easily unit tested.

**Mitigation**:
- Focus on validation and helper methods
- Test error paths (no TTY available)
- Accept that some paths require integration tests
- Document which handlers need manual testing

### Challenge: UI Output Verification

**Risk**: Output contains ANSI codes and formatting.

**Mitigation**:
- Test result values rather than formatted output
- Use result.Value and result.Metadata
- Leave visual verification for integration tests

## Appendix: File-by-File Function List

### output_mode.go (10 functions)
- `NewOutputModeWriter` ✓
- `Execute` ✗
- `executeViewport` ✗
- `executeRaw` ✗
- `executeLog` ✗
- `fallbackToLog` ✗
- `formatStepFooter` ✗
- `formatFailedFooter` ✗
- `formatSuccessFooter` ✗
- `executeNone` ✗

### shell.go (4 functions)
- `init` ✓
- `Validate` ✓
- `Execute` ✗
- `ExecuteWithWorkflow` ✗
- `getExitCode` (internal) ✗

### atmos.go (11 functions)
- `init` ✓
- `Validate` ✓
- `Execute` ✗
- `prepareExecution` ✗
- `resolveStack` ✗
- `resolveWorkDir` ✗
- `resolveEnvVars` ✗
- `runAtmosCommand` ✗
- `buildAtmosResult` ✗
- `ExecuteWithWorkflow` ✗
- `containsStackFlag` ✓
