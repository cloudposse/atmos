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

## Concurrent Sessions (MANDATORY)

Multiple Claude sessions may be working on the same branch or worktree simultaneously. To avoid destroying other sessions' work:

- **NEVER delete, reset, or discard files you didn't create** - Other sessions may have created them
- **NEVER run `git reset`, `git checkout --`, or `git clean`** without explicit user approval
- **ALWAYS ask the user before removing untracked files** - They may be work-in-progress from another session
- **When you see unfamiliar files**, assume another session created them - ask the user what to do
- **If pre-commit hooks fail due to files you didn't touch**, ask the user how to proceed rather than trying to fix or remove them

**Why this matters:** The user may have multiple Claude sessions working in parallel on different aspects of a feature. Deleting “unknown” files destroys that work.

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

## Working with Atmos Agents (RECOMMENDED)

Atmos has **specialized domain experts** in `.claude/agents/` for focused subsystems. **Use agents instead of inline work** for their areas of expertise.

**Available Agents:**
- **`@agent-developer`** - Creating/maintaining agents, agent architecture
- **`@tui-expert`** - Terminal UI, theme system, output formatting
- **`@atmos-errors`** - Error handling patterns, error builder usage
- **`@flag-handler`** - CLI commands, flag parsing, CommandProvider pattern
- **`@example-creator`** - Creating examples, mock components, test cases, EmbedFile docs

**When to delegate:**
- TUI/theme changes → `@tui-expert`
- New CLI commands → `@flag-handler`
- Error handling refactoring → `@atmos-errors`
- Creating new agents → `@agent-developer`
- Creating examples/demos → `@example-creator`

**Benefits:** Agents are domain experts with deep knowledge of patterns, PRDs, and subsystem architecture. They ensure consistency and best practices.

See `.claude/agents/README.md` for full list and `docs/prd/claude-agent-architecture.md` for architecture.

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

**Exceptions (do NOT add perf.Track):**
- Trivial getters/setters (e.g., `GetName()`, `SetValue()`)
- Command constructor functions (e.g., `DescribeCommand()`, `ListCommand()`)
- Simple factory functions that just return structs
- Functions that only delegate to another tracked function
- Pure validation/lookup functions with no I/O (e.g., `ValidateCloudEnvironment()`, `ResolveDestination()`)

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

### Test Fixture Standards (MANDATORY)
All test data and fixtures that use Atmos config section keys **MUST** use `cfg.XxxSectionName` constants from `pkg/config/const.go`, never hardcoded strings like `"import"`, `"env"`, `"vars"`:
- `"import":` → `cfg.ImportSectionName`
- `"env":` → `cfg.EnvSectionName`
- `"vars":` → `cfg.VarsSectionName`
- `"components":` → `cfg.ComponentsSectionName`
- `"metadata":` → `cfg.MetadataSectionName`
- `"terraform":` → `cfg.TerraformSectionName`

Add a constant-lock test when introducing a new constant dependency in tests. This ensures that if a constant value ever changes, the test catches the drift immediately.

**Severity override validation**: When applying user-configured severity overrides from config or CLI, always normalize and validate values case-insensitively (`error|warning|info`). Invalid values must be silently ignored (leaving original severity unchanged) and optionally debug-logged. Never cast string values directly to a `Severity` type without validation.

**Lint stacks scoping contract**: When `--stack` is provided, scope `RawStackConfigs`, `ImportGraph`, and `AllStackFiles` to the reachable import closure from that stack. If no seed file is found (no raw manifest stem matches the requested stack name), **fail closed** with an error — do NOT fall back silently to repo-scope, which would produce misleading results for rules like L-07.

**Lint stacks import graph normalization**: `buildImportGraph` must emit **absolute paths** for non-glob children whenever `basePath` is known. Non-glob imports must be resolved by joining `basePath` and inferring `.yaml`/`.yml` extensions when missing. This ensures L-03 depth traversal can follow edges using consistent absolute keys that match the importGraph key space.

**Cross-component keying in lint rules**: When building lookup maps that span multiple stacks (e.g., L-02 abstract component base vars), always key by `"<stackName>/<componentName>"` rather than just the component name to prevent false positives/negatives across stacks that define abstract components with the same name but different variables. Keep a secondary global-name fallback for cross-stack inheritance.

**File attribution in lint rules**: Build a `stackNameToFileIndex` (logical-name → absolute-manifest-path) in `LintStacks` from `RawStackConfigs`, pass it via `LintContext.StackNameToFileIndex`, and use it in rules (e.g., L-08) that need to resolve a logical stack name to a physical file path. The heuristic `stackNameToFile` fallback is unreliable for simple stack names without a path separator.

**UI output consistency**: All functions that return an error should also call `ui.Error(...)` before returning to provide a consistent user-facing message, even when the wrapped error will be printed by the top-level handler. This applies to rendering functions like `renderLintJSON`.

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
- **For aliasing/isolation tests, verify BOTH directions:** after a merge, mutate the result and confirm the original inputs are unchanged (result→src isolation); also mutate a source map before the merge and confirm the result is unaffected (src→result isolation).
- **For slice-result tests, assert element contents, not just length:** `require.Len` alone allows regressions that drop or corrupt contents. Assert at least the first and last element by value.
- **Never use platform-specific binaries in tests** (e.g., `false`, `true`, `sh` on Unix): these don't exist on Windows. Use Go-native test helpers: subprocess via `os.Executable()` + `TestMain`, temp files with cross-platform scripts, or DI to inject a fake command runner.
- **Safety guards must fail loudly:** any check that counts fixture files or validates test preconditions must use `require.Positive` (or equivalent) — never `if count > 0 { ... }` which silently disables the check when misconfigured.
- **Use absolute paths for fixture counting:** any `filepath.WalkDir` or file-count assertion must use an already-resolved absolute path (not a relative one) to be CWD-independent.
- **Add compile-time sentinels for schema field references in tests:** when a test uses a specific struct field (e.g., `schema.Provider{Kind: "azure"}`), add `var _ = schema.Provider{Kind: "azure"}` as a compile guard so a field rename immediately fails the build.
- **Add prerequisite sub-tests for subprocess behavior:** when a test depends on implicit env propagation (e.g., `ComponentEnvList` reaching a subprocess), add an explicit sub-test that confirms the behavior before the main test runs.
- **Contract vs. legacy behavior:** if a test says "matches mergo" (or any other library), add an opt-in cross-validation test behind a build tag (e.g., `//go:build compare_mergo`); otherwise state "defined contract" explicitly so it's clear the native implementation owns the behavior. Run cross-validation tests with: `go test -tags compare_mergo ./pkg/merge/... -run CompareMergo -v` (requires mergo v1.0.x installed).
- **Include negative-path tests for recovery logic:** whenever a test verifies that a recovery/fallback triggers under condition X, add a corresponding test that verifies the recovery does NOT trigger when condition X is absent (e.g., mismatched workspace name).

### Follow-up Tracking (MANDATORY)
When a PR defers work to a follow-up (e.g., migration, cleanup, refactor), **open a GitHub issue and link it by number** in the blog post, roadmap, and/or PR description before merging. Blog posts with "a follow-up issue will..." with no `#number` are incomplete — the work will never be tracked.

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
- Link to documentation using current URL paths (e.g., `/stacks`, `/components`, `/cli/configuration`)
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

### Cyclomatic Complexity (MANDATORY)
golangci-lint enforces `cyclop: max-complexity: 15` and `funlen: lines: 60, statements: 40`.
When refactoring high-complexity functions:
1. Extract blocks with clear single responsibilities into named helper functions.
2. Use the pattern: `buildXSubcommandArgs`, `resolveX`, `checkX`, `assembleX`, `handleX`.
3. Keep the orchestrator function as a flat linear pipeline of named steps (see `ExecuteTerraform`).
4. Previously high-complexity functions: `ExecuteTerraform` (160→26, see `internal/exec/terraform.go`), `ExecuteDescribeStacks` (247→10), `processArgsAndFlags`.

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

**Key utilities to reuse (not reinvent):**
- **YAML file discovery**: Use `u.GetAllYamlFilesInDir(dir)` from `pkg/utils/file_utils.go` — returns relative paths. To get absolute paths, join with the base: `filepath.Join(base, relPath)`. Do NOT reimplement `filepath.WalkDir` for `.yaml`/`.yml` enumeration.
- **Slice membership**: Use `u.SliceContainsString(slice, str)` from `pkg/utils/slice_utils.go` — do NOT write inline `for` loops to check membership.
- **Config section constants**: Use `cfg.ComponentsSectionName`, `cfg.TerraformSectionName`, `cfg.HelmfileSectionName`, `cfg.VarsSectionName`, `cfg.MetadataSectionName`, `cfg.EnvSectionName`, `cfg.InheritsSectionName`, `cfg.ImportSectionName` from `pkg/config/const.go` — NEVER hardcode section name strings like `"components"`, `"inherits"`, `"vars"`.
- **Shared internal helpers**: When a helper function (e.g., `extractInherits`, `getNestedMap`) is used by more than one file in the same package, extract it to a `helpers.go` in that package rather than duplicating it.
- **Import map `enabled` field**: When processing Atmos import sections that accept map-form objects (`{path: "...", enabled: false}`), always check for and honor the `enabled` field. Skip disabled imports; don't add them to reference graphs or treated as referenced files.
- **Import path normalization**: When building an import graph (`buildImportGraph`), resolve non-glob relative imports to absolute paths using `basePath` and extension inference (`.yaml` → `.yml` → bare). This ensures depth traversal and orphan detection rules (L-03, L-07) traverse the complete import closure correctly.

### Cross-Platform (MANDATORY)
Linux/macOS/Windows compatible. Use SDKs over binaries. Use `filepath.Join()` instead of hardcoded path separators.

**Subprocess helpers in tests (cross-platform):**
Instead of `exec.LookPath("false")` or other Unix-only binaries, use the test binary itself.
**Important:** If your package already has a `TestMain`, add the env-gate check **inside the existing `TestMain`** — do not add a second `TestMain` function (Go does not allow two in the same package).

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

**Path handling in tests:**
- **NEVER use forward slash concatenation** like `tempDir + "/components/terraform/vpc"`
- **ALWAYS use `filepath.Join()`** with separate arguments: `filepath.Join(tempDir, "components", "terraform", "vpc")`
- **NEVER use forward slashes in `filepath.Join()`** like `filepath.Join(dir, "a/b/c")` - use `filepath.Join(dir, "a", "b", "c")`
- **NEVER hardcode Unix paths in expected values** like `assert.Equal(t, "/project/components/vpc", path)` - build expected paths with `filepath.Join()`
- **For path suffix checks**, use `filepath.ToSlash()` to normalize: `strings.HasSuffix(filepath.ToSlash(path), "expected/suffix")`
- **NEVER use bash/shell commands in tests** - use Go stdlib (`os`, `filepath`, `io`) for file operations

**Why:** Windows uses backslash (`\`) as path separator, Unix uses forward slash (`/`). Hardcoded paths fail on Windows CI.

### Multi-Provider Registry (MANDATORY)
Follow registry pattern: define interface, implement per provider, register implementations, generate mocks. Example: `pkg/store/`

### Telemetry (MANDATORY)
Auto-enabled via `RootCmd.ExecuteC()`. Non-standard paths use `telemetry.CaptureCmd()`. Never capture user data.

## Development Environment

**Prerequisites**: Go 1.26+, golangci-lint, Make. See `.cursor/rules/atmos-rules.mdc`.

**Build**: CGO disabled, cross-platform, version via ldflags, output to `./build/`

### Compilation (MANDATORY)
ALWAYS compile after changes: `go build . && go test ./...`. Fix errors immediately.

### Pre-commit (MANDATORY)
NEVER use `--no-verify`. Run `make lint` before committing. Hooks run go-fumpt, golangci-lint, go mod tidy.
