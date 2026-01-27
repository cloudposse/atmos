# Agent Skills Migration Plan

## Overview

This document outlines the plan to align Atmos with the [Agent Skills](https://agentskills.io) open standard, which is governed by the Linux Foundation's Agentic AI Foundation (AAIF).

**Status: COMPLETED** - The migration from "Agents" to "Skills" terminology and SKILL.md format is complete.

## Summary of Changes

### Terminology Changes
- **Agents** → **Skills** throughout the codebase
- **pkg/ai/agents/** → **pkg/ai/skills/**
- **cmd/ai/agent/** → **cmd/ai/skill/**
- `atmos ai agent` → `atmos ai skill` commands
- `default_agent` → `default_skill` config option
- TUI references updated (skill selector, skill icons, etc.)

### Format Changes
- **Old format**: `.agent.yaml` + `prompt.md` (two files)
- **New format**: `SKILL.md` (single file with YAML frontmatter + Markdown body)

## Current State (Post-Migration)

Skills use a single-file format following the Agent Skills open standard:

```
my-skill/
├── SKILL.md       # Metadata (YAML frontmatter) + Prompt (Markdown body)
├── scripts/       # Optional: executable scripts
├── references/    # Optional: context files loaded into prompt
└── assets/        # Optional: images, diagrams, etc.
```

### SKILL.md Format

```markdown
---
name: terraform-expert
display_name: "Terraform Expert"
description: Specialized AI skill for Terraform development
version: 1.0.0
author: Cloud Posse
category: refactor
icon: "🔧"

atmos:
  min_version: 1.50.0

tools:
  allowed:
    - atmos_describe_component
    - read_component_file
    - search_files
  restricted:
    - edit_file
    - execute_bash

repository: https://github.com/cloudposse/atmos-skill-terraform
---

# Skill: Terraform Expert

You are a specialized AI skill for Terraform component development...

## Your Expertise

- Terraform Development
- Component Architecture
- Best Practices

## Instructions

[Detailed instructions here]
```

## Package Structure (Post-Migration)

```
pkg/ai/skills/
├── skill.go           # Skill struct and methods
├── registry.go        # Thread-safe registry
├── builtin.go         # Built-in skill definitions
├── loader.go          # Load skills from config
└── prompts/
    └── embedded.go    # SKILL.md parser with go:embed
    └── *.SKILL.md     # 5 built-in skill files
```

## CLI Commands (Post-Migration)

```bash
# Install skill from marketplace
atmos ai skill install github.com/cloudposse/atmos-skill-terraform

# List installed skills
atmos ai skill list

# Uninstall skill
atmos ai skill uninstall terraform-expert
```

## Configuration (Post-Migration)

```yaml
settings:
  ai:
    enabled: true
    default_provider: anthropic
    default_skill: general  # Changed from default_agent

    skills:  # Changed from agents
      custom-skill:
        display_name: "Custom Skill"
        description: "My custom skill"
        system_prompt: |
          You are a custom skill...
        allowed_tools:
          - atmos_describe_component
```

## Migration Completed

All phases have been completed:

| Phase | Status | Description |
|-------|--------|-------------|
| Phase 1: Add SKILL.md Support | ✅ Complete | Parser supports SKILL.md format |
| Phase 2: Update Built-in Skills | ✅ Complete | 5 built-in skills converted |
| Phase 3: Rename to Skills | ✅ Complete | All code and docs updated |
| Phase 4: Remove Legacy | ✅ Complete | Old .agent.yaml format removed |

## Benefits Achieved

1. **Industry Alignment**: Follows open standard backed by Linux Foundation AAIF
2. **Interoperability**: Skills may work with other tools supporting the standard
3. **Simpler Structure**: Single file vs. two files
4. **Progressive Disclosure**: ~100 token metadata load, full content on demand
5. **Ecosystem Growth**: Access to community skills from other platforms

## References

- [Agent Skills Specification](https://agentskills.io/specification)
- [Agentic AI Foundation](https://lfaidata.foundation/blog/2025/10/23/announcing-the-agentic-ai-foundation/)
- [Atmos Skills Documentation](/ai/skills)
- [Atmos Skill Marketplace](/ai/skill-marketplace)
