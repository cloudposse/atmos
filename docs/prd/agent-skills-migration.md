# Agent Skills Migration Plan

## Overview

This document outlines the plan to align Atmos Agents with the [Agent Skills](https://agentskills.io) open standard, which is now governed by the Linux Foundation's Agentic AI Foundation (AAIF).

## Current State

Atmos uses a two-file format for community agents:

```
my-agent/
├── .agent.yaml    # Metadata (YAML)
└── prompt.md      # System prompt (Markdown)
```

## Target State

The Agent Skills standard uses a single-file format:

```
my-skill/
├── SKILL.md       # Metadata (YAML frontmatter) + Prompt (Markdown body)
├── scripts/       # Optional: executable scripts
├── references/    # Optional: context files loaded into prompt
└── assets/        # Optional: images, diagrams, etc.
```

## Format Comparison

### Current Atmos Format

**.agent.yaml:**
```yaml
name: test-agent
display_name: Test Agent
version: 1.0.0
author: Test Author
description: A test agent for unit testing
category: general

atmos:
  min_version: 1.0.0
  max_version: ""

prompt:
  file: prompt.md

tools:
  allowed:
    - describe_stacks
    - describe_component
  restricted:
    - terraform_apply
    - terraform_destroy

repository: https://github.com/test/test-agent
license: MIT
```

**prompt.md:**
```markdown
# Agent: Test Agent

You are a test agent designed for unit testing the Atmos agent marketplace functionality.

## Capabilities

- Testing agent installation
- Testing agent validation
- Testing agent registry operations
```

### Agent Skills Standard Format

**SKILL.md:**
```markdown
---
name: test-agent
description: A test agent for unit testing the Atmos agent marketplace functionality
license: MIT
compatibility:
  atmos: ">=1.0.0"
metadata:
  author: Test Author
  version: 1.0.0
  category: general
  display_name: Test Agent
  repository: https://github.com/test/test-agent
allowed-tools:
  - describe_stacks
  - describe_component
---

# Agent: Test Agent

You are a test agent designed for unit testing the Atmos agent marketplace functionality.

## Capabilities

- Testing agent installation
- Testing agent validation
- Testing agent registry operations
```

## Migration Strategy

### Phase 1: Add SKILL.md Support (Non-Breaking)

1. **Update parser** to support both formats:
   - If `SKILL.md` exists, parse it (new format)
   - If `.agent.yaml` exists, parse it (legacy format)
   - Support both during transition period

2. **Update documentation** to recommend `SKILL.md` format for new agents

3. **Add validation** for Agent Skills specification compliance

### Phase 2: Update Built-in Agents

1. Convert 5 built-in agents to use embedded `SKILL.md` content
2. Maintain internal Go struct representation (no runtime change)

### Phase 3: Deprecate Legacy Format

1. Add deprecation warnings for `.agent.yaml` format
2. Provide migration tool: `atmos agent migrate`
3. Update all documentation

### Phase 4: Remove Legacy Support

1. Remove `.agent.yaml` parser (major version bump)
2. Finalize Agent Skills-only support

## Field Mapping

| Atmos Field | Agent Skills Field |
|-------------|-------------------|
| `name` | `name` |
| `description` | `description` |
| `display_name` | `metadata.display_name` |
| `version` | `metadata.version` |
| `author` | `metadata.author` |
| `category` | `metadata.category` |
| `atmos.min_version` | `compatibility.atmos` |
| `tools.allowed` | `allowed-tools` |
| `tools.restricted` | (not in standard - Atmos extension) |
| `repository` | `metadata.repository` |
| `license` | `license` |

## Atmos Extensions

The Agent Skills standard allows custom metadata. Atmos-specific extensions:

```yaml
metadata:
  # Standard fields
  author: Test Author
  version: 1.0.0

  # Atmos extensions
  category: general
  display_name: Test Agent
  repository: https://github.com/test/test-agent

  # Atmos-specific: restricted tools (not in standard)
  restricted-tools:
    - terraform_apply
    - terraform_destroy
```

## Benefits of Migration

1. **Industry Alignment**: Follows open standard backed by Anthropic, OpenAI, Block
2. **Interoperability**: Skills may work with other tools supporting the standard
3. **Simpler Structure**: Single file vs. two files
4. **Progressive Disclosure**: ~100 token metadata load, full content on demand
5. **Ecosystem Growth**: Access to community skills from other platforms

## Timeline

| Phase | Status | Target |
|-------|--------|--------|
| Phase 1: Add SKILL.md Support | Not Started | Next minor release |
| Phase 2: Update Built-in Agents | Not Started | Following release |
| Phase 3: Deprecate Legacy | Not Started | 6 months after Phase 1 |
| Phase 4: Remove Legacy | Not Started | Next major version |

## References

- [Agent Skills Specification](https://agentskills.io/specification)
- [Agentic AI Foundation](https://lfaidata.foundation/blog/2025/10/23/announcing-the-agentic-ai-foundation/)
- [AGENTS.md Standard](https://github.com/context-labs/agents-md)
