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

### I/O and UI Usage (MANDATORY)
Atmos separates I/O (streams) from UI (formatting) for clarity and testability.

**Two-layer architecture:**
- **I/O Layer** (`pkg/io/`) - Stream access (stdout/stderr/stdin), terminal capabilities, masking
- **UI Layer** (`pkg/ui/`) - Formatting (colors, styles, markdown rendering)

**Terminal as Text UI:**
The terminal window is effectively a text-based user interface (TextUI) for our CLI. Anything intended for user interaction—menus, prompts, animations, progress indicators—should be rendered to the terminal as UI output (stderr). Data intended for processing, piping, or machine consumption goes to the data channel (stdout).

**Access pattern:**
```go
import (
    iolib "github.com/cloudposse/atmos/pkg/io"
    "github.com/cloudposse/atmos/pkg/ui"
)

// I/O context initialized in cmd/root.go PersistentPreRun
// Available globally after flag parsing via data.Writer() and ui package functions
```

**Output functions (use these):**
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

**Decision tree:**
```
What am I outputting?

├─ Pipeable data (JSON, YAML, results)
│  └─ Use data.Write(), data.Writef(), data.Writeln()
│     data.WriteJSON(), data.WriteYAML()
│
├─ Plain UI messages (no icon, no color)
│  └─ Use ui.Write(), ui.Writef(), ui.Writeln()
│
├─ Status messages (with icons and colors)
│  └─ Use ui.Success(), ui.Error(), ui.Warning(), ui.Info()
│
└─ Formatted documentation
   ├─ Help text, usage → ui.Markdown() (stdout)
   └─ Error details → ui.MarkdownMessage() (stderr)
```

**Anti-patterns (DO NOT use):**
```go
// WRONG: Direct stream access
fmt.Fprintf(os.Stdout, ...)  // Use data.Printf() instead
fmt.Fprintf(os.Stderr, ...)  // Use ui.Success/Error/etc instead
fmt.Println(...)             // Use data.Println() instead

// WRONG: Will be blocked by linter
io := iolib.NewContext()
fmt.Fprintf(io.Data(), ...)  // Use data.Printf() instead
```

**Why this matters:**

**Zero-Configuration Degradation:**
Write code assuming a full-featured TTY - the system automatically handles everything:
- ✅ **Color degradation** - TrueColor → 256 → 16 → None (respects NO_COLOR, CLICOLOR, terminal capability)
- ✅ **Width adaptation** - Automatically wraps to terminal width or config max_width
- ✅ **TTY detection** - Piped/redirected output becomes plain text automatically
- ✅ **CI detection** - Detects CI environments and disables interactivity
- ✅ **Markdown rendering** - Degrades gracefully from styled to plain text
- ✅ **Icon support** - Shows icons in capable terminals, omits in others

**Security & Reliability:**
- ✅ **Automatic secret masking** - AWS keys, tokens, passwords masked before output
- ✅ **Format-aware masking** - Handles JSON/YAML quoted variants
- ✅ **No leakage** - Secrets never reach stdout/stderr/logs
- ✅ **Pattern-based** - Detects common secret patterns automatically

**Developer Experience:**
- ✅ **No capability checking** - Never write `if tty { color() } else { plain() }`
- ✅ **No manual masking** - Never write `redact(secret)` before output
- ✅ **No stream selection** - Just use `data.*` (stdout) or `ui.*` (stderr)
- ✅ **Testable** - Mock data.Writer() and ui functions for unit tests
- ✅ **Enforced by linter** - Prevents direct fmt.Fprintf usage

**User Experience:**
- ✅ **Respects preferences** - Honors --no-color, --redirect-stderr, NO_COLOR env
- ✅ **Pipeline friendly** - `atmos deploy | tee log.txt` works perfectly
- ✅ **Accessibility** - Works in all terminal environments (screen readers, etc.)
- ✅ **Consistent** - Same code path for all output, fewer bugs

**Force Flags (for screenshot generation):**
Use these flags to generate consistent output regardless of environment:
- `--force-tty` / `ATMOS_FORCE_TTY=true` - Force TTY mode with sane defaults (width=120, height=40) when terminal detection fails
- `--force-color` / `ATMOS_FORCE_COLOR=true` - Force TrueColor output even when not a TTY

**Flag behavior:**
- `--color` - Enables color **only if TTY** (respects terminal capabilities)
- `--force-color` - Forces TrueColor **even for non-TTY** (for screenshots)
- `--no-color` - Disables all color
- `terminal.color` in atmos.yaml - Same as `--color` (respects TTY)

**Example:**
```bash
# Generate screenshot with consistent output (using flags)
atmos terraform plan --force-tty --force-color | screenshot.sh

# Generate screenshot with consistent output (using env vars)
ATMOS_FORCE_TTY=true ATMOS_FORCE_COLOR=true atmos terraform plan | screenshot.sh

# Normal usage - automatically detects terminal
atmos terraform plan

# Piped output - automatically disables color
atmos terraform output | jq .vpc_id
```

See `pkg/io/example_test.go` for comprehensive examples.

### Secret Masking with Gitleaks

Atmos uses Gitleaks pattern library (120+ patterns) for comprehensive secret detection:

```yaml
# atmos.yaml
settings:
  terminal:
    mask:
      patterns:
        library: "gitleaks"  # Use Gitleaks patterns (default)
        categories:
          aws: true          # Enable AWS secret detection
          github: true       # Enable GitHub token detection
```

Disable specific categories to reduce false positives:
```yaml
settings:
  terminal:
    mask:
      patterns:
        categories:
          generic: false  # Disable generic patterns
```

Disable masking for debugging:
```bash
atmos terraform plan --mask=false
```

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
**NEVER delete existing comments without a very strong reason.**

Comments are documentation that helps developers understand:
- **Why** code was written a certain way
- **How** complex algorithms or flows work
- **What** edge cases or gotchas to be aware of
- **Where** credentials or configuration come from

**Guidelines:**
- **Preserve helpful comments** - Especially those explaining credential resolution, complex logic, or non-obvious behavior
- **Update comments to match code** - When refactoring, update comments to reflect current implementation
- **Refactor for clarity** - It's okay to improve comment wording or structure for better readability
- **Add context when modifying** - If changing code with comments, ensure comments still accurately describe the behavior

**Acceptable reasons to remove comments:**
- Comment is factually incorrect and cannot be updated
- Code is completely removed
- Comment duplicates what the code obviously does (e.g., `// increment counter` above `counter++`)
- Comment is outdated TODO that has been completed

**Anti-pattern:**
```go
// WRONG: Deleting helpful documentation during refactoring
-// LoadAWSConfig looks for credentials in the following order:
-//   1. Environment variables (AWS_ACCESS_KEY_ID, etc.)
-//   2. Shared credentials file (~/.aws/credentials)
-//   3. EC2 Instance Metadata Service (IMDS)
-//   ... (more helpful details)
 func LoadAWSConfig(ctx context.Context) (aws.Config, error) {
```

**Correct pattern:**
```go
// CORRECT: Preserving and updating helpful documentation
-// LoadAWSConfig looks for credentials in the following order:
+// LoadAWSConfigWithAuth looks for credentials in the following order:
+// When authContext is provided, uses Atmos-managed credentials.
+// Otherwise, falls back to standard AWS SDK resolution:
 //   1. Environment variables (AWS_ACCESS_KEY_ID, etc.)
 //   2. Shared credentials file (~/.aws/credentials)
 //   3. EC2 Instance Metadata Service (IMDS)
 //   ... (more helpful details)
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

### Error Handling (MANDATORY)
- **All errors MUST be wrapped using static errors defined in `errors/errors.go`**
- **Use `errors.Join` for combining multiple errors** - preserves all error chains
- **Use `fmt.Errorf` with `%w` for adding string context** - when you need formatted strings
- **Use error builder for complex errors** - adds hints, context, and exit codes
- **Use `errors.Is()` for error checking** - robust against wrapping
- **NEVER use dynamic errors directly** - triggers linting warnings
- **See `docs/errors.md`** for complete developer guide.

**Important distinction:**
- **`fmt.Errorf` with single `%w`**: Creates error **chain** - `errors.Unwrap()` returns next error. Use when error context builds sequentially through call stack. **Prefer this when error chain matters.**
- **`errors.Join`**: Creates **flat list** - `errors.Unwrap()` returns `nil`, must use `Unwrap() []error` interface. Use for independent errors (parallel operations, multiple validations).
- **`fmt.Errorf` with multiple `%w`**: Like `errors.Join` but adds format string. Valid Go 1.20+, returns `Unwrap() []error`.

**Combining Multiple Errors:**
```go
// ✅ CORRECT: Use errors.Join (unlimited errors, no formatting)
return errors.Join(errUtils.ErrFailedToProcess, underlyingErr)

// Note: Go 1.20+ supports fmt.Errorf("%w: %w", ...) but errors.Join is preferred
```

**Adding String Context:**
```go
return fmt.Errorf("%w: component=%s stack=%s", errUtils.ErrInvalidComponent, component, stack)
```

**Error Builder for Complex Errors:**
```go
import (
    errUtils "github.com/cloudposse/atmos/errors"
)

// Use builder for errors with hints, context, and exit codes.
err := errUtils.Build(errUtils.ErrLoadAwsConfig).
    WithHint("Check database credentials in atmos.yaml").
    WithHintf("Verify network connectivity to %s", dbHost).
    WithContext("component", "vpc").
    WithContext("stack", "prod").
    WithExitCode(2).
    Err()
```

**Checking Errors:**
```go
// ✅ CORRECT: Works with wrapped errors
if errors.Is(err, context.DeadlineExceeded) { ... }

// ❌ WRONG: Breaks with wrapping
if err.Error() == "context deadline exceeded" { ... }
```

**Static Error Definitions:**
```go
// Define in errors/errors.go
var (
    ErrInvalidComponent = errors.New("invalid component")
    ErrInvalidStack     = errors.New("invalid stack")
)
```

**Exit Codes:**
```go
// Attach exit code
err := errUtils.WithExitCode(err, 2)  // Usage error

// Or use builder
err := errUtils.Build(err).WithExitCode(2).Err()

// Extract exit code
exitCode := errUtils.GetExitCode(err)
// Returns: 0 (nil), custom code, exec.ExitError code, or 1 (default)
```

**Error Formatting:**
```go
// Format error for display with hints and color
config := errUtils.DefaultFormatterConfig()
config.Verbose = false  // Compact mode
config.Color = "auto"   // TTY detection

formatted := errUtils.Format(err, config)
fmt.Fprint(os.Stderr, formatted)
```

**Sentry Integration:**
```go
// Initialize Sentry from config
err := errUtils.InitializeSentry(&atmosConfig.Errors.Sentry)
defer errUtils.CloseSentry()

// Capture error with Atmos context
context := map[string]string{
    "component": "vpc",
    "stack":     "prod",
}
errUtils.CaptureErrorWithContext(err, context)
```

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
- **NEVER manually edit golden snapshot files** - Always use `-regenerate-snapshots` flag
- **ALWAYS use the test flag to regenerate** - Manual edits fail due to environment-specific formatting
- Snapshots capture exact output including invisible formatting (lipgloss padding, ANSI codes, trailing whitespace)
- Different environments produce different output (terminal width, Unicode support, styling libraries)

**Regeneration process:**
```bash
# Regenerate specific test
go test ./tests -run 'TestCLICommands/test_name' -regenerate-snapshots

# Verify snapshot
go test ./tests -run 'TestCLICommands/test_name' -v

# Review changes
git diff tests/snapshots/
```

**Why manual editing fails:**
- Lipgloss table padding varies by terminal width and environment
- Trailing whitespace is significant but invisible in editors
- ANSI color codes may differ between environments
- Unicode character rendering affects column width calculations

**When snapshot tests fail in CI:**
1. Regenerate locally: `go test ./tests -run 'TestName' -regenerate-snapshots`
2. Verify: `go test ./tests -run 'TestName'`
3. Commit and push the regenerated snapshot
4. If still fails: Environment mismatch - contact maintainers

```go
func TestGitHubVendoring(t *testing.T) {
    // Check GitHub access with rate limits
    rateLimits := tests.RequireGitHubAccess(t)
    if rateLimits != nil && rateLimits.Remaining < 20 {
        t.Skipf("Need at least 20 GitHub API requests, only %d remaining", rateLimits.Remaining)
    }
    // ... test code
}
```

### Running Specific Tests
```bash
# Run specific test
go test ./pkg/config -run TestConfigLoad
# Run with coverage
go test ./pkg/config -cover
# Integration tests
go test ./tests -run TestCLI
```

### Regenerating Test Snapshots

When CLI output changes, regenerate snapshots to match:

```bash
# Regenerate ALL snapshots
go test ./tests -v -regenerate-snapshots

# Regenerate specific test snapshot
go test ./tests -v -run 'TestCLICommands/atmos_workflow_invalid_step_type' -regenerate-snapshots

# Review changes
git diff tests/snapshots

# Add updated snapshots
git add tests/snapshots/*
```

**CRITICAL**: Never use pipe redirection (`2>&1`, `| head`, `| tail`) when running tests. Piping interferes with TTY detection and causes tests to use ASCII fallback mode instead of proper ANSI rendering, resulting in incorrect snapshots.

**Why this matters**: Atmos uses `term.IsTTYSupportForStdout()` to detect terminal capability. Piping breaks this detection:
- ✅ No pipes → TTY detected → Proper ANSI rendering → Correct snapshots
- ❌ With pipes → No TTY → ASCII fallback → Wrong snapshots.

### Test Data
Use fixtures in `tests/test-cases/` for integration tests. Each test case should have:
- `atmos.yaml` - Configuration
- `stacks/` - Stack definitions
- `components/` - Component configurations.

### Golden Snapshots (MANDATORY)
- **NEVER modify files under `tests/test-cases/` or `tests/testdata/`** unless explicitly instructed
- These directories contain golden snapshots that are sensitive to even minor changes
- Golden snapshots are used to verify expected output remains consistent
- If you need to update golden snapshots, do so intentionally and document the reason

See `tests/README.md` for details.

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

**Verifying Documentation Links (MANDATORY):**
Before adding links to documentation pages, ALWAYS verify the correct URL:

```bash
# Example: Finding the correct URL for auth user configure command
# Step 1: Find the doc file
find website/docs/cli/commands -name "*user-configure*"
# Output: website/docs/cli/commands/auth/auth-user-configure.mdx

# Step 2: Check the slug in frontmatter
head -10 website/docs/cli/commands/auth/auth-user-configure.mdx | grep slug
# Output: slug: /cli/commands/auth/auth-user-configure

# Step 3: Verify by checking existing links
grep -r "/cli/commands/auth/auth-user-configure" website/docs/
```

**Common mistakes:**
- Using command name instead of filename (e.g., `/cli/commands/auth/atmos_auth` when file is `usage.mdx`)
- Not checking the `slug` frontmatter which can override default URLs
- Guessing URLs instead of verifying against existing documentation structure

**Correct approach:**
1. Find the target doc file: `find website/docs/cli/commands -name "*keyword*"`
2. Check for `slug:` in frontmatter: `head -10 <file> | grep slug`
3. If no slug, URL is path from `docs/` without extension (e.g., `auth-user-configure.mdx` → `/cli/commands/auth/auth-user-configure`)
4. Verify by searching for existing links: `grep -r "<url>" website/docs/`

### PRD Documentation (MANDATORY)
All Product Requirement Documents (PRDs) MUST be placed in `docs/prd/`. Use kebab-case filenames. Examples: `command-registry-pattern.md`, `error-handling-strategy.md`, `testing-strategy.md`

### Pull Requests (MANDATORY)
Follow template (what/why/references).

**Blog Posts (CI Enforced):**
- PRs labeled `minor` or `major` MUST include blog post in `website/blog/YYYY-MM-DD-feature-name.mdx`
- Blog posts must use `.mdx` extension with YAML front matter
- Include `<!--truncate-->` after intro paragraph
- Tag `feature`/`enhancement`/`bugfix` (user-facing) or `contributors` (internal changes)
- CI will fail without blog post

**Blog post authorship:**
- Author should always be the committer (the one who opened the PR)
- Use GitHub username in authors list, not generic "atmos" or "cloudposse"
- Add author to `website/blog/authors.yml` if not already present

Use `no-release` label for docs-only changes.

### PR Tools
Check status: `gh pr checks {pr} --repo cloudposse/atmos`
Reply to threads: Use `gh api graphql` with `addPullRequestReviewThreadReply`

```go
   func (m *MyCommandProvider) GetGroup() string {
       return "Other Commands" // See docs/developing-atmos-commands.md
   }
   ```
3. **Add blank import to `cmd/root.go`**: `_ "github.com/cloudposse/atmos/cmd/mycommand"`
4. **Implement business logic** in `internal/exec/mycommand.go`
5. **Add tests** in `cmd/mycommand/mycommand_test.go`
6. **Create Docusaurus documentation** in `website/docs/cli/commands/<command>/<subcommand>.mdx`
7. **Build website to verify**: `cd website && npm run build`
8. **Create pull request following template format**

**See:**
- **[docs/developing-atmos-commands.md](docs/developing-atmos-commands.md)** - Complete guide with patterns and examples
- **[docs/prd/command-registry-pattern.md](docs/prd/command-registry-pattern.md)** - Architecture and design decisions

### Documentation Requirements (MANDATORY)
- **All new commands/flags/parameters MUST have Docusaurus documentation**
- **Use definition lists `<dl>` instead of tables** for arguments and flags:
  ```mdx
  ## Arguments

  <dl>
    <dt>`component`</dt>
    <dd>Atmos component name</dd>

    <dt>`stack`</dt>
    <dd>Atmos stack name</dd>
  </dl>

  ## Flags

  <dl>
    <dt>`--stack` / `-s`</dt>
    <dd>Atmos stack (required)</dd>

    <dt>`--format`</dt>
    <dd>Output format: `yaml`, `json`, or `table` (default: `yaml`)</dd>
  </dl>
  ```

- **Follow Docusaurus conventions** from existing files:
  ```mdx
  ---
  title: atmos command subcommand
  sidebar_label: subcommand
  sidebar_class_name: command
  id: subcommand
  description: Brief description of what the command does
  ---
  import Screengrab from '@site/src/components/Screengrab'
  import Terminal from '@site/src/components/Terminal'

  :::note Purpose
  Use this command to [describe purpose with links to concepts].
  :::

  <Screengrab title="atmos command subcommand --help" slug="atmos-command-subcommand--help" />

  ## Usage

  ```shell
  atmos command subcommand <args> [options]
  ```

  ## Examples

  ```shell
  atmos command subcommand example1
  atmos command subcommand example2 --flag=value
  ```

- **File location**: `website/docs/cli/commands/<command>/<subcommand>.mdx`
- **Link to core concepts** using `/core-concepts/` paths
- **Include purpose note** and help screengrab
- **Use consistent section ordering**: Usage → Examples → Arguments → Flags

### Website Documentation Build (MANDATORY)
- **ALWAYS build the website after any documentation changes** to verify there are no broken links or formatting issues
- **Build command**: Run from the `website/` directory:
  ```bash
  cd website
  npm run build
  ```
- **When to build**:
  - After adding/modifying any `.mdx` or `.md` files in `website/docs/`
  - After adding images to `website/static/img/`
  - After changing navigation in `website/sidebars.js`
  - After modifying any component in `website/src/`
- **What to check**:
  - Build completes without errors
  - No broken links reported
  - No missing images
  - Proper rendering of MDX components
- **Example workflow**:
  ```bash
  # 1. Make documentation changes
  vim website/docs/cli/commands/describe/stacks.mdx

  # 2. Build to verify
  cd website
  npm run build

  # 3. If errors, fix and rebuild
  # 4. Commit changes only after successful build
  ```

### Pull Request Requirements (MANDATORY)
- **Follow the pull request template** in `.github/PULL_REQUEST_TEMPLATE.md`:
  ```markdown
  ## what
  - High-level description of changes in plain English
  - Use bullet points for clarity

  ## why
  - Business justification for the changes
  - Explain why these changes solve the problem
  - Use bullet points for clarity

  ## references
  - Link to supporting GitHub issues or documentation
  - Use `closes #123` if PR closes an issue
  ```
- **Add changelog blog post for feature releases**:
  - PRs labeled `minor` or `major` MUST include a blog post in `website/blog/`
  - Create a new file: `website/blog/YYYY-MM-DD-feature-name.md`
  - Follow the format of existing blog posts (see template below)
  - Include `<!--truncate-->` marker after the introduction paragraph
  - The CI workflow will fail and comment on the PR if this is missing

### Blog Post Guidelines (MANDATORY)

Blog posts serve different audiences and must be tagged appropriately:

#### Audience Types

**1. User-Facing Posts** (Features, Improvements, Bug Fixes)
- **Audience**: Teams using Atmos to manage infrastructure
- **Focus**: How the change benefits users, usage examples, migration guides
- **Required tags**: Choose one or more:
  - `feature` - New user-facing capabilities
  - `enhancement` - Improvements to existing features
  - `bugfix` - Important bug fixes that affect users
- **Example tags**: `[feature, terraform, workflows]`

**2. Contributor-Facing Posts** (Refactoring, Internal Changes, Developer Tools)
- **Audience**: Atmos contributors and core developers
- **Focus**: Internal code structure, refactoring, developer experience
- **Required tag**: `contributors`
- **Additional tags**: Describe the technical area
- **Example tags**: `[contributors, atmos-core, refactoring]`

#### Blog Post Template

```markdown
---
slug: descriptive-slug
title: "Clear, Descriptive Title"
authors: [atmos]
tags: [primary-tag, secondary-tag, ...]  # See audience types above
---

Brief introduction paragraph explaining what changed and why it matters.

<!--truncate-->

## What Changed

Describe the change with code examples or visuals.

## Why This Matters / Impact on Users

Explain the benefits or reasoning.

## [For User Posts] How to Use It

Provide practical examples and usage instructions.

## [For Contributor Posts] For Atmos Contributors

Clarify this is internal with zero user impact, link to technical docs.

## Get Involved

- Link to relevant documentation
- Encourage discussion/contributions
```

#### Tag Reference

**Primary Audience Tags:**
- `feature` - New user-facing feature
- `enhancement` - Improvement to existing feature
- `bugfix` - Important bug fix
- `contributors` - For Atmos core contributors (internal changes)

**Secondary Technical Tags (for contributor posts):**
- `atmos-core` - Changes to Atmos codebase/internals
- `refactoring` - Code refactoring and restructuring
- `testing` - Test infrastructure improvements
- `ci-cd` - CI/CD pipeline changes
- `developer-experience` - Developer tooling improvements

**Secondary Technical Tags (for user posts):**
- `terraform` - Terraform-specific features
- `helmfile` - Helmfile-specific features
- `workflows` - Workflow features
- `validation` - Validation features
- `performance` - Performance improvements
- `cloud-architecture` - Cloud architecture patterns (user-facing)

**General Tags:**
- `announcements` - Major announcements
- `breaking-changes` - Breaking changes requiring migration

- **Use `no-release` label for documentation-only changes**
- **Ensure all CI checks pass** before requesting review

### Checking PR Security Alerts and CI Status
Use the GitHub CLI (`gh`) to inspect PR checks and security alerts:

```bash
# View PR checks status
gh pr checks {pr-number} --repo {owner/repo}

# Get check run annotations for a specific check (e.g., linting issues)
gh api repos/{owner/repo}/check-runs/{check-run-id}/annotations

# Get code scanning alerts for the repository
gh api repos/{owner/repo}/code-scanning/alerts

# Example for Atmos repository:
gh pr checks 1450 --repo cloudposse/atmos
gh api repos/cloudposse/atmos/check-runs/49737026433/annotations
```

### Responding to PR Review Threads (MANDATORY)
- **ALWAYS reply to specific review threads** - Do not create new PR comments
- **Use GraphQL API to reply to threads**:
  ```bash
  gh api graphql -f query='
  mutation {
    addPullRequestReviewThreadReply(input: {
      pullRequestReviewThreadId: "PRRT_kwDOEW4XoM5..."
      body: "Your response here"
    }) {
      comment { id }
    }
  }'
  ```
- Get unresolved threads:
  ```bash
  gh api graphql -f query='
  query {
    repository(owner: "cloudposse", name: "atmos") {
      pullRequest(number: 1504) {
        reviewThreads(first: 50) {
          nodes {
            id
            isResolved
            path
            line
            comments(first: 1) {
              nodes { body }
            }
          }
        }
      }
    }
  }' | jq -r '.data.repository.pullRequest.reviewThreads.nodes[] | select(.isResolved == false)'
  ```

### Adding Template Function
1. Implement in `internal/exec/template_funcs.go`
2. Register in template function map
3. Add comprehensive tests
4. Document in website if user-facing

### Bug Fixing Workflow (MANDATORY)
1. **Write a test to reproduce the bug** - create failing test that demonstrates the issue
2. **Run the test to confirm it fails** - verify the test reproduces the expected behavior
3. **Fix the bug iteratively** - make changes and re-run test until it passes
4. **Verify fix doesn't break existing functionality** - run full test suite

```go
// Example: Test should describe the expected behavior, not that it's a bug fix
func TestParseConfig_HandlesEmptyStringInput(t *testing.T) {
    // Setup conditions that reproduce the issue
    input := ""

    // Call the function that should handle this case
    result, err := ParseConfig(input)

    // Assert the expected behavior (this should initially fail)
    assert.NoError(t, err)
    assert.Equal(t, DefaultConfig, result)
}

// Or for error conditions:
func TestValidateStack_ReturnsErrorForInvalidFormat(t *testing.T) {
    invalidStack := "malformed-stack-config"

    err := ValidateStack(invalidStack)

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "invalid format")
}
```

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
