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

## Architecture

- **`cmd/`** - CLI commands (one per file)
- **`internal/exec/`** - Business logic
- **`pkg/`** - config, stack, component, utils, validate, workflow, hooks, telemetry

**Stack Pipeline**: Load atmos.yaml → process imports/inheritance → apply overrides → render templates → generate config.

## Architectural Patterns (MANDATORY)

### Registry Pattern (MANDATORY)
Use registry pattern for extensibility. Existing: Command Registry (`cmd/internal/registry.go`), Component Registry, Store Registry (`pkg/store/registry.go`). **New commands MUST use command registry pattern.** See `docs/prd/command-registry-pattern.md`

### Interface-Driven Design (MANDATORY)
Define interfaces for all major functionality. Use dependency injection for testability. Generate mocks with `go.uber.org/mock/mockgen`. Avoid integration tests by mocking external dependencies.

### Options Pattern (MANDATORY)
Use functional options pattern for configuration. Avoids parameter drilling, provides defaults, extensible without breaking changes.

### Context Usage (MANDATORY)
Use `context.Context` ONLY for:
- Cancellation signals - Propagate cancellation across API boundaries
- Deadlines/timeouts - Set operation time limits
- Request-scoped values - Trace IDs (sparingly)

DO NOT use for: configuration (use Options), dependencies (use struct fields/DI), or avoiding proper parameters. Context should be first parameter.

### I/O and UI Usage (MANDATORY)
Atmos separates I/O (streams) from UI (formatting).

**Two-layer architecture:**
- **I/O Layer** (`pkg/io/`) - Streams (stdout/stderr/stdin), terminal capabilities, masking
- **UI Layer** (`pkg/ui/`) - Formatting (colors, styles, markdown)

**Output functions:**
```go
// Data channel (stdout) - pipeable output
data.Write("result")
data.WriteJSON(structData)
data.WriteYAML(structData)

// UI channel (stderr) - human messages
ui.Success("Done!")              // ✓ Done! (green)
ui.Error("Failed")               // ✗ Failed (red)
ui.Warning("Deprecated")         // ⚠ Deprecated (yellow)
ui.Info("Processing...")         // ℹ Processing... (cyan)
```

**Anti-patterns (DO NOT use):**
```go
fmt.Fprintf(os.Stdout, ...)  // Use data.Write() instead
fmt.Fprintf(os.Stderr, ...)  // Use ui.Success/Error/etc instead
```

**Why this matters:**
- ✅ Color degradation, width adaptation, TTY detection (automatic)
- ✅ Automatic secret masking (AWS keys, tokens, passwords)
- ✅ Testable, enforced by linter
- ✅ Respects --no-color, NO_COLOR env, pipeline friendly

**Force Flags (for screenshots):**
- `--force-tty` / `ATMOS_FORCE_TTY=true` - Force TTY mode (width=120)
- `--force-color` / `ATMOS_FORCE_COLOR=true` - Force TrueColor

See `pkg/io/example_test.go` for examples.

### Package Organization (MANDATORY)
- **Avoid utils bloat** - Don't add to `pkg/utils/`
- **Create purpose-built packages** - New functionality gets own package in `pkg/`
- Examples: `pkg/store/`, `pkg/git/`, `pkg/pro/`

## Code Patterns & Conventions

### Comment Style (MANDATORY)
All comments must end with periods (enforced by `godot` linter).

### Comment Preservation (MANDATORY)
**NEVER delete existing comments without very strong reason.** Preserve, update, or refactor for clarity. Acceptable reasons: factually incorrect, code removed, obvious duplication, completed TODO.

### Import Organization (MANDATORY)
Three groups (blank line separated), alphabetically sorted:
1. Go stdlib
2. 3rd-party (NOT cloudposse/atmos)
3. Atmos packages

Maintain aliases: `cfg`, `log`, `u`, `errUtils`

### Performance Tracking (MANDATORY)
Add `defer perf.Track(atmosConfig, "pkg.FuncName")()` + blank line to all public functions. Use `nil` if no atmosConfig param.

### Configuration Loading
Precedence: CLI flags → ENV vars → config files → defaults (use Viper)

### Error Handling (MANDATORY)
- **All errors MUST be wrapped using static errors from `errors/errors.go`**
- **Use `errors.Join` for combining multiple errors**
- **Use `fmt.Errorf` with `%w` for string context**
- **Use error builder for complex errors** (hints, context, exit codes)
- **Use `errors.Is()` for checking**
- **See `docs/errors.md`** for complete guide

**Examples:**
```go
// Combining errors
return errors.Join(errUtils.ErrFailedToProcess, underlyingErr)

// String context
return fmt.Errorf("%w: component=%s", errUtils.ErrInvalidComponent, component)

// Error builder
err := errUtils.Build(errUtils.ErrLoadAwsConfig).
    WithHint("Check credentials").
    WithContext("component", "vpc").
    WithExitCode(2).
    Err()

// Checking
if errors.Is(err, context.DeadlineExceeded) { ... }
```

### Testing Strategy (MANDATORY)
- Prefer unit tests with mocks over integration tests
- Use interfaces + DI for testability
- Generate mocks with `go.uber.org/mock/mockgen`
- Table-driven tests for coverage
- Target >80% coverage

### Test Isolation (MANDATORY)
ALWAYS use `cmd.NewTestKit(t)` for cmd tests. Auto-cleans RootCmd state.

### Test Quality (MANDATORY)
- Test behavior, not implementation
- Never test stub functions - implement or remove test
- Avoid tautological tests
- Make code testable with DI
- No coverage theater
- Remove always-skipped tests
- Use `errors.Is()` for error checking

### Mock Generation (MANDATORY)
Use `go.uber.org/mock/mockgen` with `//go:generate` directives. Never manual mocks.

### File Organization (MANDATORY)
Small focused files (<600 lines). One cmd/impl per file. Co-locate tests. Never `//revive:disable:file-length-limit`.

## Testing

**Commands**: `make test-short`, `make testacc`, `make testacc-cover`

**Golden Snapshots (MANDATORY):**
- **NEVER manually edit snapshot files** - Always use `-regenerate-snapshots`
- Different environments produce different output
- Regenerate: `go test ./tests -run 'TestName' -regenerate-snapshots`
- **CRITICAL**: Never pipe test output (`2>&1`, `| head`) - breaks TTY detection

**Test Data:**
- **NEVER modify `tests/test-cases/` or `tests/testdata/`** unless explicitly instructed
- Use fixtures for integration tests
- See `tests/README.md`

## Common Development Tasks

### Adding New CLI Command
1. Create `cmd/[command]/` with CommandProvider interface
2. Add blank import to `cmd/root.go`
3. Implement in `internal/exec/mycommand.go`
4. Add tests, Docusaurus docs in `website/docs/cli/commands/`
5. Build website: `cd website && npm run build`

See `docs/developing-atmos-commands.md` and `docs/prd/command-registry-pattern.md`

### Documentation (MANDATORY)
All cmds/flags need Docusaurus docs. Use `<dl>` for args/flags. Build: `cd website && npm run build`

**Verifying Links:** ALWAYS verify URLs before adding:
```bash
find website/docs/cli/commands -name "*keyword*"
head -10 <file> | grep slug
grep -r "<url>" website/docs/
```

### PRD Documentation (MANDATORY)
All PRDs MUST be in `docs/prd/`. Use kebab-case filenames.

### Pull Requests (MANDATORY)
Follow template (what/why/references).

**Blog Posts (CI Enforced):**
- PRs labeled `minor`/`major` MUST include blog post: `website/blog/YYYY-MM-DD-feature-name.mdx`
- Use `.mdx`, YAML front matter, `<!--truncate-->` after intro
- Tag: `feature`/`enhancement`/`bugfix` (user) or `contributors` (internal)
- Author = committer (GitHub username, not "atmos")

**Blog Tags:**
- Primary: `feature`, `enhancement`, `bugfix`, `contributors`
- Technical: `atmos-core`, `refactoring`, `testing`, `ci-cd`
- User: `terraform`, `helmfile`, `workflows`, `validation`

Use `no-release` label for docs-only.

### PR Tools
Check: `gh pr checks {pr} --repo cloudposse/atmos`

**Responding to CodeRabbit (MANDATORY):**
**CRITICAL: NEVER edit comments. ONLY reply to review threads.**

Use GraphQL `addPullRequestReviewThreadReply`:
```bash
gh api graphql -f query='
mutation {
  addPullRequestReviewThreadReply(input: {
    pullRequestReviewThreadId: "PRRT_kwDOEW4XoM5..."
    body: "Response"
  }) {
    comment { id }
  }
}'
```

Get threads:
```bash
gh api graphql -f query='query {
  repository(owner: "cloudposse", name: "atmos") {
    pullRequest(number: XXXX) {
      reviewThreads(first: 100) {
        nodes { id isResolved comments(first: 1) { nodes { body path } } }
      }
    }
  }
}'
```

### Website Build (MANDATORY)
ALWAYS build after doc changes: `cd website && npm run build`

### Bug Fixing (MANDATORY)
1. Write failing test
2. Confirm it fails
3. Fix iteratively
4. Run full test suite

## Critical Development Requirements

### Git (MANDATORY)
Don't commit: todos, research, scratch. Do commit: code, tests, requested docs, schemas.

### Test Coverage (MANDATORY)
80% minimum (CodeCov). `make testacc-coverage` for reports.

### Environment Variables (MANDATORY)
Use `viper.BindEnv("ATMOS_VAR", "ATMOS_VAR", "FALLBACK")` - ATMOS_ prefix required.

### Logging vs UI (MANDATORY)
UI (prompts, status) → stderr. Data → stdout. Logging for system events only.

### Schemas (MANDATORY)
Update schemas in `pkg/datafetcher/schema/` when adding config options.

### Theme (MANDATORY)
Use colors from `pkg/ui/theme/colors.go`

### Templates (MANDATORY)
New configs support Go templating with `FuncMap()` from `internal/exec/template_funcs.go`

### Code Reuse (MANDATORY)
Search `internal/exec/` and `pkg/` before implementing. Extend, don't duplicate.

### Cross-Platform (MANDATORY)
Linux/macOS/Windows compatible. Use SDKs over binaries. Use `filepath.Join()`.

### Multi-Provider Registry (MANDATORY)
Follow registry pattern: define interface, implement per provider, register, generate mocks. Example: `pkg/store/`

### Telemetry (MANDATORY)
Auto-enabled via `RootCmd.ExecuteC()`. Never capture user data.

## Development Environment

**Prerequisites**: Go 1.24+, golangci-lint, Make

### Compilation (MANDATORY)
ALWAYS compile after changes: `go build . && go test ./...`

### Pre-commit (MANDATORY)
NEVER use `--no-verify`. Run `make lint` before committing.

### Lint Errors (MANDATORY)
**Pre-existing code** = exists in `main` branch only. New code must fix ALL lint errors. Use `git diff main...HEAD` to check.
