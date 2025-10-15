# Lintroller

Custom Go static analysis linter for Atmos-specific rules.

## Overview

Lintroller is a custom linter that enforces Atmos coding conventions around test environment variable handling and temporary directory management. It prevents common mistakes when using Go's testing utilities and ensures tests follow best practices.

## Rules

### 1. `tsetenv-in-defer`

**Prevents `t.Setenv` calls inside `defer` or `t.Cleanup` blocks.**

`t.Setenv` automatically restores environment variables after the test completes, so calling it inside defer/cleanup blocks is redundant and won't work as expected (it will panic or have no effect).

**Bad:**
```go
func TestExample(t *testing.T) {
    defer func() {
        t.Setenv("FOO", "bar")  // ❌ Will panic - t.Setenv can't be called in defer
    }()
}
```

**Good:**
```go
func TestExample(t *testing.T) {
    t.Setenv("FOO", "bar")  // ✅ Automatically restored after test

    // OR if you need manual restoration:
    defer func() {
        os.Setenv("FOO", "original")  // ✅ Use os.Setenv in defer
    }()
}
```

### 2. `os-setenv-in-test`

**Prevents `os.Setenv` calls in test files (except in defer/cleanup blocks and benchmarks).**

In test files, `t.Setenv` should be used instead of `os.Setenv` because it provides automatic cleanup and prevents test pollution.

**Exceptions:**
- `os.Setenv` IS allowed inside `defer` blocks (for manual restoration)
- `os.Setenv` IS allowed inside `t.Cleanup` blocks (for manual restoration)
- `os.Setenv` IS allowed inside benchmark functions (since `b.Setenv` doesn't exist)

**Bad:**
```go
func TestExample(t *testing.T) {
    os.Setenv("PATH", "/test/path")  // ❌ Use t.Setenv instead
    // Test code...
}
```

**Good:**
```go
func TestExample(t *testing.T) {
    t.Setenv("PATH", "/test/path")  // ✅ Automatically restored
    // Test code...
}

func BenchmarkExample(b *testing.B) {
    os.Setenv("PATH", "/test/path")  // ✅ Allowed in benchmarks (b.Setenv doesn't exist)
    defer func() { os.Setenv("PATH", originalPath) }()
    // Benchmark code...
}
```

### 3. `os-mkdirtemp-in-test`

**Prevents `os.MkdirTemp` calls in test files (except in benchmarks).**

In test files, `t.TempDir()` should be used instead of `os.MkdirTemp` because it provides automatic cleanup and prevents resource leaks.

**Exceptions:**
- `os.MkdirTemp` IS allowed inside benchmark functions (since `b.TempDir()` doesn't exist)

**Bad:**
```go
func TestExample(t *testing.T) {
    tempDir, err := os.MkdirTemp("", "test-*")  // ❌ Use t.TempDir instead
    if err != nil {
        t.Fatal(err)
    }
    defer os.RemoveAll(tempDir)
    // Test code...
}
```

**Good:**
```go
func TestExample(t *testing.T) {
    tempDir := t.TempDir()  // ✅ Automatically cleaned up
    // Test code...
}

func BenchmarkExample(b *testing.B) {
    tempDir, _ := os.MkdirTemp("", "bench-*")  // ✅ Allowed in benchmarks (b.TempDir doesn't exist)
    defer os.RemoveAll(tempDir)
    // Benchmark code...
}
```

## Usage

### Standalone Binary

Build and run the lintroller binary directly:

```bash
cd tools/lintroller
go build -o .lintroller ./cmd/lintroller
./.lintroller ./...
```

### Via Makefile

The recommended way to run lintroller locally:

```bash
make lintroller
```

This is automatically run as part of `make lint`.

### Via golangci-lint (Local Development)

Build a custom golangci-lint binary with lintroller integrated:

```bash
# Build custom golangci-lint (only needed once, or when lintroller changes)
golangci-lint custom

# Run golangci-lint with lintroller included
./custom-gcl run
```

This provides unified linting with all golangci-lint features:
- Works with `//nolint:lintroller` comments
- Integrated with other linters
- Unified output format

### Pre-commit Hook

Lintroller runs automatically via pre-commit hooks. It will block commits if violations are found.

To bypass (not recommended):
```bash
git commit --no-verify
```

## Configuration

### Standalone/Makefile

The standalone binary runs all rules by default. No configuration needed.

### golangci-lint Integration

When using `golangci-lint custom`, you can configure lintroller in `.golangci.yml`:

```yaml
linters-settings:
  custom:
    lintroller:
      tsetenv-in-defer: true       # Enable/disable tsetenv-in-defer rule
      os-setenv-in-test: true      # Enable/disable os-setenv-in-test rule
      os-mkdirtemp-in-test: true   # Enable/disable os-mkdirtemp-in-test rule
```

All rules are enabled by default.

## Architecture

### Interface-Based Design

Lintroller uses an interface-based architecture for extensibility:

```go
type Rule interface {
    Name() string
    Doc() string
    Check(pass *analysis.Pass, file *ast.File) error
}
```

Each rule is implemented in its own file:
- `rule_tsetenv_in_defer.go` - t.Setenv in defer/cleanup detection
- `rule_os_setenv.go` - os.Setenv in test files detection
- `rule_os_mkdirtemp.go` - os.MkdirTemp in test files detection

### Dual-Mode Support

Lintroller supports both standalone and golangci-lint plugin modes:

1. **Standalone Mode** (`cmd/lintroller/main.go`):
   - Uses `golang.org/x/tools/go/analysis/singlechecker`
   - Direct binary execution
   - Used by Makefile and pre-commit hooks

2. **Plugin Mode** (`plugin.go`):
   - Implements `register.LinterPlugin` interface
   - Integrates with golangci-lint
   - Auto-registers via `init()` with `register.Plugin("lintroller", New)`

## Adding New Rules

To add a new linting rule:

1. **Create a new rule file** (e.g., `rule_example.go`):

```go
package linters

import (
    "go/ast"
    "golang.org/x/tools/go/analysis"
)

type ExampleRule struct{}

func (r *ExampleRule) Name() string {
    return "example-rule"
}

func (r *ExampleRule) Doc() string {
    return "Checks for example violations"
}

func (r *ExampleRule) Check(pass *analysis.Pass, file *ast.File) error {
    // AST inspection logic here
    ast.Inspect(file, func(n ast.Node) bool {
        // Check for violations and report
        return true
    })
    return nil
}
```

2. **Register the rule** in `plugin.go`:

Add to `standaloneRun` for standalone mode:
```go
rules := []Rule{
    &TSetenvInDeferRule{},
    &OsSetenvInTestRule{},
    &ExampleRule{},  // Add new rule
}
```

Add to Settings struct and plugin run method for golangci-lint mode.

3. **Add tests** in `testdata/src/a/` directory following the `analysistest` pattern.

4. **Update documentation** in this README.

## Files

- `plugin.go` - Main plugin interface and golangci-lint integration
- `rule.go` - Rule interface definition
- `rule_tsetenv_in_defer.go` - t.Setenv in defer rule
- `rule_os_setenv.go` - os.Setenv in test files rule
- `rule_os_mkdirtemp.go` - os.MkdirTemp in test files rule
- `cmd/lintroller/main.go` - Standalone CLI entry point
- `lintroller_test.go` - Test suite
- `testdata/` - Test fixtures
- `.custom-gcl.yml` - golangci-lint custom build configuration (in repo root)

## Dependencies

- `golang.org/x/tools/go/analysis` - Go static analysis framework
- `github.com/golangci/plugin-module-register` - golangci-lint plugin registration

## References

- [Go analysis package](https://pkg.go.dev/golang.org/x/tools/go/analysis)
- [golangci-lint Module Plugin System](https://golangci-lint.run/docs/plugins/module-plugins/)
- [Atmos Testing Guidelines](../../tests/README.md)
