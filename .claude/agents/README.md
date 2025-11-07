# Claude Agents

This directory contains specialized Claude agents for the Atmos project development workflow.

## Available Agents

### agent-developer
**File:** `agent-developer.md`
**Purpose:** Meta-agent for creating, updating, and maintaining other Claude agents
**Invoke when:**
- Creating a new specialized agent
- Updating existing agent instructions
- Fixing agent frontmatter or structure
- Optimizing agents for context efficiency
- Designing multi-agent coordination patterns

## Usage

### Automatic Invocation
Agents are automatically invoked by Claude Code when task descriptions match the agent's invocation triggers defined in the `description` frontmatter field.

### Explicit Invocation
- **Direct reference:** `@agent-name` in conversation
- **Task tool:** Use Task tool with appropriate subagent_type

### Example
```
User: "Create an agent for security auditing"
Claude: "I'll use the agent-developer agent to create a security-auditor agent..."
```

## Agent Architecture

All agents follow these principles:

1. **YAML Frontmatter** - Structured metadata (name, description, tools, model, color)
2. **Focused Expertise** - Single domain, 8-20 KB typical size
3. **PRD-Aware** - Reference and follow relevant PRDs from `docs/prd/`
4. **Self-Updating** - Agents update themselves as requirements evolve
5. **Coordination-Ready** - Can invoke other agents via Task tool
6. **Context-Efficient** - Reference documentation instead of duplicating

## Frontmatter Format

```yaml
---
name: agent-name
description: >-
  Brief description of when to use this agent.

  **Invoke when:**
  - Specific scenario 1
  - Specific scenario 2

tools: Read, Write, Edit, Grep, Glob, Bash, Task, TodoWrite
model: sonnet
color: cyan
---
```

## Best Practices

### Creating New Agents

1. Research domain and existing patterns
2. Check for relevant PRDs in `docs/prd/`
3. Use agent-developer agent for creation
4. Follow context efficiency guidelines
5. Include self-maintenance instructions
6. Test invocation triggers

### Updating Existing Agents

1. Use agent-developer agent for updates
2. Preserve helpful existing content
3. Align with current PRDs
4. Optimize for context usage
5. Update invocation triggers if needed

### Agent Coordination

For complex workflows, agents can coordinate:

```
orchestrator-agent
  ├─> specialist-agent-1 (domain expertise)
  ├─> specialist-agent-2 (implementation)
  └─> code-reviewer (validation)
```

## Maintenance

Agents are self-aware and should update their own instructions when:
- New PRDs are published in their domain
- CLAUDE.md patterns evolve
- Technology stack changes
- Invocation patterns need refinement

The agent-developer agent is responsible for maintaining agent quality and consistency across the collection.

## References

- **Agent examples:** `.conductor/kabul/.claude/agents/` - Comprehensive agent collection
- **Core patterns:** `CLAUDE.md` - Development guidelines
- **Architecture docs:** `docs/prd/` - Product requirement documents
- **Agent PRD:** `docs/prd/claude-agent-architecture.md` - Agent system design

## Quality Standards

All agents must:
- Use correct YAML frontmatter format
- Have kebab-case names
- Include specific invocation triggers
- Reference PRDs instead of duplicating
- Be context-efficient (8-20 KB)
- Include self-maintenance guidance
- Use actionable, imperative language
- Provide clear workflows/processes
