# Lint Roller

Custom Go static analysis linter for Atmos-specific rules.

## Overview

Lint Roller is a custom linter that enforces Atmos coding conventions around test environment variable handling and temporary directory management. It prevents common mistakes when using Go's testing utilities and ensures tests follow best practices.

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

### 4. `os-chdir-in-test`

**Prevents `os.Chdir` calls in test files (except in benchmarks).**

In test files, `t.Chdir()` should be used instead of `os.Chdir` because it provides automatic cleanup and prevents directory pollution between tests.

**Exceptions:**
- `os.Chdir` IS allowed inside benchmark functions (since `b.Chdir()` doesn't exist)

**Bad:**
```go
func TestExample(t *testing.T) {
    originalDir, _ := os.Getwd()
    os.Chdir("/tmp/test")  // ❌ Use t.Chdir instead
    defer os.Chdir(originalDir)
    // Test code...
}
```

**Good:**
```go
func TestExample(t *testing.T) {
    t.Chdir("/tmp/test")  // ✅ Automatically restored
    // Test code...
}

func BenchmarkExample(b *testing.B) {
    originalDir, _ := os.Getwd()
    os.Chdir("/tmp/bench")  // ✅ Allowed in benchmarks (b.Chdir doesn't exist)
    defer os.Chdir(originalDir)
    // Benchmark code...
}
```

### 5. `test-no-assertions`

**Prevents test functions that contain only `t.Log()` calls with no assertions, or that unconditionally skip.**

Tests without assertions or that always skip provide no coverage value and won't catch regressions. They are essentially documentation disguised as tests and should either be removed or have actual assertions added.

**Bad - Documentation Only:**
```go
func TestDocumentationOnly(t *testing.T) {
    // ❌ Only logging, no assertions
    t.Log("This test documents the expected behavior")
    t.Log("When a user does X, the system should do Y")
    t.Logf("Some value: %s", "test")
}
```

**Bad - Unconditional Skip:**
```go
func TestAlwaysSkips(t *testing.T) {
    // ❌ Always skips, provides no coverage
    t.Skipf("This test is not implemented")
}

func TestAlwaysSkipsWithTrue(t *testing.T) {
    // ❌ if (true) is unconditional
    if true {
        t.Skipf("This also always skips")
    }
}
```

**Good:**
```go
func TestActualBehavior(t *testing.T) {
    // ✅ Has assertions to verify behavior
    t.Log("Testing the actual behavior")
    result := DoSomething()
    if result != expected {
        t.Errorf("Expected %v, got %v", expected, result)
    }
}

func TestWithError(t *testing.T) {
    // ✅ Uses t.Error/t.Fatal for assertions
    t.Log("Checking condition")
    if condition {
        t.Error("Expected condition to be false")
    }
}

func TestConditionalSkip(t *testing.T) {
    // ✅ Conditionally skips based on runtime condition
    if runtime.GOOS == "windows" {
        t.Skipf("Skipping: not supported on Windows")
    }
    // Test continues if not skipped
    result := DoSomething()
    if result != expected {
        t.Error("Test failed")
    }
}
```

## Usage

### Standalone Binary

Build and run the Lint Roller binary directly:

```bash
cd tools/lintroller
go build -o .lintroller ./cmd/lintroller
./.lintroller ./...
```

### Via Makefile

The recommended way to run Lint Roller locally:

```bash
make lintroller
```

This is automatically run as part of `make lint`.

### Via golangci-lint (Local Development)

Build a custom golangci-lint binary with Lint Roller integrated:

```bash
# Build custom golangci-lint (only needed once, or when Lint Roller changes)
golangci-lint custom

# Run golangci-lint with Lint Roller included
./custom-gcl run
```

This provides unified linting with all golangci-lint features:
- Works with `//nolint:lintroller` comments
- Integrated with other linters
- Unified output format

### Pre-commit Hook

Lint Roller runs automatically via pre-commit hooks. It will block commits if violations are found.

To bypass (not recommended):
```bash
git commit --no-verify
```

## Configuration

### Standalone/Makefile

The standalone binary runs all rules by default. No configuration needed.

### golangci-lint Integration

Lintroller integrates with golangci-lint as a **module plugin**. This allows it to run alongside all standard golangci-lint linters with unified output, SARIF support for GitHub Advanced Security, and inline PR annotations.

#### How Module Plugins Work

Module plugins in golangci-lint v2 work differently than traditional Go plugins:

1. **Single Custom Binary**: All plugins are compiled into one custom golangci-lint binary
2. **Not Dynamic Loading**: Plugins are not loaded at runtime as `.so` files
3. **Build-Time Integration**: `golangci-lint custom` compiles everything together

**Build Process:**
```bash
# Step 1: golangci-lint reads .custom-gcl.yml and finds plugin definitions
# Step 2: Clones golangci-lint repo and adds blank imports for each plugin
# Step 3: Builds single binary containing ALL standard linters + ALL plugins
# Step 4: Outputs ./custom-gcl binary

golangci-lint custom  # Reads .custom-gcl.yml
./custom-gcl run      # Runs with lintroller + all standard linters
```

#### Configuration Files

**`.custom-gcl.yml`** - Defines which plugins to compile into the binary:
```yaml
version: v2.5.0
plugins:
  - module: 'github.com/cloudposse/atmos/tools/lintroller'
    import: 'github.com/cloudposse/atmos/tools/lintroller'
    path: './tools/lintroller'
  # Add more plugins here - all compile into the same custom-gcl binary
```

**`.golangci.yml`** - Enables and configures the linters:
```yaml
linters:
  enable:
    - lintroller  # Enable the custom linter
    - bodyclose   # Standard linters also available
  settings:
    custom:
      lintroller:
        type: "module"  # Required for module plugins
        description: "Atmos-specific test rules"
        settings:
          tsetenv-in-defer: true       # Enable/disable tsetenv-in-defer rule
          os-setenv-in-test: true      # Enable/disable os-setenv-in-test rule
          os-mkdirtemp-in-test: true   # Enable/disable os-mkdirtemp-in-test rule
          os-chdir-in-test: true       # Enable/disable os-chdir-in-test rule
          test-no-assertions: true     # Enable/disable test-no-assertions rule
```

All rules are enabled by default.

#### Multiple Custom Linters

To add another custom linter:

1. **Add plugin to `.custom-gcl.yml`**:
   ```yaml
   plugins:
     - module: 'github.com/cloudposse/atmos/tools/lintroller'
       path: './tools/lintroller'
     - module: 'github.com/cloudposse/atmos/tools/another-linter'
       path: './tools/another-linter'
   ```

2. **Enable in `.golangci.yml`**:
   ```yaml
   linters:
     enable:
       - lintroller
       - another-linter
   ```

3. **Configure in `.golangci.yml`**:
   ```yaml
   settings:
     custom:
       another-linter:
         type: "module"
         settings:
           some-rule: true
   ```

4. **Rebuild**: `golangci-lint custom` creates one binary with both plugins

#### CI/CD Integration

The custom binary integrates with GitHub Actions via `golangci-lint-action`:

```yaml
- name: Install golangci-lint v2
  run: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.5.0

- name: Build custom golangci-lint with plugins
  run: |
    golangci-lint custom
    sudo cp ./custom-gcl /usr/local/bin/golangci-lint

- name: Run golangci-lint with plugins
  uses: golangci/golangci-lint-action@v8
  with:
    install-mode: none  # Use our custom binary instead of action's binary
    args: --out-format=sarif:golangci-lint.sarif
```

Benefits:
- ✅ Lintroller findings appear in GitHub Security tab via SARIF
- ✅ Inline PR annotations for violations
- ✅ Unified linting output (standard + custom linters)
- ✅ Supports `//nolint:lintroller` comments

## Architecture

### Interface-Based Design

Lint Roller uses an interface-based architecture for extensibility:

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
- `rule_os_chdir.go` - os.Chdir in test files detection
- `rule_test_no_assertions.go` - Test functions with only logging detection.

### Dual-Mode Support

Lint Roller supports both standalone and golangci-lint plugin modes:

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
- `rule_os_chdir.go` - os.Chdir in test files rule
- `rule_test_no_assertions.go` - Test functions without assertions rule
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
