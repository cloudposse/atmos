# Check Markdown File Size Action

GitHub Action to validate that PR-modified markdown files do not exceed size limits. Uses glob patterns for flexible file matching and only checks files actually modified in the pull request for efficiency.

## Features

- **PR-Modified Files Only**: Only checks files changed in the pull request using `git diff`
- **Glob Pattern Matching**: Flexible file matching with wildcards (e.g., `*.md`, `.claude/agents/*.md`)
- **Smart PR Comments**: Posts/updates comments with actionable guidance
- **CI Integration**: Fails check if any file exceeds the limit
- **Efficient**: No unnecessary checks of unmodified files

## Usage

### Check Specific File

Check a single markdown file (only if modified in PR):

```yaml
- uses: ./.github/actions/check-claude-md-size
  with:
    file-patterns: 'CLAUDE.md'
    max-size: 40000
    github-token: ${{ github.token }}
```

### Check Multiple Files with Glob Patterns

Check all markdown files matching a pattern (only those modified in PR):

```yaml
- uses: ./.github/actions/check-claude-md-size
  with:
    file-patterns: '.claude/agents/*.md'
    max-size: 25000
    github-token: ${{ github.token }}
```

### Check Multiple Patterns

Space-separated patterns to match different file locations:

```yaml
- uses: ./.github/actions/check-claude-md-size
  with:
    file-patterns: 'CLAUDE.md .claude/agents/*.md docs/prd/*.md'
    max-size: 40000
    github-token: ${{ github.token }}
```

The action uses `git diff` to identify modified files in the PR, then filters them against the provided patterns.

## Inputs

| Name | Description | Required | Default |
|------|-------------|----------|---------|
| `file-patterns` | Space-separated glob patterns for files to check (e.g., `"CLAUDE.md .claude/agents/*.md"`) | No | `CLAUDE.md` |
| `max-size` | Maximum file size in bytes | No | `40000` |
| `base-ref` | Base ref to compare against (default: PR base or origin/main) | No | _(auto-detected)_ |
| `github-token` | GitHub token for posting PR comments | Yes | - |

## Outputs

| Name | Description |
|------|-------------|
| `size` | Current file size in bytes (single file pattern mode only) |
| `exceeds-limit` | Whether any file exceeds the size limit (`true`/`false`) |
| `usage-percent` | Percentage of size limit used (single file pattern mode only) |
| `all-files-ok` | Whether all files pass size check (`true`/`false`) |

## How It Works

1. **Detects PR context**: Automatically identifies the base branch to compare against
2. **Finds modified files**: Uses `git diff --name-only --diff-filter=ACM` to get added/changed/modified files
3. **Pattern matching**: Filters modified files against provided glob patterns using bash pattern matching
4. **Size checking**: Validates each matched file against the size limit
5. **Reports results**: Posts/updates PR comment if any files exceed the limit

## Examples

### Check CLAUDE.md

```yaml
name: CLAUDE.md Size Check
on:
  pull_request:
    paths:
      - 'CLAUDE.md'
      - '.github/workflows/claude.yml'
      - '.github/actions/check-claude-md-size/**'

jobs:
  size-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: ./.github/actions/check-claude-md-size
        with:
          file-patterns: 'CLAUDE.md'
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
      - '.github/workflows/agents.yml'
      - '.github/actions/check-claude-md-size/**'

jobs:
  agent-size-check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: ./.github/actions/check-claude-md-size
        with:
          file-patterns: '.claude/agents/*.md'
          max-size: 25000
          github-token: ${{ github.token }}
```

## Behavior

### When No Files Modified

- ✅ Prints "No files modified in this PR"
- ✅ Workflow passes
- ℹ️ No PR comment posted

### When Modified Files Pass

- ✅ Prints size summary for each modified file
- ✅ Updates any existing warning comment to success
- ✅ Workflow passes

**Console Output:**
```
Comparing against base: origin/main
File patterns: .claude/agents/*.md
✅ agent-developer.md: 23514 bytes (94% of limit)
```

### When Modified Files Exceed Limit

- ❌ Lists oversized files with details
- ❌ Posts PR comment with specific guidance
- ❌ Fails workflow check

**PR Comment Example:**
```markdown
> [!WARNING]
> #### Modified Files Too Large
>
> The following modified files exceed the **25000 byte** size limit:
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
