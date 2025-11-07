# Atmos Agents

Specialized Claude Code agents for working with Atmos codebase.

## Available Agents

### `tui-expert.md`
Theme-aware Terminal UI system expert. Use for developing UI components, refactoring to theme-aware patterns, and theme architecture guidance.

## Usage

Agents are automatically available in Claude Code. Reference them in prompts:

```
@tui-expert How do I make this table theme-aware?
```

Or Claude Code will invoke them automatically when the task matches their description.

## Adding New Agents

Create a `.md` file with YAML frontmatter:

```markdown
---
name: agent-name
description: When to use this agent
tools: Read, Edit, Write
model: inherit
---

System prompt with expertise and instructions...
```

See [Claude Code documentation](https://code.claude.com/docs/en/sub-agents.md) for details.
