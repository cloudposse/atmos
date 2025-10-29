# Agent Marketplace and Download System

**Status:** Design Phase
**Version:** 1.0
**Last Updated:** 2025-10-28

## Overview

This document describes the design for Phase 2 of the Atmos AI Agent system: enabling users to discover, download, install, and manage community-contributed agents from GitHub repositories and other sources.

## Goals

1. **Extensibility** - Allow community to contribute specialized agents without modifying Atmos core
2. **Discoverability** - Make it easy to find and install useful agents
3. **Security** - Ensure downloaded agents are safe and verified
4. **Simplicity** - Provide a streamlined CLI experience for agent management
5. **Compatibility** - Work seamlessly with existing embedded agents

## Non-Goals (Future Work)

- Agent rating/review system
- Automated agent testing infrastructure
- Agent analytics/telemetry
- Agent dependency resolution beyond basic checks
- GUI for agent marketplace

## User Stories

### As a User
- I want to browse available agents so I can discover new capabilities
- I want to install an agent from GitHub with a single command
- I want to update installed agents when new versions are available
- I want to remove agents I no longer need
- I want to see which agents are installed and their versions
- I want installed agents to appear in the agent switcher (Ctrl+A) alongside built-in agents

### As an Agent Developer
- I want to publish my agent by creating a GitHub repository
- I want to version my agent using Git tags
- I want to specify which tools my agent can access
- I want to provide clear documentation for users
- I want users to easily install my agent

## Architecture

### Agent Storage Structure

```
~/.atmos/
├── agents/
│   ├── registry.json              # Installed agents metadata
│   ├── github.com/
│   │   ├── username/
│   │   │   ├── agent-name/
│   │   │   │   ├── .agent.yaml    # Agent metadata
│   │   │   │   ├── prompt.md      # Agent system prompt
│   │   │   │   ├── README.md      # Documentation
│   │   │   │   └── .git/          # Git metadata (for updates)
```

### Agent Metadata Format

Agents must include an `.agent.yaml` file:

```yaml
# .agent.yaml
name: stack-optimizer
display_name: Stack Optimizer
version: 1.2.3
author: username
description: Analyzes and optimizes Atmos stack configurations
category: optimization

# Atmos version compatibility
atmos:
  min_version: 1.50.0
  max_version: ""  # empty = no upper limit

# System prompt configuration
prompt:
  file: prompt.md

# Tool access configuration (optional)
tools:
  allowed:
    - describe_stacks
    - describe_component
    - validate_stacks
  restricted:
    - terraform_apply
    - terraform_destroy

# Agent capabilities
capabilities:
  - stack-analysis
  - performance-optimization
  - cost-estimation

# Optional: Dependencies on other agents
dependencies: []

# Optional: Required environment variables
env:
  - name: OPTIMIZER_API_KEY
    required: false
    description: API key for advanced optimization features

# Links
repository: https://github.com/username/atmos-agent-stack-optimizer
documentation: https://github.com/username/atmos-agent-stack-optimizer#readme
```

### Agent Prompt Structure

Agents must include a `prompt.md` file with consistent structure:

```markdown
# Agent: Stack Optimizer

## Role
You are a specialized agent for optimizing Atmos stack configurations...

## Your Expertise
- Stack performance analysis
- Resource optimization
- Cost reduction strategies

## Instructions
1. Analyze stack configurations for performance bottlenecks
2. Identify redundant resources
3. Suggest optimization strategies

## Tools Available
You have access to the following Atmos tools:
- `describe_stacks` - List and analyze stacks
- `describe_component` - Get component details
- `validate_stacks` - Validate configurations

## Best Practices
...
```

## CLI Commands

### List Available Agents

```bash
# List installed agents
atmos ai agent list
atmos ai agent list --installed

# Search marketplace (future: query GitHub API)
atmos ai agent search <query>
atmos ai agent search "terraform"

# Show agent details
atmos ai agent show <agent-name>
atmos ai agent show github.com/username/stack-optimizer
```

**Output:**
```
Installed Agents:

  Built-in:
  • general              - General-purpose assistant
  • stack-analyzer       - Stack analysis specialist
  • component-refactor   - Component refactoring expert
  • security-auditor     - Security auditing specialist
  • config-validator     - Configuration validation expert

  Community:
  • stack-optimizer      - Analyzes and optimizes stacks (v1.2.3)
    └─ github.com/username/stack-optimizer
  • cost-analyzer        - Cloud cost analysis (v2.0.1)
    └─ github.com/cloudposse/atmos-agent-cost
```

### Install Agent

```bash
# Install from GitHub (uses default branch or latest tag)
atmos ai agent install github.com/username/agent-name
atmos ai agent install github.com/username/agent-name@v1.2.3

# Install from URL (direct Git clone)
atmos ai agent install https://github.com/username/agent-name.git

# Install with specific name
atmos ai agent install github.com/username/agent-name --as my-agent

# Dry run (show what would be installed)
atmos ai agent install github.com/username/agent-name --dry-run
```

**Process:**
1. Clone repository to temporary directory
2. Validate `.agent.yaml` exists and is valid
3. Check Atmos version compatibility
4. Verify prompt.md exists
5. Move to `~/.atmos/agents/`
6. Register in `registry.json`
7. Display success message with usage instructions

### Update Agent

```bash
# Update specific agent
atmos ai agent update <agent-name>
atmos ai agent update stack-optimizer

# Update all agents
atmos ai agent update --all

# Check for updates without installing
atmos ai agent update --check
```

### Uninstall Agent

```bash
# Uninstall agent
atmos ai agent uninstall <agent-name>
atmos ai agent uninstall stack-optimizer

# Force uninstall (skip confirmations)
atmos ai agent uninstall <agent-name> --force
```

### Agent Information

```bash
# Show detailed agent info
atmos ai agent info <agent-name>
```

**Output:**
```
Agent: stack-optimizer
Display Name: Stack Optimizer
Version: 1.2.3
Author: username
Category: optimization
Repository: https://github.com/username/atmos-agent-stack-optimizer

Description:
Analyzes and optimizes Atmos stack configurations for performance and cost.

Capabilities:
• stack-analysis
• performance-optimization
• cost-estimation

Tool Access:
Allowed: describe_stacks, describe_component, validate_stacks
Restricted: terraform_apply, terraform_destroy

Atmos Compatibility:
Minimum: 1.50.0
Maximum: (any)

Installation:
Location: ~/.atmos/agents/github.com/username/agent-name/
Installed: 2025-10-20 14:30:00
```

## Registry Format

`~/.atmos/agents/registry.json`:

```json
{
  "version": "1.0.0",
  "agents": {
    "stack-optimizer": {
      "name": "stack-optimizer",
      "display_name": "Stack Optimizer",
      "source": "github.com/username/stack-optimizer",
      "version": "1.2.3",
      "installed_at": "2025-10-20T14:30:00Z",
      "updated_at": "2025-10-20T14:30:00Z",
      "path": "~/.atmos/agents/github.com/username/stack-optimizer",
      "is_builtin": false,
      "enabled": true
    }
  }
}
```

## Implementation Plan

### Phase 2.1: Core Installation (Week 1-2)

**Goal:** Basic agent installation from GitHub

**Tasks:**
1. Create `pkg/ai/agents/marketplace/` package
2. Implement agent downloader (Git clone)
3. Implement `.agent.yaml` parser and validator
4. Implement agent registry manager
5. Add `atmos ai agent install <url>` command
6. Add `atmos ai agent list` command
7. Add `atmos ai agent uninstall <name>` command

**Acceptance Criteria:**
- Can install agent from GitHub URL
- Can list installed agents
- Can uninstall agents
- Installed agents appear in TUI agent switcher

### Phase 2.2: Agent Management (Week 3)

**Goal:** Update and inspect agents

**Tasks:**
1. Add `atmos ai agent update` command
2. Add `atmos ai agent info` command
3. Add version checking (compare installed vs available)
4. Add agent enablement/disablement
5. Add dry-run mode for installations

**Acceptance Criteria:**
- Can update installed agents
- Can view detailed agent information
- Can check for updates without installing
- Can disable agents without uninstalling

### Phase 2.3: Security & Validation (Week 4)

**Goal:** Ensure downloaded agents are safe

**Tasks:**
1. Implement schema validation for `.agent.yaml`
2. Add Atmos version compatibility checks
3. Add tool access validation
4. Add prompt structure validation
5. Add security warnings for restricted tools
6. Add GPG signature verification (optional)

**Acceptance Criteria:**
- Invalid agents rejected with clear errors
- Incompatible versions rejected
- Tool access properly restricted
- Security warnings shown for dangerous permissions

### Phase 2.4: Discovery (Week 5)

**Goal:** Make it easy to find agents

**Tasks:**
1. Create curated agent list (JSON file in repo)
2. Add `atmos ai agent search` command
3. Add `atmos ai agent browse` command (opens browser)
4. Add featured/popular agents section
5. Update documentation with agent development guide

**Acceptance Criteria:**
- Can search for agents by keyword
- Can browse curated agent list
- Documentation explains how to publish agents

## Security Considerations

### Trust Model

1. **No Code Execution** - Agents only provide prompts, never execute code directly
2. **Tool Restrictions** - Agents specify allowed/restricted tools in `.agent.yaml`
3. **User Consent** - Destructive operations (apply, destroy) always require confirmation
4. **Sandboxing** - Agent prompts cannot escape system prompt boundaries
5. **Verification** - Display agent source and author before installation

### Security Checklist

- [ ] Validate `.agent.yaml` schema strictly
- [ ] Reject agents with invalid tool configurations
- [ ] Warn about agents requesting destructive tool access
- [ ] Show agent source URL before installation
- [ ] Store agent metadata for audit trail
- [ ] Implement rate limiting for installs (prevent DoS)
- [ ] Sanitize agent names (prevent path traversal)
- [ ] Verify Git repository authenticity
- [ ] Optional: GPG signature verification for trusted publishers

### Security Warnings

When installing agents, show warnings:

```
⚠️  Security Notice:
This agent requests access to destructive operations:
  • terraform_apply
  • terraform_destroy

Review the agent source before using:
https://github.com/username/agent-name

Do you want to continue? [y/N]
```

## Agent Development Guide

### Quick Start

1. **Create Repository**
   ```bash
   mkdir atmos-agent-myagent
   cd atmos-agent-myagent
   git init
   ```

2. **Create `.agent.yaml`**
   ```yaml
   name: myagent
   display_name: My Agent
   version: 1.0.0
   author: username
   description: Description of what your agent does
   category: general
   atmos:
     min_version: 1.50.0
   prompt:
     file: prompt.md
   ```

3. **Create `prompt.md`**
   ```markdown
   # Agent: My Agent

   ## Role
   You are a specialized agent for...

   ## Your Expertise
   - Skill 1
   - Skill 2

   ## Instructions
   1. Step 1
   2. Step 2
   ```

4. **Create `README.md`**
   Document your agent's purpose, usage, and examples.

5. **Publish**
   ```bash
   git add .
   git commit -m "Initial version"
   git tag v1.0.0
   git push origin main --tags
   ```

6. **Share**
   Users can install: `atmos ai agent install github.com/username/atmos-agent-myagent`

### Best Practices

1. **Versioning** - Use semantic versioning (v1.2.3)
2. **Documentation** - Include clear README with examples
3. **Tool Access** - Request minimal tool permissions
4. **Testing** - Test your agent prompt thoroughly
5. **Updates** - Tag releases for version tracking
6. **Categories** - Use standard categories: general, analysis, refactor, security, validation, optimization

### Publishing Checklist

- [ ] `.agent.yaml` is valid and complete
- [ ] `prompt.md` follows standard structure
- [ ] `README.md` explains agent purpose and usage
- [ ] Repository has clear license (MIT recommended)
- [ ] Version tagged with semantic versioning
- [ ] Tool access is minimal and justified
- [ ] Agent tested with real Atmos workflows

## Alternative Installation Sources

### Future: Agent Marketplace API

```bash
# Install from official marketplace
atmos ai agent install marketplace:stack-optimizer

# List marketplace agents
atmos ai agent marketplace list
atmos ai agent marketplace search "cost"
```

### Future: Local Installation

```bash
# Install from local directory
atmos ai agent install ./my-agent/

# Install from archive
atmos ai agent install ./agent.tar.gz
```

## Compatibility Matrix

| Atmos Version | Agent Format | Registry Version |
|---------------|--------------|------------------|
| 1.50.x        | 1.0.0        | 1.0.0           |
| 1.51.x+       | 1.0.0        | 1.0.0           |

## Testing Strategy

### Unit Tests
- Agent metadata parsing
- Registry operations (add, remove, update)
- Version compatibility checking
- Tool access validation

### Integration Tests
- Install agent from GitHub
- Update installed agent
- Uninstall agent
- Agent appears in TUI switcher
- Agent prompt loads correctly

### Security Tests
- Reject invalid `.agent.yaml`
- Reject incompatible versions
- Sanitize agent names (path traversal)
- Tool access restrictions enforced

## Success Metrics

- Number of community agents published
- Agent install success rate
- Agent update frequency
- User satisfaction (qualitative feedback)
- Security incidents (target: 0)

## Open Questions

1. **Agent Namespacing** - How to handle name collisions between different sources?
   - Proposal: Use full source path (e.g., `github.com/user/agent-name`)

2. **Agent Dependencies** - Should agents depend on other agents?
   - Proposal: Phase 3 feature, not MVP

3. **Private Repositories** - Support for private GitHub repos?
   - Proposal: Use SSH keys or PAT, document in agent dev guide

4. **Agent Marketplace Hosting** - Self-hosted vs third-party?
   - Proposal: Start with GitHub-only, marketplace API in Phase 3

5. **Agent Ratings** - How to indicate agent quality?
   - Proposal: Phase 3 feature, start with curated list

## References

- [Claude Code Agents](https://github.com/wshobson/agents) - Inspiration for agent marketplace
- [Awesome Claude Code Subagents](https://github.com/VoltAgent/awesome-claude-code-subagents) - Community agent examples
- [Atmos AI Agent System PRD](./ai-agents.md) - Phase 1 implementation

## Changelog

- 2025-10-28: Initial design document
