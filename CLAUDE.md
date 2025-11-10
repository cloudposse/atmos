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

**Stack Pipeline**: Load atmos.yaml → process imports/inheritance → apply overrides → render templates → generate config.

**Templates and YAML functions**: Go templates + Gomplate with `atmos.Component()`, `!terraform.state`, `!terraform.output`, store integration.

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
- Wrap with static errors from `errors/errors.go`
- Chain errors: `fmt.Errorf("%w: msg", errUtils.ErrFoo)` - creates error chain
- Join errors: `errors.Join(errUtils.ErrFoo, err)` - combines independent errors
- Multiple wrapping: `fmt.Errorf("%w: context: %w", errUtils.ErrBase, err)` (valid Go 1.20+)
- Check: `errors.Is(err, target)`
- Never dynamic errors or string comparison

**Important distinction:**
- **`fmt.Errorf` with single `%w`**: Creates error **chain** - `errors.Unwrap()` returns next error. Use when error context builds sequentially through call stack. **Prefer this when error chain matters.**
- **`errors.Join`**: Creates **flat list** - `errors.Unwrap()` returns `nil`, must use `Unwrap() []error` interface. Use for independent errors (parallel operations, multiple validations).
- **`fmt.Errorf` with multiple `%w`**: Like `errors.Join` but adds format string. Valid Go 1.20+, returns `Unwrap() []error`.

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

**Responding to CodeRabbit Review Comments (MANDATORY):**

**CRITICAL: NEVER edit CodeRabbit's comments or anyone else's comments. ONLY reply to review threads.**

When addressing CodeRabbit review comments, you MUST reply to the specific review threads, not add a general PR comment or edit existing comments.

**NEVER use these (they EDIT comments):**
- ❌ `gh api repos/OWNER/REPO/pulls/comments/COMMENT_ID --method POST --field body="..."` (EDITS the comment)
- ❌ `gh api repos/OWNER/REPO/pulls/comments/COMMENT_ID --method PATCH` (EDITS the comment)
- ❌ `gh pr comment` (Does NOT notify CodeRabbit or resolve threads)

**ALWAYS use this (replies to thread):**
- ✅ GraphQL mutation `addPullRequestReviewThreadReply` with review thread ID

**Correct workflow to reply to review threads:**
1. Get review thread IDs (format: `PRRT_kwDOEW4XoM5fXXXXXX`) via GraphQL `reviewThreads` query
2. Use GraphQL mutation to reply:
```bash
gh api graphql -f query='
mutation {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: "PRRT_kwDOEW4XoM5fXXXXXX"
    body: "Your response here"
  }) {
    comment { id }
  }
}'
```

**Key distinction:**
- Review thread ID: `PRRT_kwDOEW4XoM5fXXXXXX` (use this with `addPullRequestReviewThreadReply`)
- Comment ID: `2508617685` (NEVER use this - it edits the comment)
3. Each reply should reference the specific issue and explain the fix or dismissal

**Example workflow:**
```bash
# Find unresolved review threads
gh api graphql -f query='query { repository(owner: "cloudposse", name: "atmos") {
  pullRequest(number: XXXX) {
    reviewThreads(first: 100) {
      nodes { id isResolved comments(first: 1) { nodes { body path } } }
    }
  }
} }'

# Reply to a specific thread (use the thread ID from above)
gh api graphql -f query='mutation {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: "THREAD_ID"
    body: "Explanation of fix or why this is a false positive"
  }) {
    comment { id }
  }
}'
```

**When to reply:**
- Always reply when fixing an issue to confirm resolution
- Always reply when dismissing as false positive with clear explanation
- Include file:line references and code snippets when helpful

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

### Lint Errors and Pre-existing Code (MANDATORY)
**Pre-existing code** refers ONLY to code that exists in the `main` branch. If code does not exist in `main`, it is NOT pre-existing - it is new code being added in the current branch.

When fixing lint errors:
- **New code**: ALL lint errors in new code must be fixed before committing
- **Pre-existing code**: Lint errors in code from `main` branch can be left as-is (fix if related to your changes)
- **Determining what's pre-existing**: Use `git diff main...HEAD` to see what's new in your branch

Example: If you're working on a feature branch that adds the `toolchain/` package, and `toolchain/` doesn't exist in `main`, then ALL lint errors in `toolchain/` must be fixed - they are not pre-existing.
