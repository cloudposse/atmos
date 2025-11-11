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

- **`cmd/`** - CLI commands (one per file, lightweight - flags and command registration only)
- **`pkg/`** - Reusable business logic packages (config, stack, component, devcontainer, container, store, git, auth, etc.)
- **`internal/exec/`** - Legacy business logic (being phased out - prefer pkg/)

**Stack Pipeline**: Load atmos.yaml → process imports/inheritance → apply overrides → render templates → generate config.

**Templates and YAML functions**: Go templates + Gomplate with `atmos.Component()`, `!terraform.state`, `!terraform.output`, store integration.

### Package Organization Philosophy (MANDATORY)

**Prefer `pkg/` over `internal/exec/` or new `internal/` packages:**
- **Create focused packages in `pkg/`** - Each new feature/domain gets its own package (e.g., `pkg/devcontainer`, `pkg/store`, `pkg/git`)
- **Commands are thin wrappers** - `cmd/` files only handle CLI concerns (flags, arguments, command registration)
- **Business logic lives in `pkg/`** - All domain logic, orchestration, and operations belong in reusable packages
- **Plugin-ready architecture** - Packages in `pkg/` can be imported and reused, supporting future plugin systems

**Anti-pattern:**
```go
// WRONG: Adding business logic to internal/exec
internal/exec/new_feature.go  // ❌ Avoid this

// WRONG: Adding business logic to cmd/
cmd/mycommand/mycommand.go    // ❌ Should only have CLI setup
func (cmd *MyCmd) Run() {
    // hundreds of lines of business logic  // ❌ Wrong place
}
```

**Correct pattern:**
```go
// CORRECT: Business logic in focused pkg/
pkg/myfeature/
  ├── myfeature.go           // ✅ Core business logic
  ├── myfeature_test.go      // ✅ Unit tests
  ├── operations.go          // ✅ Helper operations
  └── types.go               // ✅ Domain types

// CORRECT: Thin CLI wrapper in cmd/
cmd/mycommand/mycommand.go   // ✅ Just CLI setup
func (cmd *MyCmd) Run() {
    return myfeature.Execute(atmosConfig, opts)  // ✅ Delegates to pkg/
}
```

**Examples of well-structured packages:**
- `pkg/devcontainer/` - Devcontainer lifecycle management (List, Start, Stop, Attach, Exec, etc.)
- `pkg/store/` - Multi-provider secret store with registry pattern
- `pkg/git/` - Git operations and repository management
- `pkg/auth/` - Authentication and identity management
- `pkg/container/` - Container runtime abstraction (Docker/Podman)

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
Use functional options pattern for configuration instead of many parameters. Provides defaults and extensibility without breaking changes.

### Context Usage (MANDATORY)
Use `context.Context` ONLY for: cancellation signals, deadlines/timeouts, request-scoped values (trace IDs). NOT for configuration (use Options), dependencies (use struct fields/DI), or avoiding proper parameters. Context must be first parameter.

### I/O and UI Usage (MANDATORY)
Atmos separates I/O (streams) from UI (formatting) for clarity and testability.

**Two-layer architecture:**
- **I/O Layer** (`pkg/io/`) - Stream access (stdout/stderr/stdin), terminal capabilities, masking
- **UI Layer** (`pkg/ui/`) - Formatting (colors, styles, markdown rendering)

**Terminal as Text UI:**
The terminal window is effectively a text-based user interface (TextUI) for our CLI. Anything intended for user interaction—menus, prompts, animations, progress indicators—should be rendered to the terminal as UI output (stderr). Data intended for processing, piping, or machine consumption goes to the data channel (stdout).

**Output functions:**
```go
// Data channel (stdout) - for pipeable output
data.Write("result")                // Plain text to stdout
data.Writef("value: %s", val)       // Formatted text to stdout
data.Writeln("result")              // Plain text with newline to stdout
data.WriteJSON(structData)          // JSON to stdout
data.WriteYAML(structData)          // YAML to stdout

// UI channel (stderr) - for human messages
ui.Write("Loading configuration...")            // Plain text (no icon, no color, stderr)
ui.Writef("Processing %d items...", count)      // Formatted text (no icon, no color, stderr)
ui.Writeln("Done")                              // Plain text with newline (no icon, no color, stderr)
ui.Success("Deployment complete!")              // ✓ Deployment complete! (green, stderr)
ui.Error("Configuration failed")                // ✗ Configuration failed (red, stderr)
ui.Warning("Deprecated feature")                // ⚠ Deprecated feature (yellow, stderr)
ui.Info("Processing components...")             // ℹ Processing components... (cyan, stderr)

// Markdown rendering
ui.Markdown("# Help\n\nUsage...")               // Rendered to stdout (data)
ui.MarkdownMessage("**Error:** Invalid config") // Rendered to stderr (UI)
```

**Decision:** Pipeable data → `data.*`; Plain UI → `ui.Write*`; Status → `ui.Success/Error/Warning/Info`; Docs → `ui.Markdown*`

**Anti-patterns:** NO `fmt.Fprintf(os.Stdout/Stderr, ...)`, NO `fmt.Println()` - linter will block.

**Benefits:** Auto color degradation, TTY detection, secret masking (AWS keys/tokens), width adaptation, testable with mocks, linter-enforced.

**Force flags (screenshots):** `--force-tty`/`ATMOS_FORCE_TTY`, `--force-color`/`ATMOS_FORCE_COLOR`. See `pkg/io/example_test.go`.

### Secret Masking with Gitleaks
Uses Gitleaks (120+ patterns) for secret detection. Configure in `atmos.yaml` under `settings.terminal.mask.patterns`. Disable: `--mask=false`.

### Package Organization (MANDATORY)
See "Package Organization Philosophy" section above for the overall strategy. Key principles:

- **Avoid utils package bloat** - Don't add new functions to `pkg/utils/`
- **Avoid internal/exec** - Don't add new business logic to `internal/exec/` (legacy, being phased out)
- **Create purpose-built packages** - New functionality gets its own package in `pkg/`
- **Well-tested, focused packages** - Each package has clear responsibility
- **Examples**: `pkg/devcontainer/`, `pkg/store/`, `pkg/git/`, `pkg/pro/`, `pkg/container/`, `pkg/auth/`

**Anti-pattern:**
```go
// WRONG: Adding to utils
pkg/utils/new_feature.go

// WRONG: Adding to internal/exec
internal/exec/new_feature.go
```

**Correct pattern:**
```go
// CORRECT: New focused package in pkg/
pkg/newfeature/
  ├── newfeature.go           // Main business logic
  ├── newfeature_test.go      // Unit tests
  ├── operations.go           // Helper operations (if needed)
  ├── types.go                // Domain types (if needed)
  ├── interface.go            // Interface definitions (if needed)
  └── mock_interface_test.go  // Generated mocks (if needed)
```

## Code Patterns & Conventions

### Comment Style (MANDATORY)
All comments must end with periods (enforced by `godot` linter).

### Comment Preservation (MANDATORY)
NEVER delete existing comments. Preserve helpful comments explaining why/how/what/where, especially for credentials, complex logic. Update to match code changes. Only remove if: factually incorrect, code removed, obvious duplication, completed TODO.

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
- `errors.Join` for multiple errors, `fmt.Errorf("%w", err)` for chains
- Use `errors.Is()` for checking, NEVER string comparison
- Error builder: `errUtils.Build(err).WithHint().WithContext().WithExitCode().Err()`
- See `docs/errors.md` for complete guide

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
- **Test behavior, not implementation** - Verify inputs/outputs, not internal state
- **Never test stub functions** - Either implement the function or remove the test
- **Avoid tautological tests** - Don't test that hardcoded stubs return hardcoded values
- **Make code testable** - Use dependency injection to avoid hard dependencies on `os.Exit`, `CheckErrorPrintAndExit`, or external systems
- **No coverage theater** - Each test must validate real behavior, not inflate metrics
- **Remove always-skipped tests** - Either fix the underlying issue or delete the test
- **Table-driven tests need real scenarios** - Use production-like inputs, not contrived data
- **Use `errors.Is()` for error checking** - Use `assert.ErrorIs(err, ErrSentinel)` for our errors and stdlib errors (e.g., `fs.ErrNotExist`, `exec.ErrNotFound`). String matching is only OK for third-party errors or testing specific message formatting.

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

**Golden Snapshots (MANDATORY):**
NEVER manually edit - use `-regenerate-snapshots` flag. Environment-specific formatting (lipgloss, ANSI, width). See `tests/README.md`.

### Running/Regenerating Tests
```bash
go test ./pkg/config -run TestName  # Specific test
go test ./tests -regenerate-snapshots  # Regenerate snapshots
```
CRITICAL: No pipes when testing (breaks TTY detection). Fixtures in `tests/test-cases/`. See `tests/README.md`.

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

**Verify doc links:** Find file, check `slug` frontmatter, verify with existing links.

### PRD Documentation (MANDATORY)
All Product Requirement Documents (PRDs) MUST be placed in `docs/prd/`. Use kebab-case filenames. Examples: `command-registry-pattern.md`, `error-handling-strategy.md`, `testing-strategy.md`

### Pull Requests (MANDATORY)
Follow template (what/why/references).

**Blog Posts (CI):** `minor`/`major` PRs need blog in `website/blog/YYYY-MM-DD-*.mdx` with `<!--truncate-->`. Tag appropriately. Author=committer. Use `no-release` for docs-only.

### PR Tools
Check status: `gh pr checks {pr} --repo cloudposse/atmos`
Reply to threads: Use `gh api graphql` with `addPullRequestReviewThreadReply`

See `docs/developing-atmos-commands.md` and `docs/prd/command-registry-pattern.md` for complete guide.

### Documentation Requirements (MANDATORY)
All commands/flags need Docusaurus docs. Use `<dl>` for args/flags. Follow existing conventions. Location: `website/docs/cli/commands/<command>/<subcommand>.mdx`

### Website Build (MANDATORY)
ALWAYS build after doc changes: `cd website && npm run build`. Check for broken links, missing images, MDX errors.

### Pull Request Requirements (MANDATORY)
Follow template: what/why/references. `minor`/`major` need blog post (`website/blog/YYYY-MM-DD-*.mdx` with `<!--truncate-->`). CI enforces.

### Blog Post Guidelines (MANDATORY)
**User-facing:** Tags `feature`/`enhancement`/`bugfix` + technical area. **Contributors:** Tag `contributors` + technical area. Template has: slug, title, authors, tags, intro, `<!--truncate-->`, sections. Use `no-release` for docs-only.

### PR Tools (MANDATORY)
Check status: `gh pr checks {pr}`. Reply to threads: Use `gh api graphql` with `addPullRequestReviewThreadReply`. Get unresolved: query `reviewThreads`.

### Adding Template Function
1. Implement in `internal/exec/template_funcs.go`
2. Register in template function map
3. Add comprehensive tests
4. Document in website if user-facing

### Bug Fixing (MANDATORY)
1. Write failing test 2. Verify fails 3. Fix iteratively 4. Run full suite

### Extending Store Integration
1. Implement interface in `pkg/store/`
2. Add to store registry
3. Update configuration schema
4. Add integration tests with mocks

### Stack Processing Changes
1. Core logic in `pkg/stack/` and `internal/exec/stack_processor_utils.go`
2. Test with multiple inheritance scenarios
3. Validate template rendering still works
4. Update schema if configuration changes

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

### Lint Exclusions (MANDATORY)
- **ALWAYS ask for user approval before adding nolint comments** - do not add them automatically
- **Prefer refactoring over nolint** - only use nolint as last resort with explicit user permission
- **Exception for bubbletea models**: `//nolint:gocritic // bubbletea models must be passed by value` is acceptable (library convention)
- **Exception for intentional subprocess calls**: `//nolint:gosec // intentional subprocess call` is acceptable for container runtimes
- **NEVER add nolint for**:
  - gocognit (cognitive complexity) - refactor the function instead
  - cyclomatic complexity - refactor the function instead
  - magic numbers - extract constants instead
  - nestif - refactor nested logic instead
- **If you think nolint is needed, stop and ask the user first** - explain why refactoring isn't possible
