# GitHub Issue: Refactor tests to use errors.Is() instead of string matching

**Title**: Refactor tests to use errors.Is() instead of string matching

**Labels**: refactor, tests, good first issue

---

## what

Replace brittle error string matching in tests with robust `errors.Is()` checks.

## why

- String matching is fragile and breaks when error messages change
- `errors.Is()` properly handles wrapped errors (the focus of PR #1599)
- Cross-platform OS errors exist in Go stdlib but we're not using them
- Aligns with the error handling patterns introduced in PR #1599

## details

**High Priority - Our Sentinel Errors (5 cases)**

1. `pkg/downloader/file_downloader_test.go` (4 occurrences)
   - Replace `assert.Contains(err.Error(), "failed to download file")`
   - With `assert.ErrorIs(err, errUtils.ErrDownloadFile)`

2. `pkg/config/imports_error_paths_test.go` (1 occurrence)
   - Replace string check for "failed to download remote config"
   - With `assert.ErrorIs(err, errUtils.ErrDownloadFile)`

3. `internal/exec/packer_test.go` (1 occurrence)
   - Replace `assert.Contains(err.Error(), "executable file not found")`
   - With `assert.ErrorIs(err, exec.ErrNotFound)` (cross-platform OS error)

**Medium Priority - Review Needed**

Files with many error string checks needing case-by-case review:
- `pkg/list/errors/types_test.go` (22 occurrences) - likely OK, testing error message formatting
- `internal/exec/copy_glob_error_paths_test.go` (15 occurrences)
- `internal/exec/stack_processor_process_stacks_helpers_test.go` (7 occurrences)
- `pkg/merge/merge_test.go` (6 occurrences)
- Various store test files

**Total**: 71 test files using error string matching

## guidelines

**Use `errors.Is()` for:**
- Our sentinel errors from `errors/errors.go`
- Standard library errors (`fs.ErrNotExist`, `exec.ErrNotFound`, `io.EOF`)
- Any wrapped error where you care about the type, not the message

**String matching is OK for:**
- Third-party library errors without sentinel errors
- Testing specific interpolated values in error messages
- Explicitly testing user-facing error message formatting

**Cross-platform OS errors:**
- `io/fs`: `fs.ErrNotExist`, `fs.ErrPermission`, `fs.ErrExist`, `fs.ErrInvalid`, `fs.ErrClosed`
- `os/exec`: `exec.ErrNotFound`

## references

- PR #1599 - Error handling implementation
- See `ERROR_CHECKING_AUDIT.md` for complete audit
