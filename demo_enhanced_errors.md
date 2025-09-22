# Enhanced Error Messages in Atmos Validate

## Problem
When `atmos validate stacks` encounters a merge error (like conflicting types between array and string), users experience two issues:

1. **Duplicate error messages** - The merge.go file was printing errors directly to stderr before returning them, causing the same error to appear multiple times
2. **No file context** - No indication of which file contains the problem or the import chain that led to it

```
cannot override two slices with different type ([]interface {}, string)
cannot override two slices with different type ([]interface {}, string)
cannot override two slices with different type ([]interface {}, string)
...
```

Even with debug mode enabled, there's no file context to help identify the issue.

## Solution
We implemented two key improvements:

1. **Removed duplicate error printing** - Eliminated direct stderr printing from merge.go, ensuring errors are only reported once through proper channels
2. **Added MergeContext wrapper** - Tracks file paths and import chains during merge operations, providing clear, actionable error messages

## Before (Original Error)
```
cannot override two slices with different type ([]interface {}, string)
```

## After (Enhanced Error with MergeContext)
```
cannot override two slices with different type ([]interface {}, string)

  File being processed: stacks/deploy/prod/us-east-1.yaml
  Import chain:
    → stacks/orgs/acme/_defaults.yaml
      → stacks/catalog/vpc/defaults.yaml
      → stacks/catalog/vpc/base.yaml
      → stacks/mixins/region/us-east-1.yaml
      → stacks/mixins/account/prod.yaml
      → stacks/deploy/prod/us-east-1.yaml

  Likely cause: A key is defined as an array in one file and as a string in another.
  Debug hint: Check the files above for keys that have different types.
  Common issues:
    - vars defined as both array and string
    - settings with inconsistent types across imports
    - overrides attempting to change field types
```

## Implementation Details

### 1. Fixed Duplicate Error Printing
- **Root Cause**: `merge.go` was using `theme.Colors.Error.Fprintln(color.Error, err.Error())` to print errors directly to stderr before returning them
- **Fix**: Removed all direct printing statements, now only returns errors for proper handling by the caller
- **Result**: Errors appear once with proper formatting through the logging system

### 2. MergeContext Structure
- Tracks the current file being processed
- Maintains the complete import chain
- Provides context-aware error formatting
- Adds helpful debugging hints for common merge errors

### 3. Integration Points
- `ProcessYAMLConfigFileWithContext` - Enhanced version that accepts MergeContext
- `MergeWithContext` - Wrapper around merge operations with context tracking
- All merge operations in stack processing now use context-aware versions

### 4. Backward Compatibility
- Original functions remain unchanged and delegate to context-aware versions with nil context
- No breaking changes to existing API
- Context is optional and degrades gracefully

## Test Coverage
- Unit tests for MergeContext functionality
- Tests verifying no duplicate error printing to stderr
- Integration tests for validate command
- Test fixtures demonstrating type mismatch scenarios
- Demo tests showing enhanced error formatting
- Tests for multiple merge operations without duplication

## Benefits
1. **Clear Error Location**: Users immediately know which file contains the problem
2. **Import Chain Visibility**: Shows the complete path of imports leading to the error
3. **Actionable Hints**: Provides specific guidance on what to look for and common issues
4. **Debugging Efficiency**: Dramatically reduces time to identify and fix configuration issues
5. **Non-Breaking**: Fully backward compatible with existing code

## Example Use Case
A user has a complex stack configuration with multiple imports:
- Base configuration defines `subnets` as an array
- Override configuration attempts to change `subnets` to a string
- The error occurs deep in the import chain

With the enhanced error messages, the user can:
1. See exactly which file is being processed when the error occurs
2. Trace back through the import chain to find conflicting definitions
3. Use the provided hints to identify the type mismatch
4. Fix the configuration quickly without extensive debugging