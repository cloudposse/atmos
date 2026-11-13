# Claude PR Review Remediator

This directory contains Claude Code agents for automating PR review feedback remediation and CI/CD issue fixing.

## Structure

```text
.claude/
├── agents/
│   └── pr-review-remediator.md    # Main PR review remediation agent
├── commands/
│   └── fix-pr.md               # Slash command to fix PR issues
└── README.md                   # This file
```

## Usage

### Method 1: Direct Agent Invocation

```bash
# Use the Task tool to invoke the agent
/agent pr-review-remediator
```

Then provide the PR number when prompted.

### Method 2: Slash Command

```bash
# Use the slash command with a PR number
/fix-pr 1440
```

### Method 3: Manual Invocation

Simply ask Claude to fix a PR:
```text
"Please fix issues in PR #1440 based on review feedback"
```

The agent will automatically be invoked based on the task description.

## Features

### CodeRabbit Integration
- Automatically finds and parses CodeRabbit review comments
- **Prioritizes "🤖 Prompt for AI Agents" sections** over code diffs
- Uses natural language prompts to understand the intent
- Implements fixes based on understanding + project standards (not copying code)
- Validates suggestions before applying them
- Distinguishes between actionable items and nitpicks
- Presents a validation analysis showing which suggestions are valid

### GitHub Status Checks
- Monitors all CI/CD checks (tests, linting, security scans)
- Identifies root causes of failures
- Provides targeted remediation strategies

### Smart Linting
- **ONLY lints changed files** in the PR (not the entire codebase)
- Uses `make lint` which already implements `--new-from-rev=origin/main`
- Targets fixes to specific files that were modified

### Safety Controls
- Always requires user approval before making changes
- Validates all suggestions against project standards
- Never modifies golden snapshots in tests/test-cases/
- Creates feature branches for fixes (never pushes to main)

## Example Workflow

1. **Analyze PR**:
   ```bash
   /analyze-pr 1440
   ```

2. **Agent Response**:
   - Fetches PR details and changed files
   - Extracts CodeRabbit comments
   - Checks CI/CD status
   - Validates each suggestion
   - Presents categorized action plan

3. **User Approval**:
   - Review the proposed fixes
   - Approve valid suggestions
   - Skip questionable ones

4. **Execution**:
   - Agent applies approved fixes
   - Runs validation (lint, test, build)
   - Prepares commit with descriptive message

## Key Commands Used by Agent

```bash
# Get changed files in PR (excluding deleted files)
gh api repos/cloudposse/atmos/pulls/<PR_NUMBER>/files \
  --jq '.[] | select(.status != "removed") | .filename'

# Find CodeRabbit comments (case-insensitive, handles [bot] suffix)
gh api repos/cloudposse/atmos/issues/<PR>/comments \
  --jq '.[] | select(.user.login | ascii_downcase | contains("coderabbit"))'

# Extract AI Agent prompts (PREFERRED)
gh pr view <PR_NUMBER> --repo cloudposse/atmos --comments | \
  grep -A 50 "Prompt for AI Agents"

# Lint only changed files
make lint  # Uses --new-from-rev=origin/main

# Run tests for changed packages
go test ./pkg/merge -v

# Check PR status
gh pr checks <PR_NUMBER> --repo cloudposse/atmos
```

## Validation Process

The agent validates CodeRabbit suggestions by:

1. **Reading AI Prompts First**: Extracts "🤖 Prompt for AI Agents" sections
2. **Understanding Intent**: Analyzes the natural language explanation
3. **Alignment with Standards**: Does it match CLAUDE.md requirements?
4. **Real Issue vs Preference**: Is it fixing an actual problem?
5. **Breaking Changes**: Will it break existing functionality?
6. **Consistency**: Does it match existing codebase patterns?
7. **Golden Snapshots**: Does it respect test fixtures?
8. **Custom Implementation**: Writes fixes based on understanding, not copying

Each suggestion is marked as:
- ✅ **Valid**: Safe to apply
- ⚠️ **Needs Review**: Requires discussion
- ❌ **Skip**: Should not be applied

## Customization

To modify the agent behavior, edit:
- `.claude/agents/pr-review-remediator.md` - Main agent logic
- `.claude/commands/analyze-pr.md` - Slash command template

## Best Practices

1. Always review the agent's analysis before approving fixes
2. Use the agent for routine feedback (formatting, linting, simple fixes)
3. Handle complex architectural changes manually
4. Keep the agent updated with project-specific requirements