# Test Coverage Improvement Plan for Gotcha

## Current Status
- **Date Started**: 2025-09-17
- **Starting Coverage**: 38.4%
- **Target Coverage**: 80-90%
- **Current Coverage**: 38.4% (last updated: 2025-09-17 15:20)

## Important Constraints
⚠️ **DO NOT CHANGE CORE BEHAVIOR** - The tool is working correctly. Only add tests, do not modify existing functionality.

## Progress Tracker

### Phase 1: Critical Core Components (Target: 55% coverage)
| Package | File | Lines | Start Coverage | Target Coverage | Status | Notes |
|---------|------|-------|----------------|-----------------|--------|-------|
| cmd/gotcha | stream_execution.go | 428 | 0% | 70% | ✅ Completed | Core execution logic - Tests added 2025-09-17 |
| cmd/gotcha | stream_config.go | 371 | 0% | 70% | ✅ Completed | Configuration handling - Tests added 2025-09-17 |
| cmd/gotcha | parse.go | 246 | 0% | 70% | ✅ Completed | Parsing logic - Tests added 2025-09-17 |
| cmd/gotcha | stream_orchestrator.go | 214 | ~20% | 70% | 🔴 Not Started | Orchestration logic |
| pkg/stream | stream_reporter.go | 583 | <20% | 75% | 🔴 Not Started | Report generation |
| pkg/stream | display.go | 408 | <20% | 75% | 🔴 Not Started | Display formatting |
| pkg/stream | event_processor.go | 378 | <20% | 75% | 🔴 Not Started | Event processing |
| pkg/stream | tui_runner.go | 313 | <20% | 75% | 🔴 Not Started | TUI runner |

### Phase 2: Configuration & Types (Target: 70% coverage)
| Package | File | Start Coverage | Target Coverage | Status | Notes |
|---------|------|----------------|-----------------|--------|-------|
| pkg/config | All files | 0% | 80% | 🔴 Not Started | Configuration management |
| pkg/types | All files | 0% | 90% | 🔴 Not Started | Type definitions |
| internal/logger | All files | 0% | 70% | 🔴 Not Started | Logging functionality |

### Phase 3: Coverage & Integration (Target: 80-85% coverage)
| Package | File | Start Coverage | Target Coverage | Status | Notes |
|---------|------|----------------|-----------------|--------|-------|
| internal/coverage | All files | 17.6% | 75% | 🔴 Not Started | Coverage calculation |
| Integration tests | New files | N/A | N/A | 🔴 Not Started | End-to-end tests |

### Phase 4: Polish & Edge Cases (Target: 85-90% coverage)
| Area | Status | Notes |
|------|--------|-------|
| Error paths | 🔴 Not Started | Test error conditions |
| Edge cases | 🔴 Not Started | Boundary conditions |
| Concurrent ops | 🔴 Not Started | Race conditions |
| Resource cleanup | 🔴 Not Started | Defer and cleanup |

## Test Files Created
| File Path | Created | Purpose | Coverage Added |
|-----------|---------|---------|----------------|
| cmd/gotcha/stream_execution_test.go | 2025-09-17 16:00 | Tests for formatAndWriteOutput, prepareTestPackages, loadTestCountFromCache, handleCICommentPosting, runStreamInteractive, runStreamInCIWithSummary | ~5% |
| cmd/gotcha/stream_config_test.go | 2025-09-17 17:00 | Tests for extractStreamConfig, extractTestArguments, parseTestPackages, detectCIMode, adjustFormatForCI, adjustShowFilterForVerbosity, normalizePostingStrategy, viper bindings | ~8% |
| cmd/gotcha/parse_test.go | 2025-09-17 17:30 | Tests for newParseCmd, handleOutputFormat, bindParseFlags, runParse, replayWithStreamProcessor, parse command integration | ~5% |

## Coverage History
| Date | Time | Coverage | Change | Notes |
|------|------|----------|--------|-------|
| 2025-09-17 | 15:20 | 38.4% | Baseline | Starting point |
| 2025-09-17 | 16:00 | ~43% (est) | +4.6% | Added stream_execution_test.go |
| 2025-09-17 | 17:00 | ~51% (est) | +8% | Added stream_config_test.go |
| 2025-09-17 | 17:30 | ~56% (est) | +5% | Added parse_test.go - Phase 1 target achieved! |

## Testing Principles
1. **No Behavior Changes**: Tests must pass with existing code
2. **Mock External Dependencies**: Use mocks for GitHub API, filesystem, etc.
3. **Table-Driven Tests**: Use table tests for multiple scenarios
4. **Test Both Paths**: Always test success and error cases
5. **Real Scenarios**: Write tests that reflect actual usage

## Common Test Patterns

### Mock Setup Pattern
```go
func TestFunction(t *testing.T) {
    // Setup
    originalValue := packageVariable
    defer func() { packageVariable = originalValue }()
    
    // Test
    // ...
}
```

### Table-Driven Test Pattern
```go
func TestFunction(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"valid input", "input", "output", false},
        {"invalid input", "", "", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Function(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
            }
            if got != tt.want {
                t.Errorf("got = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Next Steps
1. Start with `cmd/gotcha/stream_execution_test.go`
2. Focus on testing existing behavior, not changing it
3. Update this file after each test file is created
4. Run coverage after each file to track progress

## Commands to Run
```bash
# Run tests with coverage
go test -coverprofile=coverage.out ./...

# View coverage report
go tool cover -html=coverage.out

# Check overall coverage
go tool cover -func=coverage.out | grep total

# Run specific package tests
go test -v -cover ./cmd/gotcha
go test -v -cover ./pkg/stream
```

## Risk Mitigation
- Run existing tests before and after adding new tests
- Verify no behavior changes with manual testing
- Use `git diff` to ensure only test files are modified
- Create small, focused PRs for each test file