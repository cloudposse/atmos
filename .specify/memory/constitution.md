<!--
SYNC IMPACT REPORT
==================
Version change: (template/unset) → 1.0.0
Bump type: Initial ratification — all content is new.

Modified principles: N/A (initial ratification, no prior principles existed)

Added sections:
  - Core Principles (I–V)
  - Development Standards
  - Quality Gates & Contribution Requirements
  - Governance

Removed sections: N/A

Templates requiring updates:
  - .specify/templates/plan-template.md ✅ — "Constitution Check" section is generic
    and resolves at plan-time; no Atmos-specific edits required.
  - .specify/templates/spec-template.md ✅ — No constitution-specific fields referenced;
    no updates required.
  - .specify/templates/tasks-template.md ✅ — Task phases are project-agnostic;
    no updates required.

Follow-up TODOs: none — all placeholders resolved.
-->

# Atmos Constitution

## Core Principles

### I. Registry-Driven Extensibility

All extensibility MUST flow through the registry pattern. New CLI commands MUST implement
the `CommandProvider` interface and register via `cmd/internal/registry.go`. New
multi-provider integrations MUST follow the store-registry pattern in `pkg/store/registry.go`.
Ad-hoc extensibility outside a registry is not permitted.

**Rationale**: Registries make the system predictable and discoverable; they prevent the
accumulation of hidden coupling and make it straightforward to add or remove capabilities
without touching core orchestration logic.

### II. Interface-Driven Design with Dependency Injection

Every major subsystem MUST expose a well-defined Go interface. Implementations MUST be
injected (never hard-coded) so that unit tests can substitute fakes without I/O. Mocks
MUST be generated with `go.uber.org/mock/mockgen` via `//go:generate` directives —
hand-written mocks are not permitted.

**Rationale**: DI and generated mocks keep unit tests fast, hermetic, and refactor-safe.
Manual mocks drift silently; generated mocks fail at compile time on interface changes.

### III. Test-First with 80 % Coverage (NON-NEGOTIABLE)

Bug fixes MUST begin with a failing test that reproduces the bug before any code change.
New features MUST have unit tests. Coverage is enforced at 80 % minimum by CodeCov.
Table-driven tests are preferred. Integration tests MAY be added for cross-component
contracts, but unit tests with mocks are the primary testing vehicle.

**Rationale**: The 80 % floor is not aspirational; CI blocks merges below this threshold.
Test-first ensures correctness is verified before the implementation is considered
complete, not retrofitted.

### IV. Separated I/O and UI Architecture

All output MUST use the two-layer architecture:
- **I/O layer** (`pkg/io/`) — stream access (stdout/stderr/stdin), TTY detection, masking.
- **UI layer** (`pkg/ui/`) — formatting (colors, icons, markdown rendering).

`fmt.Println`, `fmt.Fprintf(os.Stdout, …)`, and `fmt.Fprintf(os.Stderr, …)` are FORBIDDEN
in all non-test code. Data MUST go to stdout via `data.*`; human messages MUST go to
stderr via `ui.*`. The system handles color degradation, CI detection, and secret masking
automatically — callers MUST NOT replicate that logic.

**Rationale**: Separating streams from formatting makes output testable and pipeable.
Automatic degradation means the caller never needs to branch on terminal capabilities.

### V. Simplicity and No Over-Engineering

Implementations MUST NOT introduce abstractions beyond what the current task requires.
Files MUST remain under 600 lines. New functionality MUST go into purpose-built packages
under `pkg/` — `pkg/utils/` MUST NOT receive new functions. YAGNI applies strictly:
three similar lines of code are acceptable; a premature abstraction that anticipates a
hypothetical fourth use is not.

**Rationale**: Atmos serves a wide range of infrastructure engineers. Understandable,
focused code reduces onboarding time and maintenance burden. Complexity MUST pay its
way with explicit justification.

## Development Standards

- **Comments**: All code comments MUST end with a period (enforced by `godot` linter).
  Comments MUST explain WHY, not WHAT. Existing comments MUST NOT be deleted without
  a strong documented reason.
- **Imports**: Three groups in order — stdlib, third-party, Atmos packages — each
  separated by a blank line and sorted alphabetically. Canonical aliases (`cfg`, `log`,
  `u`, `errUtils`) MUST be preserved.
- **Performance tracking**: Public functions with I/O or significant computation MUST
  open with `defer perf.Track(atmosConfig, "pkg.FuncName")()`. Trivial getters/setters,
  command constructors, and pure validation helpers are exempt.
- **Error handling**: Errors MUST be wrapped with static sentinels from `errors/errors.go`.
  `errors.Join` combines multiple errors. `fmt.Errorf("%w", …)` adds string context.
  The error builder adds hints, exit codes, and user-facing context. Dynamic error
  creation directly in call sites is not permitted.
- **Flag handling**: Commands MUST use `flags.NewStandardParser()` from `pkg/flags/`.
  Direct calls to `viper.BindEnv()` or `viper.BindPFlag()` outside `pkg/flags/` are
  FORBIDDEN and enforced by the Forbidigo linter.
- **Cross-platform**: All code MUST compile and run on Linux, macOS, and Windows.
  Path separators MUST use `filepath.Join()`. Shell binaries (`false`, `true`, `sh`)
  MUST NOT appear in tests — use Go-native helpers or the test binary itself.

## Quality Gates & Contribution Requirements

- **Pre-commit hooks**: MUST NOT be skipped (`--no-verify` is forbidden). Hooks run
  `go-fumpt`, `golangci-lint`, and `go mod tidy`. Lint errors MUST be fixed before commit.
- **Cyclomatic complexity**: `cyclop: max-complexity: 15` and `funlen: lines: 60,
  statements: 40` are enforced. Complex functions MUST be refactored into named helper
  functions with single responsibilities.
- **Documentation**: All new commands and flags MUST have Docusaurus documentation in
  `website/docs/cli/commands/`. Website MUST build without errors after doc changes
  (`cd website && npm run build`).
- **Pull requests**: The `/pull-request` skill MUST be invoked before opening or
  updating any PR to determine the correct semver label and changelog requirements.
  PRs labeled `minor` or `major` MUST include a blog post and a roadmap update.
- **Golden snapshots**: Snapshot files under `tests/test-cases/` and `tests/testdata/`
  MUST NEVER be manually edited. Use `-regenerate-snapshots` flag exclusively.
- **Follow-up tracking**: When a PR defers work, a GitHub issue MUST be opened and
  linked by number in the PR description or blog post before merging.

## Governance

This constitution supersedes all other development practices and guidelines.
Runtime development guidance lives in `CLAUDE.md`; when this constitution and `CLAUDE.md`
conflict on a specific technical point, `CLAUDE.md` governs the implementation detail
and this constitution governs the architectural principle.

**Amendment procedure**:
1. Propose change via PR with a written rationale.
2. Increment `CONSTITUTION_VERSION` following semantic versioning:
   - MAJOR: removal or backward-incompatible redefinition of a principle.
   - MINOR: new principle or materially expanded guidance added.
   - PATCH: clarifications, wording fixes, non-semantic refinements.
3. Update `Last Amended` date to the merge date.
4. Propagate any impacted template or guidance file changes in the same PR.

**Compliance**: All PRs and code reviews MUST verify adherence to the Core Principles.
Complexity violations MUST be explicitly justified in the PR description before merging.

**Version**: 1.0.0 | **Ratified**: 2026-06-09 | **Last Amended**: 2026-06-09
