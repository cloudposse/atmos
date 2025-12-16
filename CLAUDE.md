# CLAUDE.md

Guidance for Claude Code when working with this repository.

## Project Overview

Atmos: Go CLI for cloud infrastructure orchestration via Terraform/Helmfile/Packer with stack-based config, templating, policy validation, vendoring, and terminal UI.

## Git Worktrees (MANDATORY)

This repository uses git worktrees for parallel development. When working in a worktree:

- **ALWAYS stay within the current working directory** - Never escape to parent directories
- **Use relative paths or paths under the current working directory** - Do not hardcode `/Users/*/atmos/` paths
- **The worktree IS the repository** - All files, including `pkg/`, `cmd/`, `internal/`, exist within the worktree
- **Never assume the parent directory is the repo** - Worktrees like `.conductor/branch-name/` are complete, independent working copies

**Why this matters:** Searching outside the worktree will find stale code from the main branch instead of the current branch's code. This leads to incorrect analysis and recommendations.

**For Task agents:** When searching for files, always use the current working directory (`.`) or relative paths. Never construct absolute paths that might escape the worktree.

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
Use registry pattern for extensibility. Existing implementations:
- **Command Registry**: `cmd/internal/registry.go` - All commands register via `CommandProvider` interface
- **Store Registry**: `pkg/store/registry.go` - Multi-provider store implementations

**New commands MUST use command registry pattern.** See `docs/prd/command-registry-pattern.md`

### Interface-Driven Design (MANDATORY)
- Define interfaces for all major functionality. Use dependency injection for testability
- Generate mocks with `go.uber.org/mock/mockgen`
- Avoid integration tests by mocking external dependencies

### Options Pattern (MANDATORY)
Use functional options pattern for configuration instead of functions with many parameters. Provides defaults, extensible without breaking changes.

### Context Usage (MANDATORY)
Use `context.Context` **only** for:
- Cancellation signals across API boundaries
- Deadlines/timeouts for operation time limits
- Request-scoped values (trace IDs, request IDs - sparingly)

**DO NOT use context for:** Passing configuration (use Options pattern), passing dependencies (use struct fields or DI), or avoiding proper function parameters.

**Context should be first parameter** in functions that accept it.

### I/O and UI Usage (MANDATORY)
Atmos separates I/O (streams) from UI (formatting) for clarity and testability.

**Two-layer architecture:**
- **I/O Layer** (`pkg/io/`) - Stream access (stdout/stderr/stdin), terminal capabilities, masking
- **UI Layer** (`pkg/ui/`) - Formatting (colors, styles, markdown rendering)

**Output functions:**
```go
// Data channel (stdout) - for pipeable output
data.Write/Writef/Writeln("result")
data.WriteJSON/WriteYAML(structData)

// UI channel (stderr) - for human messages
ui.Write/Writef/Writeln("message")      // Plain (no icon, no color)
ui.Success/Error/Warning/Info("status") // With icons and colors
ui.Markdown/MarkdownMessage("text")     // Formatted docs
```

**Anti-patterns (DO NOT use):**
```go
fmt.Fprintf(os.Stdout/Stderr, ...)  // Use data.* or ui.* instead
fmt.Println(...)                     // Use data.Writeln() instead
```

**Zero-Configuration Degradation:** Write code assuming full TTY - system automatically handles color degradation, width adaptation, TTY detection, CI detection, markdown rendering, icon support, secret masking, and format-aware masking.

**Force Flags (for screenshot generation):**
- `--force-tty` / `ATMOS_FORCE_TTY=true` - Force TTY mode
- `--force-color` / `ATMOS_FORCE_COLOR=true` - Force TrueColor output

See `pkg/io/example_test.go` for comprehensive examples.

### Secret Masking with Gitleaks

Atmos uses Gitleaks pattern library (120+ patterns). Disable masking: `atmos terraform plan --mask=false`

### Package Organization (MANDATORY)
- **Avoid utils package bloat** - Don't add new functions to `pkg/utils/`
- **Create purpose-built packages** - New functionality gets its own package in `pkg/`
- Examples: `pkg/store/`, `pkg/git/`, `pkg/pro/`, `pkg/filesystem/`

## Code Patterns & Conventions

### Comment Style (MANDATORY)
All comments must end with periods (enforced by `godot` linter).

### Comment Preservation (MANDATORY)
**NEVER delete existing comments without a very strong reason.** Preserve helpful comments explaining why/how/what/where. Update comments to match code when refactoring.

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

- Commands MUST use `flags.NewStandardParser()` for command-specific flags
- **NEVER call `viper.BindEnv()` or `viper.BindPFlag()` directly** - Forbidigo enforces this
- See `cmd/version/version.go` for reference implementation
- Consult flag-handler agent for all flag-related work

### Error Handling (MANDATORY)
- **All errors MUST be wrapped using static errors defined in `errors/errors.go`**
- **Use `errors.Join` for combining multiple errors** - preserves all error chains
- **Use `fmt.Errorf` with `%w` for adding string context**
- **Use error builder for complex errors** - adds hints, context, and exit codes
- **Use `errors.Is()` for error checking** - robust against wrapping
- **NEVER use dynamic errors directly** - triggers linting warnings
- **See `docs/errors.md`** for complete developer guide

### Testing Strategy (MANDATORY)
- **Prefer unit tests with mocks** over integration tests
- Use interfaces + dependency injection for testability
- Generate mocks with `go.uber.org/mock/mockgen`
- Table-driven tests for comprehensive coverage
- Target >80% coverage

### Test Isolation (MANDATORY)
ALWAYS use `cmd.NewTestKit(t)` for cmd tests. Auto-cleans RootCmd state (flags, args).

### Test Quality (MANDATORY)
- Test behavior, not implementation
- Never test stub functions - either implement or remove
- Avoid tautological tests
- Make code testable via DI
- No coverage theater
- Remove always-skipped tests
- Use `errors.Is()` for error checking

### Mock Generation (MANDATORY)
Use `go.uber.org/mock/mockgen` with `//go:generate` directives. Never manual mocks.

### CLI Command Structure
Embed examples from `cmd/markdown/*_usage.md` using `//go:embed`. Render with `utils.PrintfMarkdown()`.

### File Organization (MANDATORY)
Small focused files (<600 lines). One cmd/impl per file. Co-locate tests. Never `//revive:disable:file-length-limit`.

## Testing

**Preconditions**: Tests skip gracefully with helpers from `tests/test_preconditions.go`. See `docs/prd/testing-strategy.md`.

**Commands**: `make test-short` (quick), `make testacc` (all), `make testacc-cover` (coverage)

**Fixtures**: `tests/test-cases/` for integration tests

**Golden Snapshots (MANDATORY):**
- **NEVER manually edit golden snapshot files** - Always use `-regenerate-snapshots` flag
- Snapshots capture exact output including invisible formatting (lipgloss padding, ANSI codes, trailing whitespace)
- Different environments produce different output (terminal width, Unicode support, styling libraries)

**Regeneration:**
```bash
go test ./tests -run 'TestCLICommands/test_name' -regenerate-snapshots
git diff tests/snapshots/
```

**CRITICAL**: Never use pipe redirection when running tests. Piping breaks TTY detection.

**Golden Snapshot Files:**
- **NEVER modify files under `tests/test-cases/` or `tests/testdata/`** unless explicitly instructed
- These contain golden snapshots sensitive to even minor changes

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

**Verifying Links:** Find doc file (`find website/docs/cli/commands -name "*keyword*"`), check slug in frontmatter (`head -10 <file> | grep slug`), verify existing links (`grep -r "<url>" website/docs/`).

**Common mistakes:** Using command name vs. filename, not checking slug frontmatter, guessing URLs.

### Documentation Requirements (MANDATORY)
CLI command docs MUST include:
1. **Frontmatter** - title, sidebar_label, sidebar_class_name, id, description
2. **Intro component** - `import Intro from '@site/src/components/Intro'` then `<Intro>Brief description</Intro>`
3. **Screengrab** - `import Screengrab from '@site/src/components/Screengrab'` then `<Screengrab title="..." slug="..." />`
4. **Usage section** - Shell code block with command syntax
5. **Arguments/Flags** - Use `<dl><dt>` for each argument/flag with `<dd>` description
6. **Examples section** - Practical usage examples

File location: `website/docs/cli/commands/<command>/<subcommand>.mdx`

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
All Product Requirement Documents (PRDs) MUST be placed in `docs/prd/`. Use kebab-case filenames.

### Pull Requests (MANDATORY)
Follow template (what/why/references).

**Blog Posts (CI Enforced):**
- PRs labeled `minor`/`major` MUST include blog post: `website/blog/YYYY-MM-DD-feature-name.mdx`
- Use `.mdx` with YAML front matter, `<!--truncate-->` after intro
- **MUST read `website/blog/tags.yml`** - Only use tags defined there, never invent new tags
- **MUST read `website/blog/authors.yml`** - Use existing author or add new entry for committer

**Blog Template:**
```markdown
---
slug: descriptive-slug
title: "Clear Title"
authors: [username]
tags: [feature]
---
Brief intro.
<!--truncate-->
## What Changed / Why This Matters / How to Use It / Get Involved
```

**Valid Tags (from `website/blog/tags.yml`):**
- User-facing: `feature`, `enhancement`, `bugfix`, `dx`, `breaking-change`, `security`, `documentation`, `deprecation`
- Internal: `core` (for contributor-only changes with zero user impact)

**Roadmap Updates (CI Enforced):**
- PRs labeled `minor`/`major` MUST also update `website/src/data/roadmap.js`
- For new features: Add milestone to relevant initiative with `status: 'shipped'`
- Link to changelog: Add `changelog: 'your-blog-slug'` to the milestone
- Link to PR: Add `pr: <pr-number>` to the milestone
- Update initiative `progress` percentage: `(shipped milestones / total milestones) * 100`
- See `.claude/agents/roadmap.md` for detailed update instructions

Use `no-release` label for docs-only changes.

### PR Tools
Check status: `gh pr checks {pr} --repo cloudposse/atmos`
Reply to threads: Use `gh api graphql` with `addPullRequestReviewThreadReply`

### Documentation Requirements (MANDATORY)
- All new commands/flags/parameters MUST have Docusaurus documentation
- Use definition lists `<dl>` instead of tables for arguments and flags
- Follow Docusaurus conventions from existing files
- File location: `website/docs/cli/commands/<command>/<subcommand>.mdx`
- Link to core concepts using `/core-concepts/` paths
- Include purpose note and help screengrab
- Use consistent section ordering: Usage → Examples → Arguments → Flags

### Website Documentation Build (MANDATORY)
ALWAYS build the website after documentation changes: `cd website && npm run build`

### Bug Fixing Workflow (MANDATORY)
1. Write a test to reproduce the bug
2. Run the test to confirm it fails
3. Fix the bug iteratively
4. Verify fix doesn't break existing functionality

## Critical Development Requirements

### Git (MANDATORY)
Don't commit: todos, research, scratch files. Do commit: code, tests, requested docs, schemas. Update `.gitignore` for patterns only.

**NEVER run destructive git commands without explicit user confirmation:**
- `git reset HEAD` or `git reset --hard` - discards staged/committed changes
- `git checkout HEAD -- .` or `git checkout -- .` - discards all working changes
- `git clean -fd` - deletes untracked files
- `git stash drop` - permanently deletes stashed changes

Always ask first: "This will discard uncommitted changes. Proceed? [y/N]"

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
Follow registry pattern: define interface, implement per provider, register implementations, generate mocks. Example: `pkg/store/`

### Telemetry (MANDATORY)
Auto-enabled via `RootCmd.ExecuteC()`. Non-standard paths use `telemetry.CaptureCmd()`. Never capture user data.

## Development Environment

**Prerequisites**: Go 1.24+, golangci-lint, Make. See `.cursor/rules/atmos-rules.mdc`.

**Build**: CGO disabled, cross-platform, version via ldflags, output to `./build/`

### Compilation (MANDATORY)
ALWAYS compile after changes: `go build . && go test ./...`. Fix errors immediately.

### Pre-commit (MANDATORY)
NEVER use `--no-verify`. Run `make lint` before committing. Hooks run go-fumpt, golangci-lint, go mod tidy.
