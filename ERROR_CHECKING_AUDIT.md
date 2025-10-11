# Error Checking Pattern Audit

This document tracks the audit of test error checking patterns as part of PR #1599 (error handling implementation).

## Goal

Replace brittle string matching (`assert.Contains(err.Error(), "...")`) with robust error checking (`errors.Is(err, ErrSentinel)`) where appropriate.

## When to Use `errors.Is()`

Use `errors.Is()` when checking for:
1. **Our sentinel errors** from `errors/errors.go` (e.g., `ErrMissingStack`, `ErrDownloadFile`)
2. **Standard library errors** (e.g., `fs.ErrNotExist`, `exec.ErrNotFound`, `io.EOF`)
3. **Any wrapped error** where you care about the underlying error type, not the message

## When String Matching is OK

String matching is acceptable for:
1. **Third-party library errors** without sentinel errors
2. **Dynamically constructed messages** where you need to verify specific interpolated values
3. **Error message formatting tests** where you explicitly test the user-facing message

## Cross-Platform OS Errors

Go provides cross-platform error sentinels in the standard library:

### File System Errors (`io/fs`)
- `fs.ErrNotExist` - "file does not exist"
- `fs.ErrPermission` - "permission denied"
- `fs.ErrExist` - "file already exists"
- `fs.ErrInvalid` - "invalid argument"
- `fs.ErrClosed` - "file already closed"

### Exec Errors (`os/exec`)
- `exec.ErrNotFound` - "executable file not found in $PATH"

### Example
```go
// ❌ WRONG: Platform-specific, brittle
assert.Contains(t, err.Error(), "executable file not found")

// ✅ CORRECT: Cross-platform, robust
assert.ErrorIs(t, err, exec.ErrNotFound)
```

## Audit Results

### High Priority - Our Sentinel Errors

Files checking our sentinel errors with string matching (should use `errors.Is()`):

#### Download Errors
- `pkg/downloader/file_downloader_test.go` (4 occurrences)
  - Checking "failed to download file" → should use `errors.Is(err, errUtils.ErrDownloadFile)`
- `pkg/config/imports_error_paths_test.go` (1 occurrence)
  - Checking "failed to download remote config" → should use `errors.Is(err, errUtils.ErrDownloadFile)`

#### OS Errors
- `internal/exec/packer_test.go` (1 occurrence) ✅ **FIXED IN PREVIOUS COMMIT**
  - Was checking "executable file not found" → should use `errors.Is(err, exec.ErrNotFound)`

### Medium Priority - Review Needed

Files with many error string checks that need case-by-case review:

1. `pkg/list/errors/types_test.go` (22 occurrences)
   - Testing error message formatting - **likely OK as-is** (testing user-facing messages)

2. `internal/exec/copy_glob_error_paths_test.go` (15 occurrences)
   - Need to review each case

3. `internal/exec/stack_processor_process_stacks_helpers_test.go` (7 occurrences)
   - Need to review each case

4. `pkg/merge/merge_test.go` (6 occurrences)
   - Need to review each case

5. `pkg/store/*_store_test.go` (multiple files)
   - Store-specific errors, need review

### Low Priority

Files testing third-party library errors or specific formatting (likely OK as-is):
- Validation test files (testing schema validation error messages)
- Store test files (testing Redis/Google APIs error messages)
- Auth test files (testing OIDC/OAuth error messages)

## Action Items

1. ✅ **DONE**: Fix `internal/exec/packer_test.go` to use `errors.Is()` for `ErrMissingComponent`
2. **TODO**: Fix download error checks (5 occurrences)
3. **TODO**: Fix OS error check in packer_test.go for `exec.ErrNotFound`
4. **TODO**: Document error checking guidelines in `CLAUDE.md`
5. **TODO**: Add linter rule to catch `assert.Contains(err.Error(), ...)` for our sentinel errors

## Statistics

- Total test files with error string matching: **71**
- High priority fixes needed: **5** (download errors + OS error)
- Status: **1 fixed** (packer component check), **4 remaining**

## References

- [Go errors package](https://pkg.go.dev/errors)
- [Go io/fs errors](https://pkg.go.dev/io/fs#pkg-variables)
- [Go os/exec errors](https://pkg.go.dev/os/exec#pkg-variables)
- [PR #1599 - Error Handling Implementation](https://github.com/cloudposse/atmos/pull/1599)
