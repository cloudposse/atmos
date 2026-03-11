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

| Audience                   | How they access skills                                                                                |
|----------------------------|-------------------------------------------------------------------------------------------------------|
| **Atmos contributors**     | Auto-discovered via `.claude/skills` symlinks when working in the Atmos repo                          |
| **Atmos users** (external) | Install via Claude Code plugin marketplace, or copy `agent-skills/` from the Atmos repo for other tools |

## Placement: `agent-skills/`

### Why `agent-skills/`

- **Tool-agnostic**: Not tied to any specific AI tool (Claude, Copilot, Codex, Gemini, Grok, etc.).
- **Industry convention**: Follows the naming used by HashiCorp and Pulumi for their agent skills.
- **Visible**: Top-level directory is easily discoverable by humans and AI tools alike.
- **Standards-compliant**: Follows the [Agent Skills open standard](https://agentskills.io/specification).
- **Co-located**: Skills stay in sync with the Atmos codebase, avoiding version drift.

### Claude Code Auto-Discovery

The `.claude/skills/` directory contains individual symlinks (one per skill) that point into the
flat plugin structure:

```text
.claude/skills/
  atmos-config -> ../../agent-skills/skills/atmos-config
  atmos-stacks -> ../../agent-skills/skills/atmos-stacks
  atmos-terraform -> ../../agent-skills/skills/atmos-terraform
  ...  # 21 symlinks total
```

Claude Code discovers skills at `.claude/skills/<skill-name>/SKILL.md` -- it expects a flat
directory of skill folders. The symlinks point directly into the flat `agent-skills/skills/`
directory. Both paths resolve to the same 21 `SKILL.md` files. Other AI tools reference
`agent-skills/` directly via `AGENTS.md`.

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

The [Claude Code skills documentation](https://code.claude.com/docs/en/skills)
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

Claude Code supports distributing skills as plugins via a marketplace system. The Atmos repo
serves as a Claude Code marketplace, enabling users to install Atmos skills in their own projects
with a single command.

##### Architecture Decision

All 21 skills are shipped as a **single plugin** named `atmos` in a flat `skills/` directory.
This simplifies installation (one command), discovery, and maintenance. Users install one
plugin and get all Atmos skills.

| Plugin | Skills | Description |
|--------|--------|-------------|
| `atmos` | 21 | All Atmos skills: terraform, helmfile, packer, ansible, workflows, custom-commands, config, schemas, introspection, auth, stores, toolchain, devcontainer, stacks, components, vendoring, validation, gitops, yaml-functions, templates, design-patterns |

##### Structural Requirement

Claude Code requires plugin skills at `<plugin>/skills/<skill-name>/SKILL.md`. The structure
uses a single plugin with one `skills/` directory containing all 21 skill subdirectories.

##### Directory Layout

```text
.claude-plugin/
  marketplace.json                         # Marketplace manifest (repo root)

agent-skills/                              # Single plugin: atmos
  AGENTS.md                                # Skill-activation router
  .claude-plugin/
    plugin.json                            # Plugin manifest
  skills/
    atmos-terraform/SKILL.md
    atmos-helmfile/SKILL.md
    atmos-packer/SKILL.md
    atmos-ansible/SKILL.md
    atmos-workflows/SKILL.md
    atmos-custom-commands/SKILL.md
    atmos-config/SKILL.md
    atmos-introspection/SKILL.md
    atmos-auth/SKILL.md
    atmos-stores/SKILL.md
    atmos-toolchain/SKILL.md
    atmos-devcontainer/SKILL.md
    atmos-stacks/SKILL.md
    atmos-components/SKILL.md
    atmos-vendoring/SKILL.md
    atmos-validation/SKILL.md
    atmos-schemas/SKILL.md
    atmos-gitops/SKILL.md
    atmos-yaml-functions/SKILL.md
    atmos-templates/SKILL.md
    atmos-design-patterns/SKILL.md
```

##### Marketplace Manifest

File: `.claude-plugin/marketplace.json` (repo root)

```json
{
  "name": "cloudposse",
  "version": "1.0.0",
  "description": "Official Atmos plugins and skills for Claude Code",
  "owner": {
    "name": "Cloud Posse",
    "email": "opensource@cloudposse.com"
  },
  "plugins": [
    {
      "name": "atmos",
      "source": "./agent-skills",
      "description": "Atmos skills for AI coding assistants: Terraform/Helmfile/Packer/Ansible orchestration, stack configuration, components, vendoring, validation, YAML functions, Go templates, authentication, stores, workflows, and design patterns.",
      "category": "development"
    }
  ]
}
```

The `source` field points to the directory containing the plugin's `.claude-plugin/plugin.json`.
Plugin-level metadata (`author`, `homepage`, `repository`, `license`, `keywords`) belongs in the
plugin manifest (`agent-skills/.claude-plugin/plugin.json`), not the marketplace manifest.

##### Plugin Manifest

The single plugin directory contains `agent-skills/.claude-plugin/plugin.json`:

```json
{
  "name": "atmos",
  "version": "1.0.0",
  "description": "Atmos skills for AI coding assistants: Terraform/Helmfile/Packer/Ansible orchestration, stack configuration, components, vendoring, validation, YAML functions, Go templates, authentication, stores, workflows, and design patterns.",
  "author": {
    "name": "Cloud Posse",
    "url": "https://github.com/cloudposse"
  },
  "homepage": "https://atmos.tools/integrations/ai/agent-skills",
  "repository": "https://github.com/cloudposse/atmos",
  "license": "Apache-2.0",
  "keywords": ["atmos", "terraform", "infrastructure", "iac", "devops", "cloud"]
}
```

##### User Installation

```bash
# Step 1: Add the Cloud Posse marketplace (one-time setup)
/plugin marketplace add cloudposse/atmos

# Step 2: Install the Atmos plugin (all 21 skills)
/plugin install atmos@cloudposse
```

A single install command provides all 21 Atmos skills.

##### Uninstalling

```bash
# Remove the plugin
/plugin uninstall atmos@cloudposse

# Remove the marketplace (optional)
/plugin marketplace remove cloudposse
```

##### Team Auto-Discovery

Teams can auto-configure the marketplace and plugin for all team members by adding
to the project's `.claude/settings.json`:

```json
{
  "extraKnownMarketplaces": {
    "cloudposse": {
      "source": {
        "source": "github",
        "repo": "cloudposse/atmos"
      }
    }
  },
  "enabledPlugins": {
    "atmos@cloudposse": true
  }
}
```

When team members trust the repository folder, Claude Code prompts them to install
the marketplace and plugin automatically.

##### Claude Code Symlink (Contributor Use)

For contributors working directly in the Atmos repo, the `.claude/skills` symlinks
provide auto-discovery without plugin installation. Each symlink points directly into
the flat `agent-skills/skills/` directory:

```bash
# .claude/skills/<skill-name> -> ../../agent-skills/skills/<skill-name>
# Claude Code auto-discovers skills at .claude/skills/<skill-name>/SKILL.md
```

##### Plugin Caching

When the plugin is installed, Claude Code copies the `agent-skills/` directory to
`~/.claude/plugins/cache`. The `references/` subdirectories within each skill
are included in the copy. The `AGENTS.md` router file is included since it lives
inside the plugin directory. AI tools load `AGENTS.md` when working directly in the
Atmos repo via the `.claude/skills` symlink or when the plugin is installed.

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

## Distribution and Usage

### How Cloud Posse Publishes Skills

Merging the PR is the only publishing step for Claude Code. There is no registration, approval,
or upload to any central registry.

**How it works**: The `.claude-plugin/marketplace.json` file at the repo root turns the
`cloudposse/atmos` GitHub repository into a Claude Code marketplace. When a user runs
`/plugin marketplace add cloudposse/atmos`, Claude Code fetches the `marketplace.json` directly
from the GitHub repo via the GitHub API. It reads the single plugin entry and its source. When the
user installs the plugin, Claude Code downloads the `agent-skills/` directory from the repo and
caches it locally at `~/.claude/plugins/cache`. The GitHub repo IS the marketplace -- no
intermediate service or registry is involved.

**User discovery**: Users must know the repo path `cloudposse/atmos` to add the marketplace.
They learn this from Atmos documentation, blog posts, and the README.

**Optional: official Anthropic marketplace**: Cloud Posse can optionally submit plugins to the
official Anthropic marketplace ([claude-plugins-official](https://github.com/anthropics/claude-plugins-official))
via the [plugin directory submission form](https://code.claude.com/docs/en/plugins#submit-your-plugin-to-the-official-marketplace). Submitted
plugins are reviewed for quality and security standards. If accepted, they appear in the
`/external_plugins` directory of the official marketplace and become discoverable via the
"Discover" tab inside Claude Code's `/plugin` UI for all users worldwide -- no marketplace setup
needed. Users install with `/plugin install atmos@claude-plugin-directory`.
This is a separate, optional step that provides broader discovery but is not required for the
self-hosted marketplace to function.

For other AI tools (Gemini CLI, Codex, Cursor, Windsurf, Copilot), the skills are distributed
as files in the public GitHub repository. There is no marketplace registration for these tools --
users access the files directly from the repo.

### How Users Install Skills

Users install the `atmos` binary -- they do NOT clone the Atmos repository. Skills are installed
separately through each AI tool's own mechanism.

| AI Tool | Marketplace? | Installation Method |
|---------|-------------|---------------------|
| **Claude Code** | Yes (third-party) | `/plugin marketplace add cloudposse/atmos` then `/plugin install atmos@cloudposse` |
| **Cursor** | Yes (curated) | Cloud Posse would submit to [cursor.com/marketplace](https://cursor.com/marketplace) (separate process, requires review) |
| **Gemini CLI** | No | Vendor skills with `atmos vendor pull`, symlink to `.gemini/skills/` |
| **GitHub Copilot** | No (requested) | Vendor skills with `atmos vendor pull`, reference from `.github/copilot-instructions.md` |
| **OpenAI Codex** | No | Vendor skills with `atmos vendor pull`, copy `AGENTS.md` to the project root |
| **Windsurf** | No | Vendor skills with `atmos vendor pull`, reference `AGENTS.md` from `.windsurfrules` |

For tools without marketplaces, users vendor the `agent-skills/` directory from the Atmos GitHub
repo into their infrastructure project using Atmos vendoring:

```yaml
# Add to vendor.yaml
apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: atmos-agent-skills
  description: Vendor Atmos AI agent skills
spec:
  sources:
    - component: "agent-skills"
      source: "github.com/cloudposse/atmos.git//agent-skills?ref={{.Version}}"
      version: "main"
      targets:
        - "agent-skills"
```

```bash
atmos vendor pull --component agent-skills
```

### How Skills Are Activated

Users do NOT invoke skills explicitly. Skills activate automatically based on context:

1. **Claude Code**: At session start, Claude reads the `description` field from each installed
   skill's SKILL.md frontmatter (~100 tokens per skill). When the user's question matches a
   skill's description, Claude loads the full SKILL.md body automatically. AGENTS.md is NOT
   needed for plugin-based activation -- it is useful only when all skills are co-located
   (e.g., working in the Atmos repo via `.claude/skills`).

2. **Gemini CLI**: Gemini reads skill metadata at startup, matches user tasks to skill
   descriptions, and prompts the user to confirm activation before loading instructions.

3. **Cursor**: Skills can be configured as "Always", "Agent Decides", or "Manual". In
   "Agent Decides" mode, Cursor activates skills based on context (similar to Claude Code).

4. **Other tools**: Tools that read `AGENTS.md` (Codex, Windsurf, Copilot) use the skill
   index table in AGENTS.md to identify and load the relevant SKILL.md file.

### Role of AGENTS.md vs. SKILL.md Frontmatter

| Mechanism | When Used | Activation |
|-----------|-----------|------------|
| **SKILL.md frontmatter** (`description`) | Plugin-based installation (Claude Code, Cursor) | AI matches user question to skill description automatically |
| **AGENTS.md** skill index table | File-based installation (Codex, Windsurf, Copilot, Gemini) and contributor auto-discovery | AI reads the routing table and loads the matching SKILL.md |

Both mechanisms achieve the same result: the AI loads the right skill without the user having
to specify which one. The SKILL.md frontmatter is the primary activation mechanism for
marketplace/plugin installations. AGENTS.md is the primary mechanism for file-based installations
and serves as a fallback router when all skills are co-located.

## Skill Inventory

### 21 Skills

All skills live in `agent-skills/skills/` as a flat list within the single `atmos` plugin.

| #  | Skill                    | Description                                                                                             |
|----|--------------------------|-------------------------------------------------------------------------------------------------------- |
| 1  | `atmos-terraform`        | plan/apply/deploy, workspace management, backend config, varfile generation                             |
| 2  | `atmos-helmfile`         | sync/apply/destroy/diff, Kubernetes deployments, EKS integration, varfile generation                    |
| 3  | `atmos-packer`           | init/build/validate/inspect/output, machine image building, template management                         |
| 4  | `atmos-ansible`          | Playbook execution, variable passing, inventory management, configuration management                    |
| 5  | `atmos-workflows`        | Multi-step workflows, Go template support, cross-component orchestration                                |
| 6  | `atmos-custom-commands`  | Custom CLI commands in atmos.yaml, arguments, flags, steps, env vars                                    |
| 7  | `atmos-config`           | Project configuration: atmos.yaml structure, all sections, discovery, merging, profiles                 |
| 8  | `atmos-schemas`          | JSON Schema for stack manifests, IDE auto-completion, schema updates for new features, validation        |
| 9  | `atmos-introspection`    | describe/list commands for querying stacks, components, dependencies, change impact, provenance          |
| 10 | `atmos-auth`             | Providers (SSO/SAML/OIDC/GCP), identities (AWS/Azure/GCP), keyring, identity chaining, login/exec/shell |
| 11 | `atmos-stores`           | AWS SSM, Azure Key Vault, GCP Secret Manager, Redis, Artifactory, hooks integration, data sharing       |
| 12 | `atmos-toolchain`        | CLI tool version management via Aqua registries, .tool-versions, install/exec/search                    |
| 13 | `atmos-devcontainer`     | Devcontainer management: start/stop/shell/exec, Docker/Podman, identity integration (experimental)      |
| 14 | `atmos-stacks`           | Stack YAML, imports, inheritance, deep merging, vars, settings, locals, metadata, overrides              |
| 15 | `atmos-components`       | Terraform root modules, abstract components, inheritance, versioning, mixins, catalog patterns          |
| 16 | `atmos-vendoring`        | vendor.yaml manifests, pulling from Git/S3/HTTP/OCI/Terraform Registry                                 |
| 17 | `atmos-validation`       | OPA/Rego policies, JSON Schema validation, schema manifests                                             |
| 18 | `atmos-gitops`           | GitHub Actions, Spacelift, Atlantis, `atmos describe affected`, PR-based plan/apply                     |
| 19 | `atmos-yaml-functions`   | YAML functions: !terraform.state, !terraform.output, !store, !env, !exec, !include, !aws.*, !literal   |
| 20 | `atmos-templates`        | Go templates, Sprig/Gomplate functions, atmos.Component, atmos.GomplateDatasource, template config      |
| 21 | `atmos-design-patterns`  | Stack organization, component catalogs, inheritance, configuration composition, version management       |

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
| `website/docs/validation/`                     | OPA, JSON Schema validation                                               |
| `website/docs/templates/`                      | Template system overview                                                  |
| `website/docs/functions/yaml/`                 | YAML functions                                                            |
| `website/docs/functions/template/`             | Go template functions                                                     |
| `website/docs/design-patterns/`                | Best practices and patterns                                               |
| `website/docs/cli/commands/toolchain/`         | Toolchain commands (install, exec, search, env, registry)                 |
| `website/docs/cli/configuration/toolchain/`    | Toolchain configuration (registries, aliases)                             |
| `website/docs/cli/commands/describe/`          | Describe commands (component, stacks, affected, dependents, config)       |
| `website/docs/cli/commands/list/`              | List commands (stacks, components, instances, affected, workflows)        |
| `docs/prd/devcontainer-command.md`             | Devcontainer feature specification                                        |
| `cmd/devcontainer/`                            | Devcontainer command implementations                                      |

## Directory Layout

### Current Layout (Flat single-plugin structure)

```text
.claude-plugin/
└── marketplace.json                         # Marketplace manifest (repo root)

agent-skills/                                # Single plugin: atmos
├── AGENTS.md                                # Skill-activation router
├── .claude-plugin/
│   └── plugin.json                          # Plugin manifest
└── skills/                                  # All 21 skills (flat)
    ├── atmos-terraform/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-helmfile/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-packer/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-ansible/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-workflows/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-custom-commands/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-config/
    │   ├── SKILL.md
    │   └── references/sections-reference.md
    ├── atmos-introspection/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-auth/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-stores/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-toolchain/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-devcontainer/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-stacks/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-components/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-vendoring/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-validation/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-schemas/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-gitops/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-yaml-functions/
    │   ├── SKILL.md
    │   └── references/
    ├── atmos-templates/
    │   ├── SKILL.md
    │   └── references/
    └── atmos-design-patterns/
        ├── SKILL.md
        └── references/

.claude/
├── agents/                                  # Existing agents (9 files)
├── skills/                                  # Symlinks for auto-discovery
│   ├── atmos-config -> ../../agent-skills/skills/atmos-config
│   ├── atmos-stacks -> ../../agent-skills/skills/atmos-stacks
│   └── ...                                  # One symlink per skill (21 total)
└── settings.local.json                      # Existing settings
```

**Total: 46 files** (21 SKILL.md + 21 references + AGENTS.md + 1 plugin.json + 1 marketplace.json) + 21 symlinks

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
8. Updated AGENTS.md with all 21 skills.

### Phase 3: Validation (Completed)

1. Added CI workflow (`.github/workflows/validate-agent-skills.yml`) to validate structure, frontmatter,
   line count (500 max), file sizes (20KB SKILL.md, 25KB references), and code fence language tags.
2. Verified all reference files are linked from their SKILL.md.
3. Validation runs on push/PR to `main` touching `agent-skills/**`.

### Phase 4: Documentation (Completed)

1. Added Docusaurus documentation page (`website/docs/integrations/ai/agent-skills.mdx`).
2. Added blog post announcing the feature (`website/blog/2026-02-27-ai-agent-skills.mdx`).

### Phase 5: Claude Code Plugin Distribution (Completed)

Restructure `agent-skills/` for Claude Code plugin compatibility and add marketplace manifests.

1. **Restructure directory layout**: Move all skill directories into a single flat
   `agent-skills/skills/` directory matching Claude Code plugin conventions.

   ```text
   # Before (original):
   agent-skills/atmos-config/SKILL.md

   # After (restructured):
   agent-skills/skills/atmos-config/SKILL.md
   ```

2. **Create marketplace manifest**: Add `.claude-plugin/marketplace.json` at the repo root
   with a single plugin entry (`atmos` with `"source": "./agent-skills"`).

3. **Create plugin manifest**: Add `agent-skills/.claude-plugin/plugin.json` for the single
   `atmos` plugin.

4. **Update `.claude/skills` symlinks**: Update symlinks to point to the flat
   `agent-skills/skills/<skill-name>` paths.

5. **Update CI workflow**: Update `.github/workflows/validate-agent-skills.yml` to validate
   the restructured directory layout.

6. **Update documentation**: Update the blog post, website doc, and this PRD to include
   plugin installation instructions.

7. **Validate**: Run `claude plugin validate .` from the repo root to verify marketplace
   and plugin manifests pass Claude Code's validation.

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
- [Claude Code Skills Documentation](https://code.claude.com/docs/en/skills) -- Claude Code skill discovery and loading
- [Claude Code Plugin Marketplaces](https://code.claude.com/docs/en/plugin-marketplaces) -- Creating and distributing plugin marketplaces
- [Claude Code Plugin Discovery](https://code.claude.com/docs/en/discover-plugins) -- Installing plugins from marketplaces
- [OpenAI Codex Skills](https://developers.openai.com/codex/skills/) -- Codex-compatible skill format (compatible with Agent Skills standard)

### Industry Examples

- [HashiCorp Agent Skills](https://github.com/hashicorp/agent-skills) -- Terraform/Packer skills in a dedicated repo
- [Pulumi Agent Skills](https://github.com/pulumi/agent-skills) -- Pulumi IaC skills
- [Anthropic Skills](https://github.com/anthropics/skills) -- Reference skill implementations (PDF, DOCX, PPTX, XLSX)
- [Official Anthropic Plugin Directory](https://github.com/anthropics/claude-plugins-official) -- Official Claude Code marketplace with reviewed plugins
- [Plugin Directory Submission Form](https://clau.de/plugin-directory-submission) -- Submit plugins for inclusion in the official Anthropic marketplace
- [antonbabenko/terraform-skill](https://github.com/antonbabenko/terraform-skill) -- Single-skill marketplace example
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
