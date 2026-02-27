# PRD: Atmos Agent Skills

## Status: Planned

## Overview

Add Atmos agent skills to the Atmos repository so that AI-powered development tools (Claude Code,
GitHub Copilot, OpenAI Codex, etc.) can provide accurate, context-aware assistance for Atmos
stack configuration, Terraform orchestration, CI/CD integration, and related workflows.

Skills give AI agents deep, structured knowledge about Atmos that goes beyond what their training
data provides. Users point their tools at the skills, and the agent loads relevant guidance
on demand via progressive disclosure.

## Problem Statement

1. AI agents hallucinate Atmos configuration syntax because their training data is outdated or sparse.
2. There is no standardized way for Atmos users to give their AI tools accurate Atmos knowledge.
3. The existing skills in the external `cloudposse/agent-skills` repo are not distributed with Atmos itself, requiring a
   separate installation step.

## Goals

1. Ship Atmos skills inside the Atmos repository at `.claude/skills/`.
2. Follow the [Agent Skills open standard](https://agentskills.io/specification) for cross-tool portability.
3. Follow the [Claude Code skills documentation](https://docs.anthropic.com/en/docs/claude-code/skills) for
   Claude-specific discovery.
4. Cover all major Atmos subsystems with accurate, source-derived content.
5. Enable external Atmos users to reference the skills from their own projects.

## Audience

| Audience                   | How they access skills                                                                  |
|----------------------------|-----------------------------------------------------------------------------------------|
| **Atmos contributors**     | Auto-discovered when working in the Atmos repo (`.claude/skills/` is project-level)     |
| **Atmos users** (external) | Reference via `--add-dir`, plugin installation, or Git clone pointing to the Atmos repo |

## Placement: `.claude/skills/`

### Why `.claude/skills/`

- **Canonical location**: Claude Code auto-discovers skills at `.claude/skills/<skill-name>/SKILL.md`.
- **Already exists**: The Atmos repo has an empty `.claude/skills/` directory.
- **Consistent**: Sits alongside `.claude/agents/` (9 existing agents) and `.claude/settings.local.json`.
- **Auto-discovery**: Contributors get skills automatically when they clone the repo.
- **External access**: Users can reference via `--add-dir` or clone the repo as a skill source.

### Alternative considered: separate `cloudposse/agent-skills` repo

HashiCorp and Pulumi use dedicated repos (`hashicorp/agent-skills`, `pulumi/agent-skills`).
This works for product-agnostic guidance but adds a separate installation step. Since Atmos
skills are tightly coupled to Atmos versions, co-locating them in the Atmos repo keeps skills
in sync with the codebase and avoids version drift.

A separate `cloudposse/agent-skills` repo can still exist as a thin wrapper that references or
re-exports skills from the Atmos repo if marketplace distribution is needed later.

## Standards

### Agent Skills Open Standard (agentskills.io/specification)

The [Agent Skills open standard](https://agentskills.io/specification) defines a cross-tool
format for packaging AI agent knowledge. All Atmos skills MUST comply.

#### Skill Directory Structure

```text
<skill-name>/
  SKILL.md          # Required: YAML frontmatter + markdown body
  references/       # Optional: additional documentation loaded on demand
  scripts/          # Optional: executable code
  assets/           # Optional: templates, images, data files
```

#### SKILL.md Format

```markdown
---
name: skill-name
description: >-
  Action-oriented description of when to use this skill.
  Max 1024 characters.
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Skill Title

Markdown body with instructions, examples, and references.
```

#### Naming Rules

| Rule            | Requirement                                   |
|-----------------|-----------------------------------------------|
| `name` field    | Required, max 64 characters                   |
| Character set   | Lowercase letters, numbers, hyphens only      |
| Directory match | `name` field MUST match parent directory name |
| Prefix          | Use `atmos-` prefix for all Atmos skills      |

#### Size Limits

| Constraint             | Limit                     | Rationale                             |
|------------------------|---------------------------|---------------------------------------|
| SKILL.md body          | < 500 lines               | Keeps agent context window manageable |
| SKILL.md instructions  | < 5000 tokens recommended | Progressive disclosure                |
| Metadata (frontmatter) | ~100 tokens               | Loaded at startup for all skills      |
| Reference files        | No hard limit             | Loaded on demand only                 |

#### Progressive Disclosure

The standard enforces a three-tier loading model:

1. **Tier 1 (always loaded)**: Frontmatter metadata (~100 tokens per skill).
2. **Tier 2 (on activation)**: Full SKILL.md body (< 5000 tokens).
3. **Tier 3 (on demand)**: Files in `references/`, `scripts/`, `assets/` (loaded only when explicitly referenced).

This prevents context window bloat when many skills are installed.

#### Reference Files

- Must be direct children of `references/` (one level deep).
- Must be linked from SKILL.md so the agent knows they exist.
- Use for deep-dive content that exceeds the 500-line SKILL.md limit.
- Format: Markdown (`.md`).

### Claude Code Skills Discovery

The [Claude Code skills documentation](https://docs.anthropic.com/en/docs/claude-code/skills)
defines how skills are discovered and loaded.

#### Discovery Hierarchy

| Level      | Path                                     | Scope                     | Priority |
|------------|------------------------------------------|---------------------------|----------|
| Enterprise | Managed settings                         | All users in organization | Highest  |
| Personal   | `~/.claude/skills/<skill-name>/SKILL.md` | All your projects         | High     |
| Project    | `.claude/skills/<skill-name>/SKILL.md`   | This project only         | Medium   |
| Plugin     | `<plugin>/skills/<skill-name>/SKILL.md`  | Where plugin is enabled   | Lowest   |

Skills at higher priority levels override same-named skills at lower levels.

#### Monorepo Support

When editing files in subdirectories, Claude Code discovers skills from nested
`.claude/skills/` directories. For example, `packages/frontend/.claude/skills/` is
discovered when editing files under `packages/frontend/`.

#### `--add-dir` Support

Skills in `.claude/skills/` within directories added via `--add-dir` are loaded
automatically with live change detection. This is how external Atmos users can
reference skills from the Atmos repo:

```bash
claude --add-dir /path/to/atmos-repo
```

#### Plugin Distribution

For marketplace distribution, skills can be packaged as a Claude Code plugin:

```text
.claude-plugin/
  marketplace.json       # Plugin registry manifest
<product>/
  <plugin-name>/
    .claude-plugin/
      plugin.json        # Plugin metadata
    skills/
      <skill-name>/
        SKILL.md
        references/
```

Users install via:

```bash
claude plugin marketplace add cloudposse/agent-skills
claude plugin install atmos-configuration@cloudposse
```

### AGENTS.md Convention

Based on [Vercel's research](https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals),
agents failed to trigger skills in 56% of cases without explicit routing instructions.
An always-present `AGENTS.md` file acting as a **skill-activation router** achieved 100%
activation rate.

#### AGENTS.md Structure

| Section         | Purpose                     | Size            |
|-----------------|-----------------------------|-----------------|
| Core Concepts   | Minimal always-on context   | ~120 words      |
| Key Commands    | Essential CLI reference     | ~10 commands    |
| Skill Index     | Task-to-skill routing table | 1 row per skill |
| Common Patterns | Frequently needed patterns  | ~100 words      |

#### Design Principles

1. **Compact**: ~2KB total; enough context to route correctly.
2. **Routing, not content**: Points to SKILL.md files for details.
3. **Retrieval-first**: Instructs agent to prefer skill content over training data.
4. **No human-facing content**: Installation docs belong in README.md, not AGENTS.md.

## Skill Inventory

### 3 Plugins, 9 Skills

| # | Skill                   | Plugin        | Description                                                                                             |
|---|-------------------------|---------------|---------------------------------------------------------------------------------------------------------|
| 1 | `atmos-stacks`          | configuration | Stack YAML, imports, inheritance, deep merging, vars, settings, locals, metadata, overrides, atmos.yaml |
| 2 | `atmos-components`      | configuration | Terraform root modules, abstract components, inheritance, versioning, mixins, catalog patterns          |
| 3 | `atmos-vendoring`       | configuration | vendor.yaml manifests, pulling from Git/S3/HTTP/OCI/Terraform Registry                                  |
| 4 | `atmos-terraform`       | orchestration | plan/apply/deploy, workspace management, backend config, varfile generation, authentication             |
| 5 | `atmos-workflows`       | orchestration | Multi-step workflows, Go template support, cross-component orchestration                                |
| 6 | `atmos-custom-commands` | orchestration | Custom CLI commands in atmos.yaml, arguments, flags, steps, env vars                                    |
| 7 | `atmos-gitops`          | integrations  | GitHub Actions, Spacelift, Atlantis, `atmos describe affected`, PR-based plan/apply                     |
| 8 | `atmos-validation`      | integrations  | OPA/Rego policies, JSON Schema, CUE validation, schema manifests                                        |
| 9 | `atmos-templates`       | integrations  | Go templates, Sprig/Gomplate functions, YAML functions, store integration                               |

### Content Sources

All SKILL.md content MUST be derived from the Atmos source documentation:

| Source Path                                    | Content                                                                   |
|------------------------------------------------|---------------------------------------------------------------------------|
| `website/docs/stacks/`                         | Stack configuration, imports, vars, locals, settings, metadata, overrides |
| `website/docs/components/`                     | Component types, root modules, backends, workspaces                       |
| `website/docs/vendor/`                         | Vendor manifests, source types, URL syntax                                |
| `website/docs/cli/commands/terraform/`         | All terraform subcommands                                                 |
| `website/docs/workflows/`                      | Workflow syntax and usage                                                 |
| `website/docs/cli/configuration/commands.mdx`  | Custom command definition                                                 |
| `website/docs/integrations/github-actions/`    | GitHub Actions workflows                                                  |
| `website/docs/cli/configuration/integrations/` | Spacelift, Atlantis config                                                |
| `website/docs/validation/`                     | OPA, JSON Schema, CUE validation                                          |
| `website/docs/templates/`                      | Template system overview                                                  |
| `website/docs/functions/yaml/`                 | YAML functions                                                            |
| `website/docs/functions/template/`             | Go template functions                                                     |
| `website/docs/design-patterns/`                | Best practices and patterns                                               |

## Proposed Directory Layout

```text
.claude/
├── agents/                              # Existing agents (9 files)
├── skills/
│   ├── AGENTS.md                        # Skill-activation router
│   ├── README.md                        # Skills overview and usage
│   │
│   ├── atmos-stacks/
│   │   ├── SKILL.md
│   │   └── references/
│   │       ├── import-patterns.md
│   │       └── inheritance-deep-merge.md
│   │
│   ├── atmos-components/
│   │   ├── SKILL.md
│   │   └── references/
│   │       ├── component-types.md
│   │       └── examples.md
│   │
│   ├── atmos-vendoring/
│   │   ├── SKILL.md
│   │   └── references/
│   │       └── vendor-manifest.md
│   │
│   ├── atmos-terraform/
│   │   ├── SKILL.md
│   │   └── references/
│   │       ├── commands-reference.md
│   │       └── backend-configuration.md
│   │
│   ├── atmos-workflows/
│   │   ├── SKILL.md
│   │   └── references/
│   │       └── workflow-syntax.md
│   │
│   ├── atmos-custom-commands/
│   │   ├── SKILL.md
│   │   └── references/
│   │       └── command-syntax.md
│   │
│   ├── atmos-gitops/
│   │   ├── SKILL.md
│   │   └── references/
│   │       ├── github-actions.md
│   │       └── spacelift.md
│   │
│   ├── atmos-validation/
│   │   ├── SKILL.md
│   │   └── references/
│   │       ├── opa-policies.md
│   │       └── json-schema.md
│   │
│   └── atmos-templates/
│       ├── SKILL.md
│       └── references/
│           ├── go-templates.md
│           └── yaml-functions-reference.md
│
└── settings.local.json                  # Existing settings
```

**Total: 28 new files** (9 SKILL.md + 15 references + AGENTS.md + README.md + 2 empty dirs created implicitly)

## Implementation Plan

### Phase 1: Copy and Adapt Skills

1. Copy the 9 SKILL.md files and 15 reference files from the external `agent-skills` repo.
2. Update `!aws.organization_id` YAML function coverage in `atmos-templates` and `yaml-functions-reference.md`.
3. Verify all SKILL.md files are under 500 lines.
4. Verify all `name` fields match directory names.
5. Add AGENTS.md skill-activation router.
6. Add README.md with usage instructions.

### Phase 2: Validation

1. Adapt `scripts/validate-structure.sh` to validate the `.claude/skills/` layout.
2. Verify all reference files are linked from their SKILL.md.
3. Run validation in CI.

### Phase 3: Documentation

1. Add Docusaurus documentation page explaining Atmos skills.
2. Update CLAUDE.md to reference the skills.
3. Add blog post announcing the feature.

## SKILL.md Writing Guidelines

### Content Principles

1. **Source-derived**: All content MUST come from Atmos documentation, not invented.
2. **Action-oriented**: Describe what the user can DO, not abstract theory.
3. **Example-heavy**: Include concrete YAML/HCL examples for every concept.
4. **Progressive**: Put the most common patterns first; edge cases in references.
5. **Cross-referenced**: Link to reference files for deep content.

### Structure Template

```markdown
---
name: atmos-<name>
description: >-
  <What this skill covers and when to activate it.>
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# <Skill Title>

## Overview

Brief description of the subsystem.

## <Core Concept 1>

Most important pattern with examples.

## <Core Concept 2>

Second most important pattern.

## <Additional Sections>

...

## Common Patterns

Frequently needed recipes.

## Best Practices

Do's and don'ts.

## Reference Files

- [reference-name.md](references/reference-name.md) -- Description of what it covers
```

### What NOT to Include

- Installation instructions (belong in README.md).
- Atmos CLI installation steps.
- Generic Terraform guidance (covered by `hashicorp/agent-skills`).
- Version-specific changelog content.
- Internal implementation details of Atmos source code.

## Relationship to Existing `.claude/agents/`

| Concern      | Agents (`.claude/agents/`)                      | Skills (`.claude/skills/`)                           |
|--------------|-------------------------------------------------|------------------------------------------------------|
| **Audience** | Atmos contributors/developers                   | Atmos users (and contributors)                       |
| **Content**  | Codebase patterns, architecture, implementation | User-facing configuration, orchestration, workflows  |
| **Examples** | `flag-handler`, `tui-expert`, `atmos-errors`    | `atmos-stacks`, `atmos-terraform`, `atmos-templates` |
| **Scope**    | How to develop Atmos                            | How to use Atmos                                     |

Agents and skills are complementary. Agents help developers build Atmos; skills help
users configure and operate Atmos.

## References

### Standards and Specifications

- [Agent Skills Open Standard](https://agentskills.io/specification) -- Cross-tool skill format specification
- [Claude Code Skills Documentation](https://docs.anthropic.com/en/docs/claude-code/skills) -- Claude Code skill discovery and loading
- [OpenAI Codex Skills](https://developers.openai.com/codex/skills/) -- Codex-compatible skill format (compatible with Agent Skills standard)

### Industry Examples

- [HashiCorp Agent Skills](https://github.com/hashicorp/agent-skills) -- Terraform/Packer skills in a dedicated repo
- [Pulumi Agent Skills](https://github.com/pulumi/agent-skills) -- Pulumi IaC skills
- [Anthropic Skills](https://github.com/anthropics/skills) -- Reference skill implementations (PDF, DOCX, PPTX, XLSX)
- [Introducing HashiCorp Agent Skills](https://www.hashicorp.com/en/blog/introducing-hashicorp-agent-skills) -- Blog post on the HashiCorp approach

### Research

- [Vercel: AGENTS.md Outperforms Skills](https://vercel.com/blog/agents-md-outperforms-skills-in-our-agent-evals) --
  Research showing always-present AGENTS.md achieves 100% skill activation vs 44% without routing
- [Agent Skills vs Rules vs Commands](https://www.builder.io/blog/agent-skills-rules-commands) -- Comparison of agent knowledge formats
- [Implementing CLAUDE.md and Agent Skills](https://www.groff.dev/blog/implementing-claude-md-agent-skills) -- Practical guide to skills in product repos

### Atmos Documentation

- [Atmos Documentation](https://atmos.tools) -- Primary source for all skill content
- [Atmos GitHub](https://github.com/cloudposse/atmos) -- Source repository
- [Cloud Posse GitHub](https://github.com/cloudposse) -- Organization
