# Go Test JSON Coverage Bug

## Issue Description

We've discovered a critical issue where `go test -json` with coverage reports tests as passing when they actually fail with `require.NoError()` assertions.

## Symptoms

1. Vanilla `go test` (without JSON) correctly reports test failure:
   ```
   === RUN   TestExecuteVendorPull
       vendor_utils_test.go:124: 
           Error Trace: vendor_utils_test.go:124
           Error:       Received unexpected error:
                        failed to vendor components: 1
           Test:        TestExecuteVendorPull
   --- FAIL: TestExecuteVendorPull (9.57s)
   ```

2. But `go test -json` with coverage reports it as passing:
   ```json
   {"Action":"pass","Package":"github.com/cloudposse/atmos/internal/exec","Test":"TestExecuteVendorPull","Elapsed":15.01}
   ```

## Root Cause

The test uses `require.NoError(t, err)` which should immediately fail the test. However, when run with coverage flags like `-coverpkg=github.com/cloudposse/atmos/...`, the test somehow continues execution and eventually gets marked as passing.

## Impact

- Test failures are not detected by gotcha when coverage is enabled
- CI passes even when tests are actually failing
- This is NOT a gotcha bug - it's a Go test runner issue

## Affected Test Pattern

```go
func TestExecuteVendorPull(t *testing.T) {
    // ... setup code ...
    
    err := ExecuteVendorPullCommand(&cmd, []string{})
    require.NoError(t, err)  // This fails but test continues
    
    // More test code that shouldn't execute but does
    files := []string{...}
    success, file := verifyFileExists(t, files)
    // ...
}
```

## Workaround

Run tests twice in CI:
1. Once with vanilla `go test` to catch failures
2. Once with `go test -json` and coverage for reporting

This is what we've implemented by adding the "Run vanilla go test (debug baseline)" step in the GitHub workflow.

## Go Version

This issue was observed with Go 1.25 in GitHub Actions CI environment.

## References

- GitHub Actions Run: 17880620987
- Affected test: `internal/exec/vendor_utils_test.go:124`