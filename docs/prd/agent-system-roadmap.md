# Atmos AI Agent System - Complete Roadmap

**Last Updated:** 2025-10-28

## Overview

This document provides a high-level roadmap for the Atmos AI Agent System, covering all phases from initial implementation to advanced marketplace features.

## Phase 1: File-Based Agent Prompts ✅ COMPLETED

**Timeline:** Completed 2025-10-28
**Status:** Production Ready

### Implemented Features

✅ **Embedded Filesystem with go:embed**
- Created `pkg/ai/agents/prompts/` directory
- 5 comprehensive agent prompt files (~6KB each)
- Embedded into binary at compile time

✅ **Agent Architecture Updates**
- Added `SystemPromptPath` field to Agent struct
- Implemented `LoadSystemPrompt()` method
- Backward compatible with hardcoded prompts

✅ **Built-in Agents**
- General - General-purpose assistant
- Stack Analyzer - Stack analysis specialist
- Component Refactor - Component refactoring expert
- Security Auditor - Security auditing specialist
- Config Validator - Configuration validation expert

✅ **TUI Integration**
- Load prompts on initialization
- Load prompts when switching agents (Ctrl+A)
- Load prompts when restoring sessions
- Alphabetical agent sorting in switcher

✅ **Testing**
- 31 tests, all passing
- Unit tests for LoadSystemPrompt
- Integration tests for full system
- Embedded filesystem tests

✅ **Documentation**
- Updated `website/docs/ai/agents.mdx`
- Explained agent prompt system architecture
- Contribution guidelines

### Key Benefits

- **Maintainability** - Easy to update prompts without recompilation
- **Version Control** - Track prompt evolution in Git
- **Token Efficiency** - Load only active agent's prompt (~6KB)
- **Separation of Concerns** - Go for infrastructure, Markdown for knowledge

## Phase 2: Agent Marketplace ⏳ DESIGNED

**Timeline:** 4-5 weeks estimated
**Status:** Design Complete, Ready for Implementation

### Design Documents

- 📄 [PRD: Agent Marketplace](./agent-marketplace.md)
- 📄 [Technical Architecture](./agent-marketplace-architecture.md)

### Planned Features

#### Phase 2.1: Core Installation (Weeks 1-2)

**Goal:** Basic agent installation from GitHub

**Features:**
- Install agents from GitHub URLs
- Download and validate agent packages
- Store in `~/.atmos/agents/`
- Local registry management (`registry.json`)
- List installed agents
- Uninstall agents
- Agents appear in TUI switcher

**Commands:**
```bash
atmos ai agent install github.com/user/agent-name
atmos ai agent install github.com/user/agent-name@v1.2.3
atmos ai agent list
atmos ai agent uninstall <name>
```

#### Phase 2.2: Agent Management (Week 3)

**Goal:** Update and inspect agents

**Features:**
- Update installed agents
- Check for available updates
- View detailed agent information
- Enable/disable agents without uninstalling
- Dry-run mode for installations

**Commands:**
```bash
atmos ai agent update <name>
atmos ai agent update --all
atmos ai agent update --check
atmos ai agent info <name>
```

#### Phase 2.3: Security & Validation (Week 4)

**Goal:** Ensure downloaded agents are safe

**Features:**
- Schema validation for `.agent.yaml`
- Atmos version compatibility checks
- Tool access validation and warnings
- Prompt structure validation
- Security warnings for destructive permissions
- Optional GPG signature verification

**Security Model:**
- No code execution (prompts only)
- Tool access restrictions
- User consent for destructive operations
- Display agent source before install

#### Phase 2.4: Discovery (Week 5)

**Goal:** Make it easy to find agents

**Features:**
- Curated agent list (JSON in repo)
- Search functionality
- Browse command (opens browser)
- Featured/popular agents
- Agent development guide

**Commands:**
```bash
atmos ai agent search <query>
atmos ai agent search "terraform"
atmos ai agent browse
```

### Agent Format

**Directory Structure:**
```
agent-repo/
├── .agent.yaml        # Agent metadata
├── prompt.md          # System prompt
└── README.md          # Documentation
```

**`.agent.yaml` Example:**
```yaml
name: stack-optimizer
display_name: Stack Optimizer
version: 1.2.3
author: username
description: Analyzes and optimizes Atmos stacks
category: optimization

atmos:
  min_version: 1.50.0
  max_version: ""

prompt:
  file: prompt.md

tools:
  allowed:
    - describe_stacks
    - describe_component
    - validate_stacks
  restricted:
    - terraform_apply
    - terraform_destroy

repository: https://github.com/username/agent-repo
```

### Implementation Packages

```
pkg/ai/agents/marketplace/
├── installer.go           # Agent installation logic
├── downloader.go          # Git clone operations
├── validator.go           # Agent validation
├── metadata.go            # .agent.yaml parsing
├── local_registry.go      # Local registry management
├── updater.go             # Agent update logic
└── marketplace_test.go    # Tests
```

## Phase 3: Advanced Features 🔮 FUTURE

**Timeline:** TBD (6+ months out)
**Status:** Conceptual

### Potential Features

1. **Agent Dependencies**
   - Automatically install dependent agents
   - Dependency resolution
   - Conflict detection

2. **Agent Marketplace API**
   - Centralized discovery service
   - Official marketplace at marketplace.atmos.tools
   - Agent verification and signing
   - Statistics and analytics

3. **Rating & Review System**
   - Community ratings (1-5 stars)
   - User reviews and comments
   - Featured/recommended badges
   - Download statistics

4. **Private Registries**
   - Enterprise private agent registries
   - Team-specific agents
   - Access control and permissions

5. **Advanced Security**
   - Agent sandboxing
   - Runtime permission requests
   - Audit logging
   - Security scanning

6. **Auto-Updates**
   - Opt-in automatic updates
   - Update notifications
   - Rollback capability

7. **Agent Analytics**
   - Usage tracking (opt-in)
   - Performance metrics
   - Error reporting

8. **Agent Composition**
   - Multi-agent workflows
   - Agent delegation
   - Specialized sub-agents

## Success Metrics

### Phase 1 (Achieved)
✅ All built-in agents use file-based prompts
✅ Zero regression in existing functionality
✅ 100% test coverage for new code
✅ Documentation complete

### Phase 2 (Planned)
- Community publishes 10+ agents in first 3 months
- 90%+ successful install rate
- 0 security incidents
- Positive user feedback

### Phase 3 (Future)
- 100+ community agents available
- Marketplace ecosystem thriving
- Enterprise adoption

## Technical Debt & Improvements

### Current System
- Consider prompt caching for performance
- Add prompt versioning system
- Improve error messages in agent loading

### Phase 2 Preparation
- Add agent schema validation framework
- Implement safe Git operations
- Create agent testing utilities

## Community Engagement

### Agent Development Resources
- Agent development guide (to be created)
- Example agent templates
- Best practices documentation
- Testing guidelines

### Curated Agent List
Potential categories:
- **General** - Multi-purpose assistants
- **Analysis** - Stack and component analysis
- **Refactoring** - Code transformation
- **Security** - Security auditing
- **Validation** - Configuration validation
- **Optimization** - Performance and cost
- **Migration** - Platform migrations
- **Documentation** - Generating docs

### Example Community Agents
Ideas for community contributions:
- `cost-analyzer` - Cloud cost analysis
- `drift-detector` - Configuration drift detection
- `migration-assistant` - Platform migration helper
- `compliance-auditor` - Compliance checking
- `performance-tuner` - Performance optimization
- `disaster-recovery` - DR planning and testing
- `inventory-manager` - Resource inventory
- `documentation-generator` - Auto-generate docs

## Dependencies & Prerequisites

### External Libraries (Phase 2)
- `go-git/go-git` - Git operations
- `go-playground/validator` - Struct validation
- `go-yaml/yaml` v3 - YAML parsing
- `hashicorp/go-version` - Semver parsing

### Infrastructure
- GitHub API access (rate limits)
- Secure file operations
- XDG directory standards

## Risks & Mitigation

### Security Risks
**Risk:** Malicious agents could abuse tool access
**Mitigation:**
- Tool access validation
- User warnings for destructive operations
- Prompt sandboxing (no code execution)
- Clear security model documentation

**Risk:** Supply chain attacks (compromised repos)
**Mitigation:**
- Display source before install
- GPG signature verification (Phase 2.3)
- Curated agent list (trusted sources)
- User education

### Technical Risks
**Risk:** Agent format changes break existing agents
**Mitigation:**
- Semantic versioning for agent format
- Backward compatibility commitment
- Migration guides

**Risk:** Performance impact from many agents
**Mitigation:**
- Lazy loading
- Caching
- Registry optimization

## Decision Log

### 2025-10-28
- ✅ **Decision:** Use Markdown for agent prompts (not JSON/YAML)
  - **Rationale:** Better readability, easier editing, version control friendly

- ✅ **Decision:** Embed built-in agent prompts in binary
  - **Rationale:** Single binary distribution, faster loading, no file dependencies

- ✅ **Decision:** Use go:embed for embedded filesystem
  - **Rationale:** Standard library, well-tested, zero external dependencies

- 📋 **Decision:** Use Git for agent distribution (Phase 2)
  - **Rationale:** Existing infrastructure, version control, easy updates

- 📋 **Decision:** Store agents in `~/.atmos/agents/`
  - **Rationale:** XDG standards, per-user installation, no root required

- 📋 **Decision:** Use YAML for agent metadata
  - **Rationale:** Consistent with Atmos config format, human-readable

## Timeline

```
Phase 1: File-Based Prompts
│
├─ 2025-10-20: Design & Implementation
├─ 2025-10-25: Testing & Documentation
└─ 2025-10-28: ✅ Complete & Production Ready

Phase 2: Agent Marketplace
│
├─ Week 1-2: Core Installation
│   └─ Install, list, uninstall agents
├─ Week 3: Agent Management
│   └─ Update, info, enable/disable
├─ Week 4: Security & Validation
│   └─ Validation, compatibility, warnings
├─ Week 5: Discovery
│   └─ Search, browse, curated list
└─ Week 6: Documentation & Polish
    └─ Agent dev guide, examples, testing

Phase 3: Advanced Features (6+ months)
│
└─ TBD based on community feedback
```

## References

- [Phase 1 PRD: AI Agents](./ai-agents.md)
- [Phase 2 PRD: Agent Marketplace](./agent-marketplace.md)
- [Phase 2 Architecture](./agent-marketplace-architecture.md)
- [Claude Code Agents](https://github.com/wshobson/agents)
- [Awesome Claude Code Subagents](https://github.com/VoltAgent/awesome-claude-code-subagents)

## Questions & Feedback

For questions or feedback about the agent system roadmap:
- GitHub Issues: https://github.com/cloudposse/atmos/issues
- Discussions: https://github.com/cloudposse/atmos/discussions

## Changelog

- 2025-10-28: Initial roadmap document
- 2025-10-28: Phase 1 marked as complete
- 2025-10-28: Phase 2 design documents added
