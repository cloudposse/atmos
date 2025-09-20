# TestExecuteVendorPull Flaky Test Issue

## Issue Description

The `TestExecuteVendorPull` test is flaky and environment-dependent. It attempts to pull OCI images from GitHub Container Registry (ghcr.io) which requires authentication.

## Symptoms

1. Vanilla `go test` (without JSON) reports test failure:
   ```
   === RUN   TestExecuteVendorPull
   2025/09/20 16:23:15 ERRO Failed to pull OCI image image=ghcr.io/cloudposse/atmos/tests/fixtures/components/terraform/mock:v0 
   error="GET https://ghcr.io/token?scope=repository%3Acloudposse%2Fatmos%2Ftests%2Ffixtures%2Fcomponents%2Fterraform%2Fmock%3Apull&service=ghcr.io: UNAUTHORIZED: authentication required"
       vendor_utils_test.go:124: 
           Error Trace: vendor_utils_test.go:124
           Error:       Received unexpected error:
                        failed to vendor components: 1
           Test:        TestExecuteVendorPull
   --- FAIL: TestExecuteVendorPull (9.57s)
   ```

2. Sometimes `go test -json` with coverage reports it as passing:
   ```json
   {"Action":"pass","Package":"github.com/cloudposse/atmos/internal/exec","Test":"TestExecuteVendorPull","Elapsed":15.01}
   ```

## Root Cause

The test tries to vendor a component from `ghcr.io/cloudposse/atmos/tests/fixtures/components/terraform/mock:v0` which requires GitHub authentication. When `GITHUB_TOKEN` is not available or the authentication fails, the vendoring fails with "failed to vendor components: 1".

The test has multiple issues:
1. It depends on external network services (GitHub Container Registry)
2. It requires authentication credentials (`GITHUB_TOKEN`)
3. It has redundant error handling (`require.NoError` followed by `if err != nil`)
4. It's not a proper unit test - it's an integration test

## Why It Sometimes Passes

The inconsistent behavior might be due to:
- Different environment variables between test runs
- Network conditions and timeouts
- GitHub API rate limiting
- Caching of OCI images
- Test execution order affecting shared state

## Impact

- Test fails locally without proper GitHub authentication
- Test behavior is inconsistent between environments
- Test may pass or fail depending on network conditions and GitHub API availability
- This is NOT a gotcha bug - it's a flaky test that depends on external services

## Affected Test Pattern

```go
func TestExecuteVendorPull(t *testing.T) {
    // ... setup code ...
    
    err := ExecuteVendorPullCommand(&cmd, []string{})
    require.NoError(t, err)  // This should stop execution on error
    if err != nil {  // This is redundant - will never execute after require.NoError
        t.Errorf("Failed to execute vendor pull command: %v", err)
    }
    
    // More test code...
}
```

## Solution

The test should be refactored to:
1. Mock the OCI registry calls instead of hitting real endpoints
2. Use local test fixtures instead of pulling from ghcr.io
3. Skip the test if GITHUB_TOKEN is not available using `t.Skip()`
4. Or move this to an integration test suite that's run separately
5. Remove the redundant `if err != nil` check after `require.NoError`

## Workaround

The "Run vanilla go test (debug baseline)" step helps identify when this test fails due to authentication issues.

## Go Version

This issue was observed with Go 1.25 in GitHub Actions CI environment.

## References

- GitHub Actions Run: 17880620987
- Affected test: `internal/exec/vendor_utils_test.go:124`
- OCI image causing issue: `ghcr.io/cloudposse/atmos/tests/fixtures/components/terraform/mock:v0`