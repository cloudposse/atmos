# Atmos AI Agent Prompts

This directory contains system prompt files for Atmos AI agents. Each agent has a dedicated Markdown file that defines its behavior, expertise, and instructions.

## Architecture

**Separation of Concerns:**
- **Go code** (`pkg/ai/agents/`) - Agent infrastructure, tool execution, registry
- **Markdown files** (`pkg/ai/agents/prompts/`) - Agent knowledge, instructions, best practices

**Benefits:**
- Update agent behavior without recompiling
- Version-controlled agent knowledge
- Easy community contributions
- Progressive disclosure (load only when agent is activated)
- Documentation-as-code approach

## File Structure

Each agent prompt file follows this structure:

```markdown
# Agent: [Name] [Icon]

## Role
Brief description of the agent's purpose

## Your Expertise
- Domain knowledge areas
- Specializations
- Key competencies

## Instructions
Step-by-step guidance for the agent

## Best Practices
Extracted from Atmos documentation (llms.txt, llms-full.txt)

## Tools You Should Use
Recommended tools for this agent's tasks

## Example Workflows
Common usage patterns and examples
```

## Agent Files

- **general.md** - 🤖 General-purpose assistant for all Atmos operations
- **stack-analyzer.md** - 📊 Stack configuration analysis and dependency mapping
- **component-refactor.md** - 🔧 Component design and Terraform refactoring
- **security-auditor.md** - 🔒 Security review and compliance validation
- **config-validator.md** - ✅ Configuration validation and schema checking

## How Prompts Are Loaded

1. **Agent Registration** - Agent defined with `SystemPromptPath` pointing to .md file
2. **Agent Activation** - When user switches to agent (Ctrl+A), prompt file is read
3. **System Prompt** - Markdown content becomes agent's system prompt
4. **Embedded FS** - Files embedded in binary via `//go:embed` for distribution

## Updating Prompts

To update an agent's behavior:
1. Edit the corresponding .md file
2. Test with `go run . ai chat` and switch to the agent
3. Commit changes to Git
4. Rebuild Atmos to embed updated prompts

## Content Sources

Agent prompts are derived from:
- **llms.txt** - Atmos documentation index
- **llms-full.txt** - Comprehensive Atmos knowledge base
- **website/docs/** - Official documentation
- Community feedback and improvements

## Progressive Disclosure

Following the pattern from wshobson/agents:
- **Metadata** (Agent struct) - Always loaded (name, description, icon)
- **Instructions** (Markdown file) - Loaded when agent is activated
- **Resources** (Tools, docs) - Loaded on-demand when agent needs them

This approach minimizes token usage while maintaining full functionality.
