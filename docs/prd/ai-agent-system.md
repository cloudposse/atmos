# PRD: AI Agent System for Atmos

## Overview

Add specialized AI agents to Atmos AI that provide task-specific expertise and tool access. Similar to Claude Code's agent system but focused on Atmos infrastructure operations.

## Problem Statement

Currently, Atmos AI uses a single general-purpose system prompt for all tasks. Users would benefit from:

1. **Task-specific expertise** - Specialized prompts for different operations (security audits, refactoring, analysis)
2. **Focused tool access** - Agents with limited tool subsets prevent accidental operations
3. **Better quality** - Domain-specific prompts improve response quality
4. **Reusable workflows** - Pre-configured agents for common tasks

## Goals

### Primary Goals

1. Enable users to select specialized agents for specific tasks
2. Provide 3-5 built-in agents for common Atmos operations
3. Allow custom agent configuration in `atmos.yaml`
4. Maintain backward compatibility with existing AI functionality

### Non-Goals (Phase 1)

- Autonomous sub-agent spawning (future phase)
- Parallel agent execution
- Agent-to-agent communication
- Dynamic agent creation from natural language

## User Experience

### TUI Agent Selection

Users can switch agents in the TUI using keyboard shortcut:

```
Press Ctrl+A to select agent:

1. General (default)
2. Stack Analyzer
3. Component Refactor
4. Security Auditor
5. Config Validator

Select agent (1-5): 2

Switched to Stack Analyzer agent
```

### CLI Agent Selection

Users can specify agent for single-shot queries:

```bash
atmos ai ask --agent stack-analyzer "Analyze all prod stacks"
```

### Agent Behavior

Each agent has:
- **Specialized system prompt** - Task-specific instructions
- **Tool subset** - Limited, relevant tools
- **Clear focus** - Communicates its specialty to user

Example:
```
You: Analyze security of vpc stack

Stack Analyzer: I'm specialized for stack analysis.
Let me examine the vpc stack configuration...

[Uses describe_component, read_stack_file tools]

Analysis complete:
✅ 15 components analyzed
⚠️  3 security concerns found:
   ...
```

## Technical Design

### Architecture

```
pkg/ai/agents/
├── agent.go          # Agent interface and base types
├── registry.go       # Agent registration system
├── builtin.go        # Built-in agent definitions
└── loader.go         # Load agents from config

pkg/schema/
└── ai.go            # Add Agents field to AISettings

pkg/ai/tui/
└── chat.go          # Agent selection and switching
```

### Agent Structure

```go
// Agent represents a specialized AI assistant.
type Agent struct {
    Name          string   // Unique identifier (e.g., "stack-analyzer")
    DisplayName   string   // User-facing name (e.g., "Stack Analyzer")
    Description   string   // What this agent does
    SystemPrompt  string   // Specialized instructions
    AllowedTools  []string // Tool names this agent can use
    RestrictedTools []string // Tools requiring extra confirmation
    Category      string   // "analysis", "refactor", "security", etc.
}
```

### Built-in Agents

#### 1. General (Default)

- **Purpose**: General-purpose assistant (current behavior)
- **Tools**: All tools
- **Prompt**: Current system prompt

#### 2. Stack Analyzer

- **Purpose**: Analyze stack configurations, dependencies, and architecture
- **Tools**:
  - `describe_component`
  - `describe_affected`
  - `list_stacks`
  - `read_stack_file`
  - `get_template_context`
- **Prompt**:
  ```
  You are a specialized Atmos Stack Analyzer. Your expertise is in:
  - Analyzing stack configurations and component relationships
  - Identifying stack dependencies and inheritance patterns
  - Reviewing architecture and design patterns
  - Finding configuration issues and inconsistencies

  When analyzing stacks:
  1. Start with high-level overview
  2. Examine component configurations
  3. Check imports and inheritance
  4. Review template variables
  5. Identify patterns and potential issues
  ```

#### 3. Component Refactor

- **Purpose**: Refactor component code and configurations
- **Tools**:
  - `list_component_files`
  - `read_component_file`
  - `read_file`
  - `edit_file`
  - `search_files`
  - `execute_bash` (for testing)
- **Prompt**:
  ```
  You are a specialized Atmos Component Refactoring Assistant. Your expertise is in:
  - Refactoring Terraform/Helmfile component code
  - Improving code structure and organization
  - Applying best practices
  - Modernizing deprecated patterns

  When refactoring:
  1. Read and understand current code
  2. Identify improvement opportunities
  3. Make targeted, safe changes
  4. Test changes when possible
  5. Explain rationale for changes
  ```

#### 4. Security Auditor

- **Purpose**: Security review of infrastructure configurations
- **Tools**:
  - `describe_component`
  - `list_stacks`
  - `read_stack_file`
  - `read_component_file`
  - `validate_stacks`
- **Prompt**:
  ```
  You are a specialized Atmos Security Auditor. Your expertise is in:
  - Identifying security vulnerabilities in configurations
  - Reviewing IAM policies and permissions
  - Checking network security (CIDR blocks, security groups)
  - Validating encryption and secrets management

  Focus on:
  - Overly permissive IAM policies
  - Public exposure (0.0.0.0/0 CIDR blocks)
  - Unencrypted resources
  - Missing security features
  - Hardcoded secrets
  ```

#### 5. Config Validator

- **Purpose**: Validate Atmos configuration files
- **Tools**:
  - `validate_stacks`
  - `describe_config`
  - `read_stack_file`
  - `validate_file_lsp` (if LSP enabled)
- **Prompt**:
  ```
  You are a specialized Atmos Configuration Validator. Your expertise is in:
  - Validating YAML syntax and structure
  - Checking schema compliance
  - Verifying variable references
  - Ensuring import paths are valid

  When validating:
  1. Check YAML syntax
  2. Validate against schema
  3. Verify all imports exist
  4. Check variable references
  5. Report clear, actionable errors
  ```

### Configuration Schema

```yaml
settings:
  ai:
    enabled: true
    default_provider: "anthropic"
    default_agent: "general"  # Optional, defaults to "general"

    # Built-in agents are always available
    # Custom agents can be added here
    agents:
      custom-stack-deployer:
        display_name: "Stack Deployer"
        description: "Deploy stacks with safety checks"
        system_prompt: |
          You are a specialized deployment assistant...
        allowed_tools:
          - describe_component
          - validate_stacks
          - execute_atmos_command
        restricted_tools:
          - execute_atmos_command  # Require confirmation
        category: "deployment"
```

### Agent Registry

```go
// Registry manages available agents.
type Registry struct {
    agents map[string]*Agent
    mu     sync.RWMutex
}

// Register adds an agent to the registry.
func (r *Registry) Register(agent *Agent) error

// Get retrieves an agent by name.
func (r *Registry) Get(name string) (*Agent, error)

// List returns all registered agents.
func (r *Registry) List() []*Agent

// ListByCategory returns agents in a category.
func (r *Registry) ListByCategory(category string) []*Agent
```

### TUI Integration

```go
// In ChatModel
type ChatModel struct {
    // ... existing fields
    currentAgent *agents.Agent
    agentRegistry *agents.Registry
}

// Agent selection handling
case tea.KeyMsg:
    if key.Matches(msg, m.keyMap.SwitchAgent) { // Ctrl+A
        return m.showAgentSelector()
    }

// System prompt uses agent's prompt
func (m *ChatModel) getSystemPrompt() string {
    if m.currentAgent != nil {
        return m.currentAgent.SystemPrompt
    }
    return defaultSystemPrompt
}

// Tool filtering
func (m *ChatModel) getAvailableTools() []tools.Tool {
    allTools := m.executor.ListTools()
    if m.currentAgent != nil && len(m.currentAgent.AllowedTools) > 0 {
        return filterTools(allTools, m.currentAgent.AllowedTools)
    }
    return allTools
}
```

## Implementation Plan

### Phase 1: Core Infrastructure (Current)

1. ✅ Create PRD document
2. Create `pkg/ai/agents/` package
3. Define Agent struct and Registry
4. Implement built-in agents
5. Add configuration schema
6. Add tests

### Phase 2: TUI Integration

7. Add agent selection UI (Ctrl+A)
8. Display current agent in status line
9. Add agent indicator in prompts
10. Implement tool filtering

### Phase 3: CLI Integration

11. Add `--agent` flag to `atmos ai ask`
12. Add `--agent` flag to `atmos ai chat`
13. List available agents command

### Phase 4: Documentation

14. Document agent system in `website/docs/ai/agents.mdx`
15. Update configuration docs
16. Add examples for each built-in agent

## Testing Strategy

### Unit Tests

- Agent registration and retrieval
- Tool filtering logic
- Configuration loading
- Agent validation

### Integration Tests

- Agent switching in TUI
- Tool execution with filtered tools
- Custom agent configuration

### Manual Testing

- Test each built-in agent with real tasks
- Verify tool restrictions work
- Test agent switching mid-conversation

## Success Metrics

1. **Adoption**: 30%+ of AI sessions use non-general agents
2. **Quality**: Higher user satisfaction for task-specific agents
3. **Safety**: Reduced accidental operations with tool filtering
4. **Usage**: At least 50 uses per built-in agent per month

## Future Enhancements (Post-Phase 1)

### Phase 2: Advanced Features

- **Agent spawning**: Spawn sub-agents from main conversation
- **Parallel execution**: Run multiple agents concurrently
- **Agent communication**: Agents can collaborate
- **Dynamic agents**: Create agents from natural language descriptions

### Phase 3: Community Agents

- **Agent marketplace**: Share and discover community agents
- **Agent templates**: Easy agent creation wizard
- **Agent analytics**: Track agent performance and usage

## Security Considerations

1. **Tool restrictions**: Agents cannot bypass allowed tool lists
2. **Confirmation still required**: Tool permissions still apply
3. **No elevated privileges**: Agents don't grant extra permissions
4. **Audit trail**: Log agent selection and tool usage

## Backward Compatibility

- ✅ Default behavior unchanged (uses "general" agent)
- ✅ Existing configurations work without changes
- ✅ No breaking API changes
- ✅ Opt-in feature

## Open Questions

1. **Agent memory**: Should agents have persistent memory?
2. **Agent costs**: Track token usage per agent?
3. **Agent templates**: Allow users to create agents from templates?
4. **Agent inheritance**: Can agents extend other agents?

## References

- Claude Code agents: https://github.com/wshobson/agents
- Claude Code Task tool documentation
- Atmos tool system: `pkg/ai/tools/`
- Atmos AI configuration: `website/docs/ai/configuration.mdx`
