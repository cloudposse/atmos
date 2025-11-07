# Check Markdown File Size Action

GitHub Action to validate that markdown files do not exceed size limits. Supports both single file and directory modes for automatic checking of all markdown files.

## Features

- **Single File Mode**: Check one markdown file against a size limit
- **Directory Mode**: Automatically check all `.md` files in a directory
- **Smart PR Comments**: Posts/updates comments with actionable guidance
- **Exclude Patterns**: Skip specific files like `README.md`
- **CI Integration**: Fails check if any file exceeds the limit

## Usage

### Single File Mode

Check a specific markdown file:

```yaml
- uses: ./.github/actions/check-claude-md-size
  with:
    file-path: CLAUDE.md
    max-size: 40000
    github-token: ${{ github.token }}
```

### Directory Mode (Recommended)

Automatically check all `.md` files in a directory:

```yaml
- uses: ./.github/actions/check-claude-md-size
  with:
    file-path: .claude/agents
    max-size: 25000
    exclude-pattern: README.md
    github-token: ${{ github.token }}
```

The action automatically detects if `file-path` is a file or directory and adjusts behavior accordingly.

## Inputs

| Name | Description | Required | Default |
|------|-------------|----------|---------|
| `file-path` | Path to markdown file or directory | No | `CLAUDE.md` |
| `max-size` | Maximum file size in bytes | No | `40000` |
| `exclude-pattern` | Pattern to exclude files (directory mode) | No | `README.md` |
| `github-token` | GitHub token for posting PR comments | Yes | - |

## Outputs

| Name | Description |
|------|-------------|
| `size` | Current file size in bytes (single file mode only) |
| `exceeds-limit` | Whether any file exceeds the size limit (`true`/`false`) |
| `usage-percent` | Percentage of size limit used (single file mode only) |
| `all-files-ok` | Whether all files pass size check (`true`/`false`) |

## Modes

The action automatically detects the mode based on `file-path`:

- **Single File Mode**: When `file-path` is a file, checks that one file
- **Directory Mode**: When `file-path` is a directory, finds all `.md` files (excluding pattern) and checks each

## Examples

### Check CLAUDE.md

```yaml
name: CLAUDE.md Size Check
on:
  pull_request:
    paths:
      - 'CLAUDE.md'

jobs:
  size-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: ./.github/actions/check-claude-md-size
        with:
          file-path: CLAUDE.md
          max-size: 40000
          github-token: ${{ github.token }}
```

### Check All Agent Files

```yaml
name: Agent Quality Checks
on:
  pull_request:
    paths:
      - '.claude/agents/**'

jobs:
  agent-size-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: ./.github/actions/check-claude-md-size
        with:
          file-path: .claude/agents
          max-size: 25000
          exclude-pattern: README.md
          github-token: ${{ github.token }}
```

## Behavior

### When Files Pass

- ✅ Prints size summary for each file
- ✅ Updates any existing warning comment to success
- ✅ Workflow passes

**Console Output:**
```
✅ agent-developer.md: 23514 bytes (94% of limit)
✅ cobra-flag-expert.md: 15000 bytes (60% of limit)
```

### When Files Exceed Limit

- ❌ Lists oversized files with details
- ❌ Posts PR comment with specific guidance
- ❌ Fails workflow check

**PR Comment Example:**
```markdown
> [!WARNING]
> #### Files Too Large
>
> The following files exceed the **25000 byte** size limit:
>
> - `agent-developer.md`: **27000 bytes** (over by 2000 bytes, ~8%)
>
> **Action needed:** Please compress the oversized files. Consider:
> - Removing verbose explanations
> - Consolidating redundant examples
> - Keeping only essential requirements
> - Moving detailed guides to separate docs in `docs/` or `docs/prd/`
>
> All MANDATORY requirements must be preserved.
```

## Smart Comment Management

- Uses `<!-- claude-md-size-check -->` marker for identification
- Updates existing comments instead of creating duplicates
- Switches from warning to success when issues are resolved
- Only posts comments on pull requests (skips direct pushes)

## Integration

This action is used by:
- `.github/workflows/claude.yml` - CLAUDE.md size enforcement
- `.github/workflows/agents.yml` - Agent file size enforcement

## Related

- `CLAUDE.md` - Main development guidelines
- `.claude/agents/` - Claude agent definitions
- `docs/prd/claude-agent-architecture.md` - Agent architecture and size guidelines
