# PRD: Claude Agent Architecture

**Status:** Draft
**Author:** Claude Code (agent-developer)
**Created:** 2025-11-07
**Last Updated:** 2025-11-07

## Overview

This PRD defines the architecture, conventions, and best practices for Claude agents used in the Atmos development workflow. Agents are specialized AI assistants that embody domain expertise and automate complex development tasks.

## Goals

1. **Standardize agent structure** - Consistent frontmatter and content format
2. **Enable agent coordination** - Multi-agent workflows for complex tasks
3. **Optimize context usage** - Efficient, focused agents that reference documentation
4. **Support self-maintenance** - Agents update themselves as requirements evolve
5. **Ensure PRD awareness** - Agents follow documented architectural patterns
6. **Scale Atmos development** - Create specialized agents as core functionality grows

## Strategic Vision

**Scale development through specialized domain expertise.**

As Atmos core functionality expands, we create **small, purposeful agents that are experts in key areas of Atmos**. Each specialized subsystem gets a dedicated agent.

**Example agents (for illustration purposes):**
- `command-registry-expert` - Command registry patterns
- `component-registry-expert` - Component discovery
- `cobra-flag-expert` - Flag parsing with Cobra
- `stack-processor-expert` - Stack inheritance pipeline
- `template-engine-expert` - Go templates and YAML functions
- `auth-system-expert` - Authentication patterns
- `store-registry-expert` - Multi-provider stores

**When to create an agent:**
1. New core subsystem emerges (registry, integration, major feature)
2. Subsystem reaches complexity (3+ files, distinct patterns)
3. PRD exists with established patterns
4. Repeated questions about the same domain

**Benefits:**
- Consistent pattern application across codebase
- Knowledge retention as expertise is captured
- Faster onboarding with guided development
- Development velocity scales with agent support

## Non-Goals

- Creating agents for every possible task (focus on high-value domains)
- Replacing human decision-making (agents assist, humans decide)
- Embedding all documentation in agents (reference, don't duplicate)

## Agent Architecture

### Directory Structure

```
.conductor/<branch-name>/.claude/agents/
├── README.md                    # Agent collection overview
├── agent-developer.md           # Meta-agent for creating agents
├── cli-developer.md             # CLI/TUI specialist
├── test-automation-expert.md    # Testing specialist
├── security-auditor.md          # Security specialist
└── ...                          # Other domain specialists
```

### File Format

Every agent is a Markdown file with YAML frontmatter:

```markdown
---
name: agent-name
description: >-
  Multi-line description with invocation triggers.

  **Invoke when:**
  - Specific scenario 1
  - Specific scenario 2

tools: Read, Write, Edit, Grep, Glob, Bash, Task, TodoWrite
model: sonnet
color: cyan
---

# Agent Name - Brief Tagline

[Agent content in Markdown]
```

## Frontmatter Specification

### Required Fields

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Kebab-case agent identifier | `test-automation-expert` |
| `description` | string | Multi-line invocation criteria | See template below |
| `tools` | array/string | Available tools or "inherit" | `Read, Write, Edit, Bash` |

### Optional Fields

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `model` | string | Model override | `sonnet`, `haiku`, `inherit` |
| `color` | string | Visual identifier | `cyan`, `purple`, `green` |

### Description Format

The description field is critical for automatic agent invocation:

```yaml
description: >-
  Use this agent for [domain] tasks. Expert in [technologies].

  **Invoke when:**
  - [Specific trigger scenario 1]
  - [Specific trigger scenario 2]
  - [Specific trigger scenario 3]
```

**Requirements:**
- Start with imperative ("Use this agent for...")
- List explicit expertise (technologies, patterns)
- Include 3-5 concrete invocation triggers
- Use active voice and specific language

### Naming Conventions

**Format:** `{domain}-{role}`

**Examples:**
- ✅ `test-automation-expert` (domain + role)
- ✅ `cli-developer` (domain + role)
- ✅ `security-auditor` (domain + role)
- ❌ `developer` (too broad)
- ❌ `test_expert` (wrong separator)
- ❌ `security-audit-agent` (redundant "agent")

### Tool Selection

**Standard tools** (most agents need these):
```yaml
tools: Read, Write, Edit, Grep, Glob, Bash, Task, TodoWrite
```

**Tool purposes:**
- `Read` - Read files and documentation
- `Write` - Create new files
- `Edit` - Modify existing files
- `Grep` - Search code content
- `Glob` - Find files by pattern
- `Bash` - Execute shell commands
- `Task` - Invoke other agents
- `TodoWrite` - Track multi-step workflows

**Specialized tools** (add only if needed):
- `WebFetch` - External API calls
- `NotebookEdit` - Jupyter notebook editing
- `Skill` - Invoke specialized skills

**Default:** Use explicit tool list instead of "inherit" for clarity.

## Content Structure

### Recommended Sections

```markdown
# Agent Name - Brief Tagline

Brief overview paragraph.

## Core Responsibilities

1. Primary responsibility
2. Secondary responsibility
3-5 total

## [Domain] Expertise

Domain-specific knowledge, patterns, best practices.

## Workflow

1. Phase 1: [Name]
   - Step A
   - Step B

2. Phase 2: [Name]
   - Step C
   - Step D

## Quality Checks

- [ ] Validation criteria 1
- [ ] Validation criteria 2

## Relevant PRDs

- `docs/prd/relevant-prd.md` - Description

## Self-Maintenance

How and when to update this agent.

## References

- External resources
- Documentation links
```

### Writing Style Guidelines

**Be concise:**
- ✅ Bullet points and lists
- ✅ Code examples
- ✅ Step-by-step processes
- ❌ Long prose paragraphs
- ❌ Repeated information

**Use imperatives:**
- ✅ "Create tests before implementation"
- ✅ "Invoke code-reviewer for validation"
- ❌ "You should create tests"
- ❌ "It would be good to validate"

**Be specific:**
- ✅ "Use `go.uber.org/mock/mockgen` for mocks"
- ✅ "Follow registry pattern from `docs/prd/command-registry-pattern.md`"
- ❌ "Use a mocking tool"
- ❌ "Follow best practices"

## Context Management

### File Size Guidelines

| Agent Type | Target Size | Max Size |
|------------|-------------|----------|
| Focused specialist | 8-15 KB | 20 KB |
| Comprehensive specialist | 15-25 KB | 40 KB |
| Meta-agent | 15-30 KB | 50 KB |

### Context Efficiency Patterns

**Reference, don't duplicate:**

✅ **Good:**
```markdown
## Code Patterns

Follow patterns documented in:
- `CLAUDE.md` - Core architectural patterns
- `docs/prd/command-registry-pattern.md` - Registry extensibility

**Key requirements:**
- Use registry pattern for extensibility
- Implement interface-driven design
- Generate mocks with mockgen
```

❌ **Bad:**
```markdown
## Code Patterns

[10 KB of content duplicated from CLAUDE.md]
```

**Read live documentation:**

Agents should read documentation at invocation time, not embed it:

```markdown
## Process

1. **Check for PRD**
   ```bash
   find docs/prd/ -name "*{keyword}*"
   ```

2. **Read PRD if exists**
   Use Read tool to access current version

3. **Follow documented patterns**
   Implement per PRD specifications
```

## Agent Coordination

### Invocation Patterns

**Automatic invocation** - Claude Code matches task to agent description:
```
User: "Create comprehensive tests for the auth module"
→ Claude invokes test-automation-expert automatically
```

**Explicit invocation** - User or agent specifies agent:
```
User: "@security-auditor review the authentication code"
→ Claude invokes security-auditor explicitly
```

**Task tool invocation** - Agent coordinates with another agent:
```markdown
If security concerns arise, invoke security-auditor:

<uses Task tool>
- subagent_type: general-purpose (or specific agent if in system)
- Prompt includes context and specific task for agent
```

### Multi-Agent Workflows

Complex tasks can involve multiple agents:

```
feature-development-orchestrator
  ├─> prd-writer (create PRD)
  ├─> cli-developer (implement feature)
  ├─> test-automation-expert (create tests)
  ├─> security-auditor (security review)
  └─> code-reviewer (final validation)
```

**Coordination responsibilities:**
- **Orchestrator agent** - Manages workflow, invokes specialists
- **Specialist agents** - Execute specific tasks, return to orchestrator
- **Validator agents** - Review work, provide feedback

## Self-Updating Pattern

Agents must actively monitor dependencies and update themselves when those dependencies change.

### Dependency Tracking

Every agent must track:
1. **PRD dependencies** - Specific PRD files with version/date
2. **CLAUDE.md sections** - Which patterns it implements
3. **Implementation files** - Key files it references

### Self-Maintenance Section (Required)

```markdown
## Self-Maintenance

This agent actively monitors and updates itself when dependencies change.

**Dependencies to monitor:**
- `docs/prd/{specific-prd}.md` - [Description] (v1.0, YYYY-MM-DD)
- `CLAUDE.md` - Core development patterns
- Related files: `pkg/{package}/`, `internal/exec/{implementation}.go`

**Update triggers:**
1. **PRD updated** - Dependent PRD modified (check with `git log`)
2. **CLAUDE.md changes** - Core patterns evolve
3. **Implementation patterns mature** - Codebase patterns stabilize
4. **Invocation unclear** - Agent not triggered appropriately
5. **Context bloat** - Agent exceeds 20 KB

**Update process:**
1. Detect change: `git log -1 --format="%ai %s" docs/prd/{prd}.md`
2. Read updated documentation
3. Invoke agent-developer for updates
4. Review changes for accuracy
5. Test with sample invocations
6. Commit referencing PRD version

**Self-check:**
- **Before each invocation:** Read latest PRD version
- **When task fails:** Check if patterns changed
- **Periodic:** Monthly or after major features
```

### PRD Currency Checking

Agents should verify PRD currency at invocation:

```markdown
## Workflow

1. **Verify PRD currency** (first step, every invocation)
   ```bash
   # Check when PRD was last updated
   git log -1 --format="%ai %s" docs/prd/command-registry-pattern.md

   # Read current version
   cat docs/prd/command-registry-pattern.md
   ```

2. **Compare with known version**
   - Agent notes: "Last sync: 2025-01-15"
   - PRD shows: "2025-02-20" → Update needed!

3. **Update self if outdated**
   - Invoke agent-developer
   - Sync with new patterns
   - Update version tracker

4. **Proceed with task** using current patterns
```

### Agent-Developer Self-Maintenance

The agent-developer agent itself must:

1. **Monitor its own PRD**
   ```bash
   git log -1 --format="%ai %s" docs/prd/claude-agent-architecture.md
   ```

2. **Update when PRD changes**
   - Read updated architecture patterns
   - Refine agent creation process
   - Update templates and examples
   - Improve quality standards

3. **Propagate updates to existing agents**
   - When architecture PRD changes, review all agents
   - Update agents that don't match new patterns
   - Document migration path for pattern changes

## PRD Awareness Pattern

Every agent must be aware of relevant PRDs:

```markdown
## Relevant PRDs

This agent implements patterns from:

- `docs/prd/command-registry-pattern.md` - Command extensibility architecture
- `docs/prd/testing-strategy.md` - Testing requirements and patterns
- `docs/prd/error-handling-strategy.md` - Error handling conventions

**Before implementing:**

1. **Search for PRDs**
   ```bash
   find docs/prd/ -name "*{keyword}*"
   grep -r "{pattern}" docs/prd/
   ```

2. **Read applicable PRDs**
   Use Read tool to access full PRD content

3. **Follow documented patterns**
   Implement exactly as specified in PRD

4. **Reference PRD in code**
   Add comments linking to PRD sections
```

## Agent Development Workflow

### Creating a New Agent

1. **Invoke agent-developer**
   ```
   User: "Create an agent for database migrations"
   → Invokes agent-developer
   ```

2. **Research domain**
   - Find similar agents: `find .conductor/*/.claude/agents/`
   - Find relevant PRDs: `find docs/prd/ -name "*{keyword}*"`
   - Search codebase patterns: `grep -r "{pattern}" pkg/`

3. **Draft agent**
   - Create frontmatter with correct format
   - Write core sections (responsibilities, expertise, workflow)
   - Add PRD references
   - Include self-maintenance instructions

4. **Validate agent**
   - Check frontmatter YAML syntax
   - Verify file size (target 8-20 KB)
   - Ensure no content duplication
   - Test invocation triggers

5. **Save agent**
   - Save to `.claude/agents/{agent-name}.md`
   - Update README.md with new agent
   - Commit with descriptive message

### Updating an Existing Agent

1. **Invoke agent-developer**
   ```
   User: "Update cli-developer agent for new TUI patterns"
   → Invokes agent-developer
   ```

2. **Read current agent**
   ```bash
   cat .claude/agents/cli-developer.md
   ```

3. **Identify changes needed**
   - New PRD published?
   - Patterns evolved?
   - Invocation unclear?
   - Context bloat?

4. **Update agent**
   - Preserve helpful existing content
   - Add new expertise sections
   - Update references to PRDs
   - Optimize context usage

5. **Validate and save**
   - Verify frontmatter intact
   - Check file size
   - Test invocation
   - Commit changes

## Quality Standards

### Agent Quality Checklist

Before finalizing any agent:

- [ ] Frontmatter uses correct YAML format
- [ ] Name is kebab-case and descriptive
- [ ] Description has 3-5 concrete invocation triggers
- [ ] Tool set is appropriate and explicit
- [ ] Content organized in clear sections
- [ ] File size is reasonable (typically 8-20 KB)
- [ ] References PRDs instead of duplicating
- [ ] Includes self-maintenance guidance
- [ ] Has clear workflow/process section
- [ ] Uses imperative, actionable language
- [ ] No redundant content with CLAUDE.md
- [ ] Examples are concrete and realistic
- [ ] Agent can coordinate with other agents via Task

### Anti-Patterns

❌ **Avoid these patterns:**

1. **Overly broad agents**
   - Problem: "developer" agent (no focus)
   - Solution: "cli-developer", "backend-developer" (specific)

2. **Duplicating documentation**
   - Problem: Embedding entire CLAUDE.md
   - Solution: Reference sections, read at invocation time

3. **Vague invocation triggers**
   - Problem: "Use when you need help"
   - Solution: "Use when implementing CLI commands with Cobra"

4. **Missing PRD awareness**
   - Problem: No references to architectural docs
   - Solution: List relevant PRDs, check before implementing

5. **No self-maintenance**
   - Problem: Static agent that becomes outdated
   - Solution: Include update process and triggers

6. **Unclear coordination**
   - Problem: Doesn't specify when to invoke other agents
   - Solution: Document coordination patterns explicitly

## Implementation Plan

### Phase 1: Foundation (Complete)
- [x] Create agent-developer meta-agent
- [x] Define frontmatter specification
- [x] Document agent architecture patterns
- [x] Create this PRD

### Phase 2: Core Agents (Future)
- [ ] Create specialized agents as needed
- [ ] Establish coordination patterns
- [ ] Document agent workflows
- [ ] Gather usage feedback

### Phase 3: Optimization (Future)
- [ ] Refine invocation triggers
- [ ] Optimize context usage
- [ ] Improve coordination patterns
- [ ] Update based on real-world usage

## Success Metrics

1. **Agent effectiveness** - Tasks completed successfully without human intervention
2. **Context efficiency** - Average agent file size ≤ 20 KB
3. **Coordination success** - Multi-agent workflows complete end-to-end
4. **Maintenance frequency** - Agents update themselves when PRDs change
5. **Developer satisfaction** - Positive feedback on agent assistance

## References

### Documentation
- `CLAUDE.md` - Core development patterns
- `docs/prd/command-registry-pattern.md` - Registry architecture
- `docs/prd/testing-strategy.md` - Testing requirements

### Tools and Resources
- Claude Code documentation
- YAML frontmatter specification
- Task tool for agent coordination

---

**Next Steps:**
1. Review this PRD with team
2. Create additional specialized agents as needed
3. Gather feedback on agent effectiveness
4. Iterate on patterns based on real-world usage
