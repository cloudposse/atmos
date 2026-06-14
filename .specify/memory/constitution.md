<!--
SYNC IMPACT REPORT
==================
Version change: [TEMPLATE] → 1.0.0 (initial fill — MINOR: all sections newly defined)

Modified principles:
  [PRINCIPLE_1_NAME] → I. Registry-Driven Architecture
  [PRINCIPLE_2_NAME] → II. Interface-First, Mock-Based Testing
  [PRINCIPLE_3_NAME] → III. Separation of I/O and UI (NON-NEGOTIABLE)
  [PRINCIPLE_4_NAME] → IV. Complexity Budget
  [PRINCIPLE_5_NAME] → V. Cross-Platform & Error Contract

Added sections:
  - Code Quality Gates (replaces [SECTION_2_NAME])
  - Development Workflow & PR Contract (replaces [SECTION_3_NAME])

Removed sections: none

Templates requiring updates:
  ✅ .specify/templates/plan-template.md — "Constitution Check" gates now have concrete names
  ✅ .specify/templates/spec-template.md — no structural changes required
  ✅ .specify/templates/tasks-template.md — no structural changes required

Deferred TODOs: none — all placeholders resolved from CLAUDE.md + codebase context.
-->

# Atmos Constitution

## Core Principles

### I. Registry-Driven Architecture

Every CLI command MUST register via the `CommandProvider` interface in `cmd/internal/registry.go`
and be imported with a blank import in `cmd/root.go`. No command may be wired directly to
`RootCmd` outside the registry pattern. Multi-provider backends (stores, CI plugins, etc.) MUST
follow the `pkg/store/registry.go` pattern: define an interface, implement per provider,
register implementations, generate mocks.

**Rationale**: The registry pattern is the primary extension mechanism. Bypassing it creates
untracked commands that miss telemetry, flag binding, and test isolation guarantees.

### II. Interface-First, Mock-Based Testing

All major functionality MUST be defined as a Go interface before implementation. Mocks MUST be
generated with `go.uber.org/mock/mockgen` via `//go:generate` directives — never hand-written.
Unit test coverage MUST reach ≥80% (CodeCov enforced). Integration tests are only acceptable
where mocking is impossible (e.g., actual filesystem layout). Table-driven tests are the default
structure. Tests MUST verify behavior, not implementation; tautological tests and stub-only tests
are prohibited.

**Rationale**: Interface + DI enables testing without cloud credentials or live infrastructure.
Manual mocks diverge silently; generated mocks fail at compile time.

### III. Separation of I/O and UI (NON-NEGOTIABLE)

Pipeline-consumable data MUST be written to stdout via `data.*` functions (`pkg/io/`).
Human-facing status, warnings, and errors MUST be written to stderr via `ui.*` functions
(`pkg/ui/`). `fmt.Println`, `fmt.Fprintf(os.Stdout/Stderr, ...)`, and direct `log.*` calls for
UI output are PROHIBITED in all business logic. Code MUST be written assuming full TTY; the
runtime handles color degradation, CI detection, width adaptation, and secret masking
automatically.

**Rationale**: Mixing stdout/stderr breaks pipelines and scripting. The two-layer architecture
is the sole guarantee that `atmos ... | jq` works and that secrets are masked consistently.

### IV. Complexity Budget

Functions MUST stay within `cyclop: max-complexity: 15` and `funlen: lines: 60, statements: 40`
(golangci-lint enforced). When a function exceeds budget, extract named helpers following the
`buildX`, `resolveX`, `checkX`, `assembleX`, `handleX` naming pattern. The orchestrator function
MUST remain a flat linear pipeline of named steps. `//revive:disable:file-length-limit` is NEVER
permitted. File size MUST remain below 600 lines.

**Rationale**: High cyclomatic complexity is the primary source of untestable code and merge
conflicts in this codebase. The budget is a hard gate, not a guideline.

### V. Cross-Platform & Error Contract

All path construction MUST use `filepath.Join()` with individual path segments — never string
concatenation or forward slashes inside `filepath.Join`. Environment variable bindings MUST use
`viper.BindEnv` with the `ATMOS_` prefix via `pkg/flags/` infrastructure; direct
`viper.BindEnv()` or `viper.BindPFlag()` calls outside `pkg/flags/` are PROHIBITED (Forbidigo
enforced). All errors MUST be wrapped against static sentinel values defined in
`errors/errors.go`; dynamic `errors.New(...)` at call sites is PROHIBITED. `errors.Is()` MUST be
used for error checking.

**Rationale**: The Windows CI matrix catches path separator bugs; Forbidigo prevents flag binding
drift; static sentinels make error chains introspectable and testable.

## Code Quality Gates

All code merged to `main` MUST pass these automated gates:

- **Linting**: `make lint` (custom golangci-lint + lintroller). Enforces godot (comments end with
  periods), gofumpt, cyclop, funlen, forbidigo, and Atmos-specific rules (no `t.Setenv` misuse,
  no `os.Setenv` in tests).
- **Imports**: Three groups separated by blank lines in alphabetical order — Go stdlib, 3rd-party,
  Atmos packages (`github.com/cloudposse/atmos/...`). Maintain aliases: `cfg`, `log`, `u`,
  `errUtils`.
- **Performance tracking**: `defer perf.Track(atmosConfig, "pkg.FuncName")()` MUST be the first
  line of all public functions. Exceptions: trivial getters/setters, command constructor functions
  (`DescribeCommand()`, `ListCommand()`), simple factory functions, pure validation/lookup
  functions, functions that only delegate to another already-tracked function.
- **Schemas**: Any new config option MUST update `pkg/datafetcher/schema/`.
- **Theme**: Colors MUST come from `pkg/ui/theme/colors.go` — no hardcoded hex values.
- **No pre-commit bypass**: `--no-verify` is NEVER permitted. Hooks run gofumpt, golangci-lint,
  and `go mod tidy`.

## Development Workflow & PR Contract

### Bug Fix Workflow

1. Write a failing test that reproduces the bug.
2. Confirm the test fails.
3. Fix the bug iteratively.
4. Confirm existing tests still pass (`make test-short`).

### New CLI Command Checklist

1. Create `cmd/[command]/` with `CommandProvider` interface.
2. Add blank import to `cmd/root.go`.
3. Implement business logic in `internal/exec/[command].go`.
4. Add unit tests targeting ≥80% coverage.
5. Add Docusaurus documentation in `website/docs/cli/commands/<command>/<subcommand>.mdx`.
6. Build and verify: `cd website && npm run build`.

### PR Label & Release Artifact Rules

- All PRs MUST use the `/pull-request` skill before opening/updating.
- `no-release`: docs-only changes.
- `patch`/`minor`/`major`: follow semantic versioning.
- `minor`/`major` PRs MUST include a blog post at `website/blog/YYYY-MM-DD-feature-name.mdx`
  using only tags from `website/blog/tags.yml` and authors from `website/blog/authors.yml`.
- `minor`/`major` PRs MUST update `website/src/data/roadmap.js` with `status: 'shipped'`,
  `changelog`, and `pr` fields.
- Deferred follow-up work MUST be tracked as a GitHub issue with a `#number` reference before
  merging.

### PRD Documentation

All Product Requirement Documents MUST be placed in `docs/prd/` using kebab-case filenames.

## Governance

This constitution supersedes all other development practices for the Atmos project. Where
CLAUDE.md provides more detailed runtime guidance on applying these principles, CLAUDE.md
is authoritative on the *how*; this constitution is authoritative on the *what*.

**Amendment procedure**:

1. Propose the change by updating this file with the new or revised principle.
2. Increment `CONSTITUTION_VERSION` per semantic versioning rules defined in this document.
3. Update `LAST_AMENDED_DATE` to the amendment date (ISO 8601).
4. Update any dependent templates (plan, spec, tasks) if the amendment adds or removes
   mandatory sections or task categories.
5. The amendment is ratified when merged to `main` under a PR labeled `no-release` or higher.

**Versioning policy**:
- MAJOR: Backward-incompatible removals or redefinitions of existing principles.
- MINOR: New principle or section added, or material expansion of existing guidance.
- PATCH: Clarifications, wording fixes, non-semantic refinements.

**Compliance review**: Every PR description MUST include a "Constitution Check" confirming no
principle is violated, or explicitly justify any exception with a Complexity Tracking entry in
`plan.md`.

**Version**: 1.0.0 | **Ratified**: 2026-06-09 | **Last Amended**: 2026-06-09
