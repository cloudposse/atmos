# CLAUDE.md

Guidance for Claude Code when working with this repository.

## Project Overview

Atmos: Go CLI for cloud infrastructure orchestration via Terraform/Helmfile/Packer with stack-based config, templating, policy validation, vendoring, and terminal UI.

## Essential Commands

```bash
make build                   # Build to ./build/atmos
make testacc                 # Run tests
make testacc-cover           # Tests with coverage
make lint                    # golangci-lint on changed files
```

## Working with Atmos Agents (RECOMMENDED)

Atmos has **specialized domain experts** in `.claude/agents/` for focused subsystems. **Use agents instead of inline work** for their areas of expertise.

**Available Agents:**
- **`@agent-developer`** - Creating/maintaining agents, agent architecture
- **`@tui-expert`** - Terminal UI, theme system, output formatting
- **`@atmos-errors`** - Error handling patterns, error builder usage
- **`@flag-handler`** - CLI commands, flag parsing, CommandProvider pattern

**When to delegate:**
- TUI/theme changes → `@tui-expert`
- New CLI commands → `@flag-handler`
- Error handling refactoring → `@atmos-errors`
- Creating new agents → `@agent-developer`

**Benefits:** Agents are domain experts with deep knowledge of patterns, PRDs, and subsystem architecture. They ensure consistency and best practices.

See `.claude/agents/README.md` for full list and `docs/prd/claude-agent-architecture.md` for architecture.

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
type ComponentLoader interface {
    Load(path string) (*Component, error)
}
//go:generate go run go.uber.org/mock/mockgen@latest -source=loader.go -destination=mock_loader_test.go
```

### Service-Oriented Architecture (MANDATORY)

For complex domains with multiple operations and concerns (like devcontainer, store, auth), use Service-Oriented Architecture with provider interfaces:

**Pattern:**
1. **One Service struct** per domain (not one-off structs per operation)
2. **Provider interfaces** for classes of problems (ConfigProvider, RuntimeProvider, UIProvider)
3. **Default implementations** wrapping existing code
4. **Mock implementations** for testing
5. **Dependency injection** for testability

**Example (devcontainer domain):**
```go
// Service coordinates all devcontainer operations
type Service struct {
    config   ConfigProvider    // ALL config operations
    runtime  RuntimeProvider   // ALL runtime operations
    ui       UIProvider        // ALL UI operations
}

// Provider interfaces for classes of problems
type ConfigProvider interface {
    LoadAtmosConfig() (*schema.AtmosConfiguration, error)
    ListDevcontainers(config *schema.AtmosConfiguration) ([]string, error)
}

type RuntimeProvider interface {
    Start(ctx context.Context, name string, opts StartOptions) error
    Stop(ctx context.Context, name string, timeout int) error
    Attach(ctx context.Context, name string, opts AttachOptions) error
}

type UIProvider interface {
    IsInteractive() bool
    Prompt(message string, options []string) (string, error)
}

// Commands use service, not individual helpers
service := NewService()
service.Start(ctx, name, opts)
```

**Benefits:**
- Reusable across all commands in domain
- Clear separation of concerns (config/runtime/UI)
- Easy to test with mock providers
- Extensible (new provider = new implementation)
- Avoids one-off struct proliferation

**See:** `docs/prd/devcontainer-service-architecture.md` for complete implementation guide.

**Existing examples:** `pkg/store/` (multi-provider store), `pkg/auth/` (authentication providers), `pkg/container/` (Docker/Podman abstraction)

### Options Pattern (MANDATORY)
Use functional options pattern for configuration instead of many parameters. Provides defaults and extensibility without breaking changes.

**Example:**
```go
type Option func(*Config)
func WithTimeout(d time.Duration) Option { return func(c *Config) { c.Timeout = d } }
func NewClient(opts ...Option) *Client {
    cfg := &Config{/* defaults */}
    for _, opt := range opts { opt(cfg) }
    return &Client{config: cfg}
}
// Usage: client := NewClient(WithTimeout(30*time.Second), WithRetries(3))
```

### Context Usage (MANDATORY)
Use `context.Context` for:
- **Cancellation signals** - Propagate cancellation across API boundaries
- **Deadlines/timeouts** - Set operation time limits
- **Request-scoped values** - Trace IDs, request IDs (sparingly)

**DO NOT use context for:** Configuration (use Options pattern), dependencies (use struct fields/DI), or avoiding proper function parameters.

**Context should be first parameter** in functions that accept it.

### I/O and UI Usage (MANDATORY)
Atmos separates I/O (streams) from UI (formatting) for clarity and testability.

**Two-layer architecture:**
- **I/O Layer** (`pkg/io/`) - Stream access (stdout/stderr/stdin), terminal capabilities, masking
- **UI Layer** (`pkg/ui/`) - Formatting (colors, styles, markdown rendering)

The terminal is a text-based UI (TextUI). User interaction (menus, prompts, animations, progress) → stderr. Data for processing/piping → stdout.

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

**Decision tree:** Pipeable data → `data.*`, Plain UI → `ui.Write*()`, Status messages → `ui.Success/Error/Warning/Info()`, Formatted docs → `ui.Markdown*()`

**Anti-patterns:** Never use `fmt.Fprintf(os.Stdout/Stderr, ...)`, `fmt.Println()`, or direct stream access. Use `data.*` or `ui.*` instead.

**Why this matters:**
- **Auto-degradation**: Color (TrueColor→256→16→None), width adaptation, TTY/CI detection, markdown rendering, icon support
- **Security**: Automatic secret masking (AWS keys, tokens), format-aware, pattern-based
- **DX**: No capability checking, no manual masking, no stream selection, testable, enforced by linter
- **UX**: Respects preferences (--no-color, NO_COLOR), pipeline friendly, accessible, consistent

**Force Flags (screenshot generation):**
- `--force-tty` / `ATMOS_FORCE_TTY=true` - Force TTY mode (width=120, height=40)
- `--force-color` / `ATMOS_FORCE_COLOR=true` - Force TrueColor even for non-TTY

See `pkg/io/example_test.go` for examples.

### Secret Masking with Gitleaks

Atmos uses Gitleaks pattern library (120+ patterns):

```yaml
settings:
  terminal:
    mask:
      patterns:
        library: "gitleaks"  # default
        categories:
          aws: true / github: true / generic: false
```

Disable: `atmos terraform plan --mask=false`

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
**NEVER delete existing comments without a very strong reason.** Comments document why/how/what/where.

**Guidelines:** Preserve helpful comments, update to match code, refactor for clarity, add context when modifying.

**Acceptable removals:** Factually incorrect, code removed, duplicates obvious code, outdated TODO completed.

**Example:**
```go
// CORRECT: Preserving and updating helpful documentation
-// LoadAWSConfig looks for credentials in the following order:
+// LoadAWSConfigWithAuth looks for credentials in the following order:
+// When authContext is provided, uses Atmos-managed credentials.
+// Otherwise, falls back to standard AWS SDK resolution:
 //   1. Environment variables (AWS_ACCESS_KEY_ID, etc.)
 //   2. Shared credentials file (~/.aws/credentials)
 //   3. EC2 Instance Metadata Service (IMDS)
```

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

### Flag Handling (MANDATORY)

**CRITICAL: Unified flag parsing infrastructure is FULLY IMPLEMENTED in `pkg/flags/`.**

**Current Architecture:**
- ✅ `pkg/flags/` package is fully implemented with robust flag parsing infrastructure
- ✅ Commands MUST use `flags.NewStandardParser()` for command-specific flags
- ✅ **NEVER call `viper.BindEnv()` or `viper.BindPFlag()` directly** - Forbidigo enforces this
- ✅ Flag-handler agent provides guidance on correct usage patterns.

**Correct Pattern:**
```go
// In cmd/mycommand/mycommand.go
var (
    myParser *flags.StandardParser
)

func init() {
    // Create parser with command-specific flags
    myParser = flags.NewStandardParser(
        flags.WithBoolFlag("check", "c", false, "Enable checking"),
        flags.WithStringFlag("format", "f", "", "Output format"),
        flags.WithEnvVars("check", "ATMOS_MY_CHECK"),
        flags.WithEnvVars("format", "ATMOS_MY_FORMAT"),
    )

    // Register flags with Cobra command
    myParser.RegisterFlags(myCmd)

    // Bind to Viper for precedence handling (flags > env > config > defaults)
    if err := myParser.BindToViper(viper.GetViper()); err != nil {
        panic(err)
    }
}
```

**In RunE:**
```go
RunE: func(cmd *cobra.Command, args []string) error {
    // Parse flags using Viper (respects precedence)
    v := viper.GetViper()
    if err := myParser.BindFlagsToViper(cmd, v); err != nil {
        return err
    }

    // Access values via Viper
    check := v.GetBool("check")
    format := v.GetString("format")

    return exec.MyCommand(check, format)
}
```

**Linter Protection (Forbidigo):**
- ❌ `viper.BindEnv()` is BANNED outside `pkg/flags/`
- ❌ `viper.BindPFlag()` is BANNED outside `pkg/flags/`
- ✅ Consult flag-handler agent for all flag-related work
- ✅ See `cmd/version/version.go` for reference implementation.

### Error Handling (MANDATORY)

#### PRIMARY PATTERN: ErrorBuilder with Sentinel Errors

ALL user-facing errors MUST use ErrorBuilder with sentinel errors from `errors/errors.go`:

```go
// PREFERRED: Sentinel with underlying cause (preserves actual error message)
err := runtime.Start(ctx, containerID) // returns "container already running"
return errUtils.Build(errUtils.ErrContainerRuntimeOperation).
    WithCause(err).  // Preserves Docker/Podman error message
    WithExplanation("Failed to start container").
    WithHint("Check Docker is running").
    WithContext("container", containerName).
    Err()

// ALSO VALID: Sentinel as base (when no underlying error to preserve)
err := errUtils.Build(errUtils.ErrContainerRuntimeOperation).
    WithExplanation("Container runtime not configured").
    WithHint("Check atmos.yaml configuration").
    Err()
```

**Testing (MANDATORY):**
```go
// ✅ CORRECT: Always use errors.Is()
assert.ErrorIs(t, err, errUtils.ErrContainerRuntimeOperation)

// ❌ WRONG: Never string matching
assert.Contains(t, err.Error(), "...")  // FORBIDDEN
```

**Rules:**
- ✅ ALWAYS use `errors.Is()` for checking, NEVER string comparison.
- ✅ ALWAYS use `assert.ErrorIs()` in tests, NEVER `assert.Contains(err.Error(), ...)`.
- ✅ ALWAYS use sentinel errors from `errors/errors.go`.
- ❌ NEVER create dynamic errors: `errors.New("msg")`.
- ❌ NEVER string matching: `err.Error() == "..."` or `strings.Contains(err.Error(), ...)`.

**Legacy patterns (internal/non-user-facing only):**
- `errors.Join` for multiple errors, `fmt.Errorf("%w", err)` for chains.
- `fmt.Errorf` with single `%w`: Error chain (sequential call stack)
- `errors.Join`: Flat list (independent errors, parallel operations)
- `fmt.Errorf` with multiple `%w`: Like `errors.Join` but with format string (Go 1.20+)

**Additional utilities:**
```go
// Exit codes
err := errUtils.WithExitCode(err, 2)
exitCode := errUtils.GetExitCode(err) // 0 (nil), custom, exec.ExitError, or 1 (default)

// Formatting
formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())

// Sentry
errUtils.InitializeSentry(&atmosConfig.Errors.Sentry)
defer errUtils.CloseSentry()
errUtils.CaptureErrorWithContext(err, map[string]string{"component": "vpc"})
```

See "docs/errors.md" for complete ErrorBuilder API guide.

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
- Test behavior, not implementation
- Never test stub functions - implement or remove
- Avoid tautological tests - don't test hardcoded stubs return hardcoded values
- Make code testable - use DI to avoid `os.Exit`, `CheckErrorPrintAndExit`, external systems
- No coverage theater - validate real behavior
- Remove always-skipped tests - fix or delete
- Table-driven tests need real scenarios
- Use `assert.ErrorIs(err, ErrSentinel)` for our/stdlib errors. String matching OK for third-party errors.

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
- **NEVER manually edit golden snapshot files** - Always use `-regenerate-snapshots` flag
- Snapshots capture exact output including invisible formatting (lipgloss padding, ANSI codes, trailing whitespace)
- Different environments produce different output

**Regeneration:**
```bash
go test ./pkg/config -run TestName  # Specific test
go test ./tests -run 'TestCLICommands/test_name' -regenerate-snapshots
go test ./tests -run 'TestCLICommands/test_name' -v  # Verify
git diff tests/snapshots/  # Review
```

**Why manual editing fails:** Lipgloss padding varies, trailing whitespace significant, ANSI codes differ, Unicode rendering affects columns.

**CI failures:** Regenerate locally, verify, commit. If still fails: environment mismatch.

**CRITICAL**: Never use pipe redirection (`2>&1`, `| head`, `| tail`) when running tests. Piping breaks TTY detection → ASCII fallback → wrong snapshots.

### Test Data
Use fixtures in `tests/test-cases/`: `atmos.yaml`, `stacks/`, `components/`.

**NEVER modify `tests/test-cases/` or `tests/testdata/`** unless explicitly instructed. Golden snapshots are sensitive to minor changes.

See `tests/README.md` for details.

## Common Development Tasks

### Adding New CLI Command

1. Create `cmd/[command]/` with CommandProvider interface
2. Add blank import to `cmd/root.go`: `_ "github.com/cloudposse/atmos/cmd/mycommand"`
3. Implement in `internal/exec/mycommand.go`
4. Add tests in `cmd/mycommand/mycommand_test.go`
5. Create Docusaurus docs in `website/docs/cli/commands/<command>/<subcommand>.mdx`
6. Build website: `cd website && npm run build`

See `docs/developing-atmos-commands.md` and `docs/prd/command-registry-pattern.md`

### Documentation (MANDATORY)
All cmds/flags need Docusaurus docs in `website/docs/cli/commands/`. Use `<dl>` for args/flags. Build: `cd website && npm run build`

**Verifying Links:** Find doc file (`find website/docs/cli/commands -name "*keyword*"`), check slug in frontmatter (`head -10 <file> | grep slug`), verify existing links (`grep -r "<url>" website/docs/`).

**Common mistakes:** Using command name vs. filename, not checking slug frontmatter, guessing URLs.

### Documentation Requirements (MANDATORY)
Use `<dl>` for arguments/flags. Follow Docusaurus conventions: frontmatter, purpose note, screengrab, usage/examples/arguments/flags sections. File location: `website/docs/cli/commands/<command>/<subcommand>.mdx`

### Website Build (MANDATORY)
ALWAYS build after doc changes: `cd website && npm run build`. Verify: no broken links, missing images, MDX component rendering.

### Regenerating Screengrabs (IMPORTANT)
**When:** After modifying CLI behavior/help/output, adding commands. NOT for doc-only changes.

**How (Linux/CI only):**
1. GitHub Actions: `gh workflow run screengrabs.yaml` (creates PR)
2. Local Linux: `cd demo/screengrabs && make all`
3. Docker (macOS): `make -C demo/screengrabs docker-all`

**Notes:** Captures exact output, ANSI→HTML, `script` syntax differs BSD/GNU, regenerate all together, no pipe indirection.

### PRD Documentation (MANDATORY)
All PRDs in `docs/prd/`. Use kebab-case: `command-registry-pattern.md`, `error-handling-strategy.md`, `testing-strategy.md`

### Pull Requests (MANDATORY)
Follow template (what/why/references).

**Blog Posts (CI Enforced):**
- PRs labeled `minor`/`major` MUST include blog post: `website/blog/YYYY-MM-DD-feature-name.mdx`
- Use `.mdx` with YAML front matter, `<!--truncate-->` after intro
- **ONLY use existing tags** - check `website/blog/*.mdx` for valid tags before writing
- Author: committer's GitHub username, add to `website/blog/authors.yml`

**Blog Template:**
```markdown
---
slug: descriptive-slug
title: "Clear Title"
authors: [username]
tags: [primary-tag, secondary-tag]
---
Brief intro.
<!--truncate-->
## What Changed / Why This Matters / How to Use It / Get Involved
```

**Existing Tags (use only these):**
- Primary: `feature`, `enhancement`, `bugfix`
- Secondary: `dx`, `security`, `documentation`, `core`, `breaking-change`

**Finding valid tags:**
```bash
grep -h "^  - " website/blog/*.mdx | sort | uniq -c | sort -rn
```

Use `no-release` label for docs-only changes.

### PR Tools
```bash
gh pr checks {pr} --repo cloudposse/atmos
gh api repos/{owner/repo}/check-runs/{id}/annotations
gh api repos/{owner/repo}/code-scanning/alerts
```

### Responding to PR Threads (MANDATORY)
ALWAYS reply to specific threads (not new comments). Use GraphQL API: `gh api graphql -f query='mutation { addPullRequestReviewThreadReply(...) }'`

### Bug Fixing Workflow (MANDATORY)
1. Write failing test reproducing the bug
2. Run test to confirm it fails
3. Fix iteratively until test passes
4. Verify full test suite

**Example:** Test describes expected behavior, not that it's a bug fix.

### Other Tasks
**Template Function:** Implement in `internal/exec/template_funcs.go`, register, test, document.

**Store Integration:** Implement interface in `pkg/store/`, add to registry, update schema, test with mocks.

**Stack Processing:** Core logic in `pkg/stack/` and `internal/exec/stack_processor_utils.go`, test inheritance, validate templates, update schema.

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
Follow registry pattern: 1) Define interface in dedicated package, 2) Implement per provider, 3) Register implementations, 4) Generate mocks. Example: `pkg/store/` with AWS SSM, Azure Key Vault, Google Secret Manager.

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
