# Atmos AI Built-in Skills

This directory contains built-in skills for Atmos AI following the [Agent Skills](https://agentskills.io) open standard (Linux Foundation AAIF).

## Directory Structure

Each skill is a directory containing a `SKILL.md` file:

```
builtin/
├── general/
│   └── SKILL.md
├── stack-analyzer/
│   └── SKILL.md
├── component-refactor/
│   └── SKILL.md
├── security-auditor/
│   └── SKILL.md
└── config-validator/
    └── SKILL.md
```

## SKILL.md Format

Each skill file follows the Agent Skills standard with YAML frontmatter and Markdown body:

```markdown
---
name: skill-name
description: Brief description of what this skill does and when to use it.
metadata:
  author: Cloud Posse
  version: "1.0"
---

# Skill: [Name]

## Role
Brief description of the skill's purpose

## Your Expertise
- Domain knowledge areas
- Specializations
- Key competencies

## Instructions
Step-by-step guidance for the skill
```

### Required Fields

| Field | Description |
|-------|-------------|
| `name` | Lowercase, hyphens only, max 64 chars, must match directory name |
| `description` | Max 1024 chars, describes what the skill does and when to use it |

### Optional Fields

| Field | Description |
|-------|-------------|
| `license` | License name or reference |
| `compatibility` | Environment requirements |
| `metadata` | Arbitrary key-value pairs (author, version, etc.) |
| `allowed-tools` | Space-delimited list of pre-approved tools |

## Available Skills

| Skill | Purpose |
|-------|---------|
| **general** | General-purpose assistant for all Atmos operations |
| **stack-analyzer** | Stack configuration analysis and dependency mapping |
| **component-refactor** | Component design and Terraform refactoring |
| **security-auditor** | Security review and compliance validation |
| **config-validator** | Configuration validation and schema checking |

## How Skills Are Loaded

1. **Skill Registration** - Skill defined with `SystemPromptPath` pointing to `{skill}/SKILL.md`
2. **Skill Activation** - When user switches to skill (Ctrl+A), file is read
3. **System Prompt** - Markdown body becomes skill's system prompt
4. **Embedded FS** - Files embedded in binary via `//go:embed` for distribution

## Updating Skills

To update a skill's behavior:

1. Edit the corresponding `SKILL.md` file
2. Test with `go run . ai chat` and switch to the skill (Ctrl+A)
3. Commit changes to Git
4. Rebuild Atmos to embed updated skills

## Progressive Disclosure

Following the Agent Skills standard:

- **Metadata** (~100 tokens) - `name` and `description` loaded at startup for all skills
- **Instructions** (< 5000 tokens recommended) - Full SKILL.md body loaded when skill is activated
- **Resources** (as needed) - Additional files loaded only when required

## References

- [Agent Skills Specification](https://agentskills.io/specification)
- [Agentic AI Foundation](https://lfaidata.foundation/)
