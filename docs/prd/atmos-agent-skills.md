# PRD: Atmos Agent Skills

## Status: Implemented

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
3. Skills must be co-located with the Atmos codebase to stay in sync with the codebase and avoid version drift.

## Goals

1. Ship Atmos skills inside the Atmos repository at `agent-skills/`.
2. Follow the [Agent Skills open standard](https://agentskills.io/specification) for cross-tool portability.
3. Support all major AI tools (Claude Code, GitHub Copilot, OpenAI Codex, Gemini, Grok, etc.).
4. Cover all major Atmos subsystems with accurate, source-derived content.
5. Enable external Atmos users to reference the skills from their own projects.

## Audience

| Audience                   | How they access skills                                                                  |
|----------------------------|-----------------------------------------------------------------------------------------|
| **Atmos contributors**     | Auto-discovered when working in the Atmos repo                                          |
| **Atmos users** (external) | Reference via `--add-dir`, plugin installation, or Git clone pointing to the Atmos repo |

## Placement: `agent-skills/`

### Why `agent-skills/`

- **Tool-agnostic**: Not tied to any specific AI tool (Claude, Copilot, Codex, Gemini, Grok, etc.).
- **Industry convention**: Follows the naming used by HashiCorp and Pulumi for their agent skills.
- **Visible**: Top-level directory is easily discoverable by humans and AI tools alike.
- **Standards-compliant**: Follows the [Agent Skills open standard](https://agentskills.io/specification).
- **Co-located**: Skills stay in sync with the Atmos codebase, avoiding version drift.

### Claude Code Auto-Discovery

A symlink at `.claude/skills/ -> ../agent-skills/` enables Claude Code to auto-discover skills
at the canonical `.claude/skills/<skill-name>/SKILL.md` path. Other AI tools can reference
`agent-skills/` directly.

### Alternative considered: separate dedicated repo

HashiCorp and Pulumi use dedicated repos for agent skills.
This works for product-agnostic guidance but adds a separate installation step. Since Atmos
skills are tightly coupled to Atmos versions, co-locating them in the Atmos repo keeps skills
in sync with the codebase and avoids version drift.

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
claude plugin marketplace add <org>/<plugin-name>
claude plugin install <skill-name>@<org>
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

### 16 Skills

| #  | Skill                    | Category      | Description                                                                                             |
|----|--------------------------|---------------|---------------------------------------------------------------------------------------------------------|
| 1  | `atmos-stacks`           | configuration | Stack YAML, imports, inheritance, deep merging, vars, settings, locals, metadata, overrides, atmos.yaml |
| 2  | `atmos-components`       | configuration | Terraform root modules, abstract components, inheritance, versioning, mixins, catalog patterns          |
| 3  | `atmos-vendoring`        | configuration | vendor.yaml manifests, pulling from Git/S3/HTTP/OCI/Terraform Registry                                 |
| 4  | `atmos-terraform`        | orchestration | plan/apply/deploy, workspace management, backend config, varfile generation                             |
| 5  | `atmos-helmfile`         | orchestration | sync/apply/destroy/diff, Kubernetes deployments, EKS integration, varfile generation                    |
| 6  | `atmos-packer`           | orchestration | init/build/validate/inspect/output, machine image building, template management                         |
| 7  | `atmos-ansible`          | orchestration | Playbook execution, variable passing, inventory management, configuration management                    |
| 8  | `atmos-workflows`        | orchestration | Multi-step workflows, Go template support, cross-component orchestration                                |
| 9  | `atmos-custom-commands`  | orchestration | Custom CLI commands in atmos.yaml, arguments, flags, steps, env vars                                    |
| 10 | `atmos-auth`             | platform      | Providers (SSO/SAML/OIDC/GCP), identities (AWS/Azure/GCP), keyring, identity chaining, login/exec/shell |
| 11 | `atmos-stores`           | platform      | AWS SSM, Azure Key Vault, GCP Secret Manager, Redis, Artifactory, hooks integration, data sharing       |
| 12 | `atmos-schemas`          | platform      | JSON Schema for stack manifests, IDE auto-completion, schema updates for new features, validation        |
| 13 | `atmos-gitops`           | integrations  | GitHub Actions, Spacelift, Atlantis, `atmos describe affected`, PR-based plan/apply                     |
| 14 | `atmos-validation`       | integrations  | OPA/Rego policies, JSON Schema, CUE validation, schema manifests                                        |
| 15 | `atmos-templates`        | integrations  | Go templates, Sprig/Gomplate functions, YAML functions, store integration                               |
| 16 | `atmos-design-patterns`  | guidance      | Stack organization, component catalogs, inheritance, configuration composition, version management       |

### Content Sources

All SKILL.md content MUST be derived from the Atmos source documentation:

| Source Path                                    | Content                                                                   |
|------------------------------------------------|---------------------------------------------------------------------------|
| `website/docs/stacks/`                         | Stack configuration, imports, vars, locals, settings, metadata, overrides |
| `website/docs/components/`                     | Component types (Terraform, Helmfile, Packer, Ansible)                    |
| `website/docs/vendor/`                         | Vendor manifests, source types, URL syntax                                |
| `website/docs/cli/commands/terraform/`         | All terraform subcommands                                                 |
| `website/docs/cli/commands/helmfile/`          | All helmfile subcommands, source management                               |
| `website/docs/cli/commands/packer/`            | All packer subcommands, source management                                 |
| `website/docs/cli/commands/ansible/`           | Ansible playbook command, variable passing                                |
| `website/docs/workflows/`                      | Workflow syntax and usage                                                 |
| `website/docs/cli/configuration/commands.mdx`  | Custom command definition                                                 |
| `website/docs/cli/configuration/auth/`         | Auth providers, identities, keyring, logs                                 |
| `website/docs/cli/commands/auth/`              | Auth commands (login, exec, shell, console, etc.)                         |
| `website/docs/cli/configuration/stores.mdx`    | Store backends configuration                                              |
| `website/docs/cli/configuration/schemas.mdx`   | JSON Schema configuration, IDE integration                                |
| `website/static/schemas/`                      | Atmos manifest JSON Schema files                                          |
| `pkg/datafetcher/schema/`                      | Embedded schema files (stack-config, vendor, manifest)                    |
| `website/docs/integrations/github-actions/`    | GitHub Actions workflows                                                  |
| `website/docs/cli/configuration/integrations/` | Spacelift, Atlantis config                                                |
| `website/docs/validation/`                     | OPA, JSON Schema, CUE validation                                          |
| `website/docs/templates/`                      | Template system overview                                                  |
| `website/docs/functions/yaml/`                 | YAML functions                                                            |
| `website/docs/functions/template/`             | Go template functions                                                     |
| `website/docs/design-patterns/`                | Best practices and patterns                                               |

## Directory Layout

```text
agent-skills/                                # Tool-agnostic skills (primary location)
├── AGENTS.md                                # Skill-activation router
│
├── atmos-stacks/                            # Configuration
│   ├── SKILL.md
│   └── references/
│       ├── import-patterns.md
│       └── inheritance-deep-merge.md
│
├── atmos-components/
│   ├── SKILL.md
│   └── references/
│       ├── component-types.md
│       └── examples.md
│
├── atmos-vendoring/
│   ├── SKILL.md
│   └── references/
│       └── vendor-manifest.md
│
├── atmos-terraform/                         # Orchestration
│   ├── SKILL.md
│   └── references/
│       ├── commands-reference.md
│       └── backend-configuration.md
│
├── atmos-helmfile/
│   ├── SKILL.md
│   └── references/
│       └── commands-reference.md
│
├── atmos-packer/
│   ├── SKILL.md
│   └── references/
│       └── commands-reference.md
│
├── atmos-ansible/
│   ├── SKILL.md
│   └── references/
│       └── commands-reference.md
│
├── atmos-workflows/
│   ├── SKILL.md
│   └── references/
│       └── workflow-syntax.md
│
├── atmos-custom-commands/
│   ├── SKILL.md
│   └── references/
│       └── command-syntax.md
│
├── atmos-auth/                              # Platform
│   ├── SKILL.md
│   └── references/
│       ├── providers-and-identities.md
│       └── commands-reference.md
│
├── atmos-stores/
│   ├── SKILL.md
│   └── references/
│       └── store-providers.md
│
├── atmos-schemas/
│   ├── SKILL.md
│   └── references/
│       └── schema-structure.md
│
├── atmos-gitops/                            # Integrations
│   ├── SKILL.md
│   └── references/
│       ├── github-actions.md
│       └── spacelift.md
│
├── atmos-validation/
│   ├── SKILL.md
│   └── references/
│       ├── opa-policies.md
│       └── json-schema.md
│
├── atmos-templates/
│   ├── SKILL.md
│   └── references/
│       ├── go-templates.md
│       └── yaml-functions-reference.md
│
└── atmos-design-patterns/                   # Guidance
    ├── SKILL.md
    └── references/
        ├── stack-organization.md
        └── version-management.md

.claude/
├── agents/                                  # Existing agents (9 files)
├── skills -> ../agent-skills                # Symlink for Claude Code auto-discovery
└── settings.local.json                      # Existing settings
```

**Total: 41 files** (16 SKILL.md + 24 references + AGENTS.md) + 1 symlink

## Implementation Plan

### Phase 1: Initial Skills (Completed)

1. Created 9 SKILL.md files and 15 reference files for core subsystems.
2. Updated `!aws.organization_id` YAML function coverage in `atmos-templates` and `yaml-functions-reference.md`.
3. Verified all SKILL.md files are under 500 lines.
4. Verified all `name` fields match directory names.
5. Added AGENTS.md skill-activation router.

### Phase 2: Expanded Coverage (Completed)

1. Added `atmos-helmfile` skill for Kubernetes deployment orchestration.
2. Added `atmos-packer` skill for machine image building.
3. Added `atmos-ansible` skill for configuration management.
4. Added `atmos-auth` skill for authentication and identity management.
5. Added `atmos-stores` skill for external key-value store backends.
6. Added `atmos-schemas` skill for JSON Schema system and how to update schemas.
7. Added `atmos-design-patterns` skill for architectural patterns and best practices.
8. Updated AGENTS.md with all 16 skills.

### Phase 3: Validation (Planned)

1. Add validation script to check `.claude/skills/` layout (frontmatter, name/dir match, line limits).
2. Verify all reference files are linked from their SKILL.md.
3. Run validation in CI.

### Phase 4: Documentation (Planned)

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
