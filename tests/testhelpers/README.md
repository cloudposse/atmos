# Test Helpers

This package provides helper utilities for writing tests in the Atmos codebase.

## Cobra Command Helpers

Create mock Cobra commands for testing:

```go
import "github.com/cloudposse/atmos/tests/testhelpers"

// Create command with string flags
cmd := testhelpers.NewMockCommand("test", map[string]string{
    "stack":  "dev",
    "format": "yaml",
})

// Create command with boolean flags
cmd := testhelpers.NewMockCommandWithBool("test", map[string]bool{
    "verbose": true,
    "dry-run": false,
})

// Create command with mixed flags
cmd := testhelpers.NewMockCommandWithMixed(
    "test",
    map[string]string{"stack": "dev"},
    map[string]bool{"verbose": true},
)
```

## Filesystem Mock Builder

Fluent interface for building filesystem mocks:

```go
import (
    "github.com/golang/mock/gomock"
    "github.com/cloudposse/atmos/tests/testhelpers"
)

func TestMyFunction(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    // Build a mock filesystem with expected operations
    mockFS := testhelpers.NewMockFS(ctrl).
        WithTempDir("/tmp/test-12345").
        WithFile("/tmp/test-12345/config.yaml", []byte("test: data")).
        WithWriteFile("/tmp/test-12345/output.txt", []byte("result"), 0644).
        WithRemoveAll("/tmp/test-12345", nil).
        Build()

    // Use the mock in your test
    result := myFunctionUsingFS(mockFS)
    assert.NoError(t, result)
}
```

### Available Builder Methods

- `WithTempDir(path)` - Expect MkdirTemp to succeed
- `WithTempDirError(err)` - Expect MkdirTemp to fail
- `WithFile(path, content)` - Expect ReadFile to return content
- `WithFileError(path, err)` - Expect ReadFile to fail
- `WithWriteFile(path, content, perm)` - Expect WriteFile to succeed
- `WithWriteFileError(path, content, perm, err)` - Expect WriteFile to fail
- `WithMkdirAll(path, perm)` - Expect MkdirAll to succeed
- `WithMkdirAllError(path, perm, err)` - Expect MkdirAll to fail
- `WithRemoveAll(path, err)` - Expect RemoveAll with optional error
- `WithStat(path, info, err)` - Expect Stat to return FileInfo

## Usage Example

```go
func TestProcessOciImage_TempDirFailure(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    // Setup: temp dir creation fails
    mockFS := testhelpers.NewMockFS(ctrl).
        WithTempDirError(fmt.Errorf("permission denied")).
        Build()

    // Execute
    err := processOciImageWithFS(nil, "test/image", "/dest", mockFS)

    // Verify
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "permission denied")
}
```

## Best Practices

1. **Use builders for complex mocks** - They make test setup more readable
2. **Chain methods** - The fluent interface improves readability
3. **Call Build() last** - Always call Build() to get the final mock
4. **One builder per test** - Create a new builder for each test case
5. **Document expectations** - Add comments explaining why you expect each operation

## Adding New Helpers

When adding new helper functions:

1. Keep them focused and single-purpose
2. Use fluent interfaces where appropriate
3. Add comprehensive documentation
4. Include usage examples
5. Write tests for the helpers themselves
