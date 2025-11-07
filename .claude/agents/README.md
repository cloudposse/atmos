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
