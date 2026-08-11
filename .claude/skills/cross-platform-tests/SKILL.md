---
name: cross-platform-tests
description: Cross-platform test patterns for Atmos - avoiding Unix-only binaries and hardcoded path separators so tests pass on Windows CI. Invoke when writing or reviewing Go tests that spawn subprocesses or build file paths.
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Cross-Platform Tests

Atmos targets Linux/macOS/Windows. Windows uses backslash (`\`) as path separator, Unix uses forward slash (`/`) — hardcoded paths and Unix-only binaries fail on Windows CI.

## Subprocess helpers in tests

Instead of `exec.LookPath("false")` or other Unix-only binaries, use the test binary itself.

**Important:** if your package already has a `TestMain`, add the env-gate check **inside the existing `TestMain`** — do not add a second `TestMain` function (Go does not allow two in the same package).

```go
// In testmain_test.go — merge this check into the existing TestMain:
func TestMain(m *testing.M) {
    // If _ATMOS_TEST_EXIT_ONE is set, exit immediately with code 1.
    // This lets tests use the test binary itself as a cross-platform "exit 1" command.
    if os.Getenv("_ATMOS_TEST_EXIT_ONE") == "1" { os.Exit(1) }
    os.Exit(m.Run())
}
// NOTE: If your package already defines TestMain, insert the _ATMOS_TEST_EXIT_ONE
// check at the top of the existing function rather than copying the whole snippet.

// In the test itself:
exePath, _ := os.Executable()
info.Command = exePath
info.ComponentEnvList = []string{"_ATMOS_TEST_EXIT_ONE=1"}
```

## Path handling in tests

- **NEVER use forward slash concatenation** like `tempDir + "/components/terraform/vpc"`.
- **ALWAYS use `filepath.Join()`** with separate arguments: `filepath.Join(tempDir, "components", "terraform", "vpc")`.
- **NEVER use forward slashes in `filepath.Join()`** like `filepath.Join(dir, "a/b/c")` - use `filepath.Join(dir, "a", "b", "c")`.
- **NEVER hardcode Unix paths in expected values** like `assert.Equal(t, "/project/components/vpc", path)` - build expected paths with `filepath.Join()`.
- **For path suffix checks**, use `filepath.ToSlash()` to normalize: `strings.HasSuffix(filepath.ToSlash(path), "expected/suffix")`.
- **NEVER use bash/shell commands in tests** - use Go stdlib (`os`, `filepath`, `io`) for file operations.
