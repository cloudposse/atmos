# Check Agent Size Action

GitHub Action to validate that Claude agent files don't exceed size limits, ensuring context efficiency and optimal performance.

## Purpose

Enforces agent size limits to maintain:
- **Context efficiency** - Agents stay focused and don't bloat the context window
- **Performance** - Smaller agents load and process faster
- **Best practices** - Encourages referencing PRDs instead of duplicating content

## Size Guidelines

Based on `docs/prd/claude-agent-architecture.md`:

| Agent Type | Target Size | Max Size |
|------------|-------------|----------|
| Focused specialist | 8-15 KB | 20 KB |
| Comprehensive specialist | 15-25 KB | 25 KB |
| Meta-agent | 15-25 KB | 25 KB |

## Usage

```yaml
- name: Check agent sizes
  uses: ./.github/actions/check-agent-size
  with:
    agent-path: .claude/agents
    max-size: 25000
    github-token: ${{ github.token }}
```

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `agent-path` | Path to agent file or directory | No | `.claude/agents` |
| `max-size` | Maximum file size in bytes | No | `25000` (25KB) |
| `github-token` | GitHub token for PR comments | Yes | - |

## Outputs

| Output | Description |
|--------|-------------|
| `all-agents-ok` | Whether all agents pass size check (true/false) |
| `oversized-agents` | Comma-separated list of oversized agents with sizes |

## Behavior

### When Agents Pass

- ✅ Prints size summary for each agent
- Updates any existing warning comment to success message
- Workflow continues

### When Agents Exceed Limit

- ❌ Lists oversized agents with excess bytes and percentage
- Posts or updates PR comment with actionable guidance
- Fails the workflow check

## PR Comment Example

When agents exceed the limit:

```markdown
> [!WARNING]
> #### Agent Files Too Large
>
> The following agent files exceed the **25000 byte** (25KB) size limit:
>
> - `agent-developer.md`: **27000 bytes** (over by 2000 bytes, ~8%)
>
> **Action needed:** Please compress the oversized agent files. Consider:
> - Removing verbose explanations
> - Consolidating redundant examples
> - Referencing PRDs instead of duplicating content
> - Moving detailed guides to `docs/prd/`
>
> **Target sizes:**
> - Focused specialists: 8-15 KB
> - Comprehensive specialists: 15-25 KB
> - Meta-agents: up to 25 KB
```

## Implementation Details

- Scans all `.md` files in agent directory (excludes `README.md`)
- Calculates exact byte size using `wc -c`
- Updates existing PR comments instead of creating duplicates
- Uses comment marker `<!-- agent-size-check -->` for identification
- Provides clear guidance on how to reduce size

## Related

- `docs/prd/claude-agent-architecture.md` - Agent architecture and size guidelines
- `.claude/agents/agent-developer.md` - Meta-agent for creating agents
- `.github/workflows/agents.yml` - Workflow that uses this action
