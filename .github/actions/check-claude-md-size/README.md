# Check CLAUDE.md Size Action

Validates that CLAUDE.md does not exceed the configured size limit to maintain performance.

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `file-path` | Path to CLAUDE.md file | No | `CLAUDE.md` |
| `max-size` | Maximum file size in characters | No | `40000` |
| `github-token` | GitHub token for posting comments | Yes | - |

## Outputs

| Output | Description |
|--------|-------------|
| `size` | Current file size in characters |
| `exceeds-limit` | Whether the file exceeds the size limit (true/false) |
| `usage-percent` | Percentage of limit used |

## Usage

### Basic Usage

```yaml
- name: Check CLAUDE.md size
  uses: ./.github/actions/check-claude-md-size
  with:
    github-token: ${{ github.token }}
```

### Custom Configuration

```yaml
- name: Check CLAUDE.md size
  uses: ./.github/actions/check-claude-md-size
  with:
    file-path: CLAUDE.md
    max-size: 40000
    github-token: ${{ secrets.GITHUB_TOKEN }}
```

## Behavior

1. **File Size Check**: Validates the file size against the configured limit
2. **PR Comments**: Posts/updates warning or success comments on pull requests
3. **CI Integration**: Fails the check if the file exceeds the limit
4. **Smart Comments**: Updates existing comments rather than creating duplicates

## Error Messages

If the file exceeds the limit, the action will:
- Post a warning comment to the PR with actionable guidance
- Fail the CI check to prevent merging
- Show the exact size, limit, and overage percentage

## Success Messages

When the file is within limits and a previous warning exists:
- Updates the warning comment to a success message
- Shows current usage percentage
- Passes the CI check
