# /refactor Command

Analyzes code and guides refactoring to improve quality while maintaining functionality.

## Usage

```
/refactor [file|directory|function]
```

## Description

The `/refactor` command helps improve code quality by:
- Analyzing complexity and identifying problem areas
- Suggesting decomposition strategies
- Ensuring tests remain passing throughout
- Maintaining lint compliance
- Preventing common refactoring mistakes

## Examples

### Analyze a specific file
```
/refactor tools/gotcha/pkg/stream/event_processor.go
```

### Refactor a function
```
/refactor processEvent in event_processor.go
```

### Analyze entire package
```
/refactor tools/gotcha/pkg/stream/
```

## Process

1. **Analysis Phase**
   - Runs golangci-lint to identify issues
   - Measures cognitive and cyclomatic complexity
   - Checks function and file lengths
   - Identifies violation patterns

2. **Planning Phase**
   - Suggests decomposition strategy
   - Identifies extraction opportunities
   - Plans incremental changes
   - Estimates effort and risk

3. **Execution Phase**
   - Makes incremental changes
   - Compiles after each change
   - Runs tests continuously
   - Commits working states

4. **Verification Phase**
   - Confirms all tests pass
   - Validates lint compliance
   - Measures complexity reduction
   - Ensures coverage maintained

## Options

### --dry-run
Analyze and plan without making changes:
```
/refactor event_processor.go --dry-run
```

### --complexity-only
Focus only on reducing complexity:
```
/refactor processEvent --complexity-only
```

### --fix-tests
Update tests to match refactored code:
```
/refactor processor.go --fix-tests
```

## Complexity Thresholds

The command enforces these limits from `.golangci.yml`:

| Metric | Warning | Error |
|--------|---------|-------|
| Cognitive Complexity | 20 | 25 |
| Cyclomatic Complexity | 10 | 15 |
| Function Lines | 50 | 60 |
| Function Statements | 35 | 40 |
| File Lines | 400 | 500 |
| Nesting Depth | 3 | 4 |

## Common Refactoring Patterns

### Extract Method
Identifies and extracts cohesive code blocks:
```go
// Before: 100-line function
// After: 5 focused 20-line functions
```

### Replace Conditional with Polymorphism
Converts switch statements to interface implementations:
```go
// Before: Giant switch statement
// After: Strategy pattern with interfaces
```

### Introduce Parameter Object
Groups related parameters:
```go
// Before: func process(a, b, c, d, e string)
// After: func process(config ProcessConfig)
```

## Safety Features

- **Compilation Check**: Verifies code compiles after each change
- **Test Guard**: Ensures tests pass before and after
- **Rollback**: Can undo changes if tests fail
- **Incremental**: Makes small, reviewable changes
- **Coverage Protection**: Prevents coverage decrease

## Error Handling

The command will stop and report if:
- Compilation fails after a change
- Tests begin failing
- Coverage drops significantly
- Complexity increases instead of decreases
- File becomes longer instead of shorter

## Integration with Other Commands

Works well with:
- `/test` - Run tests before and after refactoring
- `/lint` - Check compliance throughout
- `/pr` - Create PR with refactoring changes

## Best Practices

1. **Start Small**: Refactor one function at a time
2. **Test First**: Ensure good test coverage before refactoring
3. **Compile Often**: Run `go build` after every change
4. **Commit Frequently**: Save working states
5. **Measure Impact**: Compare before/after metrics

## Troubleshooting

### "Function too complex to auto-refactor"
The function may need manual decomposition. Use `--dry-run` to see suggestions.

### "Tests failing after refactor"
Check if tests were too tightly coupled to implementation. May need test updates.

### "Coverage decreased"
New extracted functions may need additional test cases.

### "Compilation errors"
Ensure all imports are correct and types match after extraction.

## Metrics and Reporting

After refactoring, the command reports:
- Lines of code reduced
- Complexity reduction percentage
- Number of functions extracted
- Test coverage change
- Lint issues resolved

Example output:
```
Refactoring Summary:
- Split 336-line function into 8 functions (avg 42 lines)
- Reduced cognitive complexity from 142 to 18 (-87%)
- Extracted 3 helper packages
- Test coverage: 72% → 78% (+6%)
- Resolved 12 lint violations
```