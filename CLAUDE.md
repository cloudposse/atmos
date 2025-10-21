# CLAUDE.md

Guidance for Claude Code when working with this repository.

## Project Overview

Atmos: Go CLI for cloud infrastructure orchestration via Terraform/Helmfile/Packer with stack-based config, templating, policy validation, vendoring, and terminal UI.

## Essential Commands

```bash
# Build & Test
make build                   # Build to ./build/atmos
make testacc                 # Run tests
make testacc-cover           # Tests with coverage
make lint                    # golangci-lint on changed files
```

## Architecture

- **`cmd/`** - CLI commands (one per file)
- **`internal/exec/`** - Business logic
- **`pkg/`** - config, stack, component, utils, validate, workflow, hooks, telemetry

**Stack Pipeline**: Load atmos.yaml → process imports/inheritance → apply overrides → render templates → generate config

**Templates**: Go templates + Gomplate with `atmos.Component()`, `terraform.output()`, store integration

## Architectural Patterns (MANDATORY)

### Registry Pattern (MANDATORY)
Use registry pattern for extensibility and plugin-like architecture. Existing implementations:
- **Command Registry**: `cmd/internal/registry.go` - All commands register via `CommandProvider` interface
- **Component Registry**: Component discovery and management
- **Store Registry**: `pkg/store/registry.go` - Multi-provider store implementations

**New commands MUST use command registry pattern.** See `docs/prd/command-registry-pattern.md`

### Interface-Driven Design (MANDATORY)
- Define interfaces for all major functionality
- Use dependency injection for testability
- Generate mocks with `go.uber.org/mock/mockgen`
- Avoid integration tests by mocking external dependencies

**Example:**
```go
// Define interface
type ComponentLoader interface {
    Load(path string) (*Component, error)
}

// Implement
type FileSystemLoader struct{}
func (f *FileSystemLoader) Load(path string) (*Component, error) { ... }

// Generate mock
//go:generate go run go.uber.org/mock/mockgen@latest -source=loader.go -destination=mock_loader_test.go
```

### Options Pattern (MANDATORY)
Avoid functions with many parameters. Use functional options pattern for configuration:

```go
// Define option type
type Option func(*Config)

// Provide option builders
func WithTimeout(d time.Duration) Option {
    return func(c *Config) { c.Timeout = d }
}

func WithRetries(n int) Option {
    return func(c *Config) { c.Retries = n }
}

// Constructor accepts variadic options
func NewClient(opts ...Option) *Client {
    cfg := &Config{/* defaults */}
    for _, opt := range opts {
        opt(cfg)
    }
    return &Client{config: cfg}
}

// Usage
client := NewClient(
    WithTimeout(30*time.Second),
    WithRetries(3),
)
```

**Benefits:** Avoids parameter drilling, provides defaults, extensible without breaking changes.

### Context Usage (MANDATORY)
Use `context.Context` for these specific purposes only:
- **Cancellation signals** - Propagate cancellation across API boundaries
- **Deadlines/timeouts** - Set operation time limits
- **Request-scoped values** - Trace IDs, request IDs (sparingly)

**DO NOT use context for:**
- Passing configuration (use Options pattern)
- Passing dependencies (use struct fields or DI)
- Avoiding proper function parameters

**Correct usage:**
```go
// IO operations, network calls, long-running tasks
func FetchData(ctx context.Context, url string) error {
    req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
    // ... respects cancellation
}

// Functions that coordinate multiple operations
func ProcessAll(ctx context.Context, items []Item) error {
    for _, item := range items {
        if err := ctx.Err(); err != nil {
            return err // Stop if cancelled
        }
        if err := processItem(ctx, item); err != nil {
            return err
        }
    }
}
```

**Context should be first parameter** in functions that accept it.

### Package Organization (MANDATORY)
- **Avoid utils package bloat** - Don't add new functions to `pkg/utils/`
- **Create purpose-built packages** - New functionality gets its own package in `pkg/`
- **Well-tested, focused packages** - Each package has clear responsibility
- **Examples**: `pkg/store/`, `pkg/git/`, `pkg/pro/`, `pkg/filesystem/`

**Anti-pattern:**
```go
// WRONG: Adding to utils
pkg/utils/new_feature.go
```

**Correct pattern:**
```go
// CORRECT: New focused package
pkg/newfeature/
  ├── newfeature.go
  ├── newfeature_test.go
  ├── interface.go
  └── mock_interface_test.go
```

## Code Patterns & Conventions

### Comment Style (MANDATORY)
All comments must end with periods (enforced by `godot` linter).

### Import Organization (MANDATORY)
Three groups separated by blank lines, sorted alphabetically:
1. Go stdlib
2. 3rd-party (NOT cloudposse/atmos)
3. Atmos packages

Maintain aliases: `cfg`, `log`, `u`, `errUtils`

### Performance Tracking (MANDATORY)
Add `defer perf.Track(atmosConfig, "pkg.FuncName")()` + blank line to all public functions. Use `nil` if no atmosConfig param.

### Configuration Loading
Precedence: CLI flags → ENV vars → config files → defaults (use Viper)

### Error Handling (MANDATORY)
- Wrap with static errors from `errors/errors.go`
- Combine: `errors.Join(errUtils.ErrFoo, err)`
- Add context: `fmt.Errorf("%w: msg", errUtils.ErrFoo)`
- Check: `errors.Is(err, target)`
- Never dynamic errors or string comparison

### Testing Strategy (MANDATORY)
- **Prefer unit tests with mocks** over integration tests
- Use interfaces + dependency injection for testability
- Generate mocks with `go.uber.org/mock/mockgen`
- Table-driven tests for comprehensive coverage
- Integration tests in `tests/` only when necessary
- Target >80% coverage

### Test Isolation (MANDATORY)
ALWAYS use `cmd.NewTestKit(t)` for cmd tests. Auto-cleans RootCmd state (flags, args). Required for any test touching RootCmd.

### Test Quality (MANDATORY)
Test behavior not implementation. No stub/tautological tests. Use DI for testability. Real scenarios only.

### Mock Generation (MANDATORY)
Use `go.uber.org/mock/mockgen` with `//go:generate` directives. Never manual mocks.

### Testing Production Code Paths (MANDATORY)
Tests must call actual production code, never duplicate logic.

### Test Skipping Conventions (MANDATORY)
Use `t.Skipf("reason")` with clear context. CLI tests auto-build temp binaries.

### CLI Command Structure
Embed examples from `cmd/markdown/*_usage.md` using `//go:embed`. Render with `utils.PrintfMarkdown()`.

### File Organization (MANDATORY)
Small focused files (<600 lines). One cmd/impl per file. Co-locate tests. Never `//revive:disable:file-length-limit`.

## Template Functions

`atmos.Component/Stack/Setting()`, `terraform.output/state()`, `store.get()`, `exec()`, `env()`. See `pkg/store/registry.go` for stores.

## Testing

**Preconditions**: Tests skip gracefully with helpers from `tests/test_preconditions.go`. See `docs/prd/testing-strategy.md`.

**Commands**: `make test-short` (quick), `make testacc` (all), `make testacc-cover` (coverage)

**Fixtures**: `tests/test-cases/` for integration tests

**Golden Snapshots**: NEVER modify `tests/test-cases/` or `tests/testdata/` unless instructed

## Common Development Tasks

### Adding New CLI Command

1. Create `cmd/[command]/` with CommandProvider interface
2. Add blank import to `cmd/root.go`
3. Implement in `internal/exec/mycommand.go`
4. Add tests, Docusaurus docs in `website/docs/cli/commands/`
5. Build website: `cd website && npm run build`

See `docs/developing-atmos-commands.md` and `docs/prd/command-registry-pattern.md`

### Documentation (MANDATORY)
All cmds/flags need Docusaurus docs in `website/docs/cli/commands/`. Use `<dl>` for args/flags. Build: `cd website && npm run build`

### PRD Documentation (MANDATORY)
All Product Requirement Documents (PRDs) MUST be placed in `docs/prd/`. Use kebab-case filenames. Examples: `command-registry-pattern.md`, `error-handling-strategy.md`, `testing-strategy.md`

### Pull Requests (MANDATORY)
Follow template (what/why/references). `minor`/`major` PRs need blog post in `website/blog/` with `<!--truncate-->`. Tag: `feature`/`enhancement`/`bugfix` (users) or `contributors` (internal). Use `no-release` for docs-only.

### PR Tools
Check status: `gh pr checks {pr} --repo cloudposse/atmos`
Reply to threads: Use `gh api graphql` with `addPullRequestReviewThreadReply`

### Bug Fixing (MANDATORY)
1. Write failing test
2. Fix iteratively
3. Verify with full test suite

## Critical Development Requirements

### Git (MANDATORY)
Don't commit: todos, research, scratch files. Do commit: code, tests, requested docs, schemas. Update `.gitignore` for patterns only.

### Test Coverage (MANDATORY)
80% minimum (CodeCov enforced). All features need tests. `make testacc-coverage` for reports.

### Environment Variables (MANDATORY)
Use `viper.BindEnv("ATMOS_VAR", "ATMOS_VAR", "FALLBACK")` - ATMOS_ prefix required.

### Logging vs UI (MANDATORY)
UI (prompts, status) → stderr. Data → stdout. Logging for system events only. Never use logging for UI.

### Schemas (MANDATORY)
Update all schemas in `pkg/datafetcher/schema/` when adding config options.

### Theme (MANDATORY)
Use colors from `pkg/ui/theme/colors.go`

### Templates (MANDATORY)
New configs support Go templating with `FuncMap()` from `internal/exec/template_funcs.go`

### Code Reuse (MANDATORY)
Search `internal/exec/` and `pkg/` before implementing. Extend, don't duplicate.

### Cross-Platform (MANDATORY)
Linux/macOS/Windows compatible. Use SDKs over binaries. Use `filepath.Join()`, not hardcoded separators.

### Multi-Provider Registry (MANDATORY)
Follow registry pattern for extensibility:
1. Define interface in dedicated package
2. Implement per provider (separate files)
3. Register implementations in registry
4. Generate mocks for testing

**Example**: `pkg/store/` has registry pattern with AWS SSM, Azure Key Vault, Google Secret Manager providers.

### Telemetry (MANDATORY)
Auto-enabled via `RootCmd.ExecuteC()`. Non-standard paths use `telemetry.CaptureCmd()`. Never capture user data.

## Development Environment

**Prerequisites**: Go 1.24+, golangci-lint, Make. See `.cursor/rules/atmos-rules.mdc`.

**Build**: CGO disabled, cross-platform, version via ldflags, output to `./build/`

### Compilation (MANDATORY)
ALWAYS compile after changes: `go build . && go test ./...`. Fix errors immediately.

### Pre-commit (MANDATORY)
NEVER use `--no-verify`. Run `make lint` before committing. Hooks run go-fumpt, golangci-lint, go mod tidy.
