# Claude Agents

Specialized Claude agents for Atmos development. Each agent is an expert in a specific domain, helping scale development
through focused expertise.

## Available Agents

### agent-developer

Expert in creating and maintaining Claude agents with correct frontmatter, context efficiency, and PRD awareness.

**Use when:** Creating new agents, updating existing agents, or optimizing agent instructions.

### tui-expert

Theme-aware Terminal UI system expert. Use for developing UI components, refactoring to theme-aware patterns, and theme architecture guidance.

**Use when:** Working with theme system, TUI components, or terminal output formatting.

### example-creator

Expert in creating Atmos examples with proper structure, documentation, mock components, and CI testing integration.

**Use when:** Creating new examples/demos, adding mock components, writing test cases for examples, or updating documentation with EmbedFile components.

### coderabbit-review

Reviews and addresses CodeRabbit feedback on code changes: parses review threads, verifies each finding against current code, applies valid fixes, skips stale/wrong ones with explanation.

**Use when:** A CodeRabbit review lands with unresolved feedback, on-demand or from the `pr-maintenance-loop` skill's hourly cycle.

### lint-fix

Fixes `golangci-lint` findings (patch-scoped or full-repo) produced by the `lint` skill, following this repo's formatting/error-handling/comment conventions.

**Use when:** The `lint` skill has findings to fix, either human-invoked or from `pr-maintenance-loop`'s hourly cycle.

### test-coverage-fix

Fixes in-scope failing tests (never pre-existing ones), then fixes genuine coverage gaps on added lines, for the current patch vs `origin/main`. Zero tolerance for coverage theater or weakened assertions.

**Use when:** The `test-coverage` skill has failures or coverage gaps to address, either human-invoked or from `pr-maintenance-loop`'s hourly cycle.

### merge-conflict-resolve

Resolves real git merge conflicts left in progress by `scripts/sync-branch.sh` when `origin/main` can't be auto-merged — only when confident it's a structural, non-overlapping conflict; aborts and reports for human attention otherwise.

**Use when:** The `fix-all` skill's sync step reports `STATUS: MERGE_CONFLICT`, either human-invoked or from `pr-maintenance-loop`'s hourly cycle.

## Strategic Approach

As Atmos grows, we create focused agents for each major subsystem. This scales development velocity through specialized
domain expertise.

**Example future agents:**

- `command-registry-expert` - Command registry patterns
- `cobra-flag-expert` - Flag parsing and Cobra integration
- `stack-processor-expert` - Stack inheritance pipeline
- `auth-system-expert` - Authentication patterns

New agents are created in separate PRs as subsystems mature and patterns are established.

## Usage

Agents are automatically invoked based on task descriptions, or explicitly via `@agent-name`.

## Quality Standards

All agents:

- Follow patterns in `docs/prd/claude-agent-architecture.md`
- Stay under 25KB for context efficiency
- Reference PRDs instead of duplicating content
- Self-update when dependencies change (with user approval)

See `docs/prd/claude-agent-architecture.md` for complete architecture and guidelines.
