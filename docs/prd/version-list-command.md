# Version List and Show Commands

## Overview

This document describes the implementation of `atmos version list` and `atmos version show`, new subcommands that query the GitHub API to display Atmos releases with formatted output including markdown-rendered titles, platform-specific assets, and multiple output formats.

## Problem Statement

### Current State

Atmos provides `atmos version` to show the current version and check for updates:

```bash
$ atmos version
👽 Atmos 1.95.0 on darwin/arm64

$ atmos version --check
👽 Atmos 1.95.0 on darwin/arm64
✨ A new version (1.96.0) is available! Run 'atmos version' to see update instructions.
```

### Limitations

1. **No release history** - Users can't see what versions are available
2. **No release notes** - Can't preview what changed in recent releases
3. **No artifact inspection** - Can't see what files are available in each release
4. **Manual browsing required** - Must visit GitHub to explore releases
5. **No pagination** - Can't navigate through release history
6. **CI/CD friction** - Scripts need to scrape GitHub or use `gh` CLI

### User Stories

**As a platform engineer**, I want to:
- Browse available Atmos versions to plan upgrades
- Read release notes before upgrading
- See what artifacts are available for each release
- Script version discovery in CI/CD pipelines
- Paginate through release history without hitting API limits

**As an infrastructure team**, we need to:
- Audit which versions are available for compliance
- Compare release notes across versions
- Verify release artifacts before downloading
- Automate version discovery in deployment workflows

## Solution: `atmos version list` and `atmos version show`

### Command Design

```bash
# List recent releases (default: 10, stable releases only)
atmos version list

# List specific number of releases with pagination
atmos version list --limit 20 --offset 10

# Include prerelease versions (beta, alpha, rc, etc.)
atmos version list --include-prereleases

# Filter releases by date
atmos version list --since 2025-01-01

# Output in machine-readable format
atmos version list --format json
atmos version list --format yaml

# View details for the latest release
atmos version show

# View details for a specific release
atmos version show v1.95.0
atmos version show 1.95.0  # Also works without 'v' prefix
```

### Output Features

The commands provide:

1. **List View** (text format - default)
   - Borderless table with header separator
   - Version number with green bullet (●) for current installed version
   - Publication date (YYYY-MM-DD format)
   - Release title with markdown rendering (preserves backticks, bold, etc.)
   - Prerelease indicator for beta/alpha releases
   - Terminal width detection (minimum 40 chars)
   - Automatic word wrapping for long titles

2. **Detail View** (`atmos version show`)
   - Full release notes (markdown-rendered with colors preserved)
   - Release metadata (version, published date, title)
   - Artifact list filtered by current OS and architecture:
     - File names
     - File sizes (in MB)
     - Download URLs styled as links (blue, underlined)

3. **Spinner Feedback**
   - Shows spinner animation during GitHub API calls (when TTY detected)
   - Provides visual feedback for network operations

### Flags

#### `atmos version list`

<dl>
  <dt><code>--limit</code>, <code>-l</code></dt>
  <dd>Number of releases to fetch per page (default: 10, max: 100)</dd>

  <dt><code>--offset</code>, <code>-o</code></dt>
  <dd>Number of releases to skip for pagination (default: 0)</dd>

  <dt><code>--since</code>, <code>-s</code></dt>
  <dd>Filter releases published on or after this date (ISO 8601: YYYY-MM-DD)</dd>

  <dt><code>--include-prereleases</code></dt>
  <dd>Include prerelease versions in results (default: false)</dd>

  <dt><code>--format</code>, <code>-f</code></dt>
  <dd>Output format: <code>text</code>, <code>json</code>, <code>yaml</code> (default: text)</dd>
</dl>

#### `atmos version show`

<dl>
  <dt><code>[version]</code></dt>
  <dd>Version to show details for (optional, defaults to latest release)</dd>

  <dt><code>--format</code>, <code>-f</code></dt>
  <dd>Output format: <code>text</code>, <code>json</code>, <code>yaml</code> (default: text)</dd>
</dl>

### Environment Variables

- `ATMOS_GITHUB_TOKEN` / `GITHUB_TOKEN`: GitHub personal access token for increased API rate limits
  - Unauthenticated: 60 requests/hour
  - Authenticated: 5,000 requests/hour
  - Get token: `gh auth token` (if using GitHub CLI) or create at GitHub settings

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                 atmos version list/show                          │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│         cmd/version/ (Command Registry Pattern)                  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ version.go - Parent command + VersionCommandProvider      │  │
│  │ - Implements CommandProvider interface                    │  │
│  │ - Registers with cmd/internal/registry.go                 │  │
│  │ - Group: "Other Commands"                                 │  │
│  └───────────────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ list.go - List subcommand                                 │  │
│  │ - Parse flags (--limit, --offset, --format)               │  │
│  │ - Validate inputs                                          │  │
│  │ - Fetch releases with spinner                              │  │
│  │ - Format output                                            │  │
│  └───────────────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ show.go - Show subcommand                                 │  │
│  │ - Parse version argument                                   │  │
│  │ - Handle optional argument (defaults to latest)            │  │
│  │ - Fetch release with spinner                               │  │
│  │ - Format output                                            │  │
│  └───────────────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ formatters.go - Output formatting                         │  │
│  │ - Table rendering with lipgloss                            │  │
│  │ - Markdown rendering with glamour                          │  │
│  │ - JSON/YAML formatting                                     │  │
│  │ - Platform-specific asset filtering                        │  │
│  └───────────────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ github.go - GitHub client interface                       │  │
│  │ - GitHubClient interface for testability                  │  │
│  │ - RealGitHubClient implementation                          │  │
│  │ - MockGitHubClient for testing                             │  │
│  └───────────────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ markdown/ - Embedded usage examples                       │  │
│  │ - atmos_version_list_usage.md                             │  │
│  │ - atmos_version_show_usage.md                             │  │
│  └───────────────────────────────────────────────────────────┘  │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│         pkg/utils/github_utils.go (GitHub API)                   │
│  - GetGitHubRepoReleases() with pagination                       │
│  - GetGitHubReleaseByTag() for specific version                  │
│  - GetGitHubLatestRelease() for latest                           │
│  - OAuth2 authentication with ATMOS_GITHUB_TOKEN/GITHUB_TOKEN   │
│  - Rate limit checking and handling                              │
│  - Filter out draft releases                                     │
│  - Optional prerelease filtering                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Command Registry Pattern Integration

The `version` command uses Atmos's **Command Registry Pattern** for modular, self-contained organization:

- ✅ **Self-registering commands** - Package init() auto-registers with registry
- ✅ **Modular organization** - Each command family in its own package
- ✅ **Type-safe interfaces** - CommandProvider interface ensures consistency
- ✅ **Custom command compatibility** - Works seamlessly with atmos.yaml commands
- ✅ **Embedded usage examples** - Markdown files embedded via go:embed

**Package Structure:**

```
cmd/version/
├── version.go                      # Parent command + VersionCommandProvider
├── list.go                         # List subcommand with spinner
├── show.go                         # Show subcommand with spinner
├── formatters.go                   # All output formatting logic
├── github.go                       # GitHubClient interface
└── markdown/
    ├── atmos_version_list_usage.md
    └── atmos_version_show_usage.md
```

**Registration in cmd/root.go:**

```go
import (
    // Blank import for side-effect registration
    _ "github.com/cloudposse/atmos/cmd/version"
)
```

### Output Formats

#### Text (Default)

```
  VERSION    DATE        TITLE
──────────────────────────────────────────────────────────────
● vtest      2025-10-17  vtest
  v1.194.1   2025-10-13  Fix and Improve Performance Heatmap
  v1.194.0   2025-10-08  Improve Atmos Auth
  v1.193.0   2025-10-03  Add Performance Profiling Heatmap
                         Visualization to Atmos CLI
```

Features:
- Green bullet (●) for current installed version
- Markdown-rendered titles with ANSI colors preserved
- Automatic word wrapping based on terminal width
- Borderless table with header separator only
- Muted gray dates

#### JSON

```json
{
  "releases": [
    {
      "version": "v1.95.0",
      "title": "Enhanced Vendoring and Bug Fixes",
      "published_at": "2025-04-15T10:30:00Z",
      "url": "https://github.com/cloudposse/atmos/releases/tag/v1.95.0",
      "prerelease": false,
      "current": true
    }
  ]
}
```

#### YAML

```yaml
releases:
  - version: v1.95.0
    title: Enhanced Vendoring and Bug Fixes
    published_at: "2025-04-15T10:30:00Z"
    url: https://github.com/cloudposse/atmos/releases/tag/v1.95.0
    prerelease: false
    current: true
```

### Release Details View (`atmos version show`)

```bash
$ atmos version show v1.95.0

Version: v1.95.0
Published: April 15, 2025
Title: Enhanced Vendoring and Bug Fixes

Release Notes
─────────────────────────────────────────────────────────────────

## What Changed

Enhanced vendoring capabilities with support for...

Assets for darwin/arm64:
  atmos_1.95.0_darwin_arm64 (14.8 MB)
  https://github.com/cloudposse/atmos/releases/download/...
```

Features:
- Full markdown-rendered release notes
- Platform-specific asset filtering (current OS/arch only)
- Muted gray file sizes
- Blue underlined download URLs

### Error Handling

**Rate Limit Exceeded:**
```
Error: GitHub API rate limit exceeded: only 5 requests remaining,
resets at 2025-04-17T15:30:00Z (in 28m)

To increase your rate limit:
1. Set ATMOS_GITHUB_TOKEN or GITHUB_TOKEN environment variable
2. Get your token: gh auth token

Authenticated requests get 5,000 requests/hour.
```

**Terminal Too Narrow:**
```
Error: terminal too narrow: detected 35 chars, minimum required 40 chars
```

**Network Error:**
```
Error: Failed to connect to GitHub API

Please check your internet connection and try again.
```

### GitHub API Integration

**Endpoints Used:**

1. **List Releases** (pagination)
   - `GET /repos/cloudposse/atmos/releases`
   - Query params: `per_page`, `page`
   - Rate limit: Counts as 1 request per page

2. **Get Release** (details)
   - `GET /repos/cloudposse/atmos/releases/tags/{tag}`
   - Rate limit: 1 request per release

3. **Get Latest Release**
   - `GET /repos/cloudposse/atmos/releases/latest`
   - For `atmos version show` without arguments

**Authentication:**
- Optional OAuth2 token via `ATMOS_GITHUB_TOKEN` or `GITHUB_TOKEN`
- Graceful degradation to unauthenticated mode
- Clear error messages on rate limit
- Proactive rate limit checking before requests

**Filtering Logic:**
- **Drafts**: Always excluded (not published releases)
- **Prereleases**: Excluded by default, included with `--include-prereleases`
- **Current version**: Synthetically added to list if not present
- **Platform assets**: Filtered to match current OS and architecture

### Testing Strategy

#### Unit Tests

The implementation includes mocking support via the `GitHubClient` interface:

```go
type GitHubClient interface {
    GetReleases(owner, repo string, opts ReleaseOptions) ([]*github.RepositoryRelease, error)
    GetRelease(owner, repo, tag string) (*github.RepositoryRelease, error)
    GetLatestRelease(owner, repo string) (*github.RepositoryRelease, error)
}

type MockGitHubClient struct {
    Releases []*github.RepositoryRelease
    Release  *github.RepositoryRelease
    Err      error
}
```

This enables testing without actual GitHub API calls.

### Implementation Details

**Key Features Implemented:**

1. ✅ Self-contained cmd/version package following command registry pattern
2. ✅ Borderless table with lipgloss (header separator only)
3. ✅ Markdown rendering for titles with Glamour (ANSI colors preserved)
4. ✅ Terminal width detection with minimum validation (40 chars)
5. ✅ Spinner during GitHub API calls (TTY-aware using bubbletea)
6. ✅ Platform-specific asset filtering (runtime.GOOS/GOARCH matching)
7. ✅ Multiple output formats (text, JSON, YAML)
8. ✅ Current version indicator (green bullet ●)
9. ✅ GitHubClient interface for testability
10. ✅ Environment variable binding (ATMOS_GITHUB_TOKEN/GITHUB_TOKEN fallback)
11. ✅ Embedded usage markdown files (go:embed)
12. ✅ Debug logging for terminal width detection
13. ✅ Static error definitions (ErrTerminalTooNarrow, etc.)

### Success Criteria

- ✅ `atmos version list` displays releases in formatted table
- ✅ `--limit` and `--offset` pagination works correctly
- ✅ `--since` date filtering works (ISO 8601 dates)
- ✅ `--include-prereleases` flag excludes/includes prereleases
- ✅ Release titles render with markdown formatting and colors
- ✅ Current installed version marked with green bullet
- ✅ `atmos version show <version>` displays single release
- ✅ `atmos version show` (no args) displays latest release
- ✅ Assets filtered to current platform only
- ✅ All output formats work (text, json, yaml)
- ✅ GitHub token authentication works with fallback
- ✅ Rate limiting handled gracefully with helpful errors
- ✅ Terminal width detection prevents broken tables
- ✅ Spinner shows during network operations (when TTY)
- ✅ Documentation includes embedded usage examples
- ✅ All linting passes
- ✅ Follows all MANDATORY conventions from CLAUDE.md

### Future Enhancements

#### `atmos version diff` (Future PR)
Compare changes between two releases with side-by-side comparison.

### Security Considerations

1. **Token Storage**: Never log or display GitHub tokens
2. **URL Validation**: Only show official GitHub download URLs
3. **Rate Limit Protection**: Proactive checking before requests
4. **Error Handling**: Clear, actionable error messages

### Backward Compatibility

- ✅ `atmos version` continues to work unchanged
- ✅ `atmos version --check` continues to work unchanged
- ✅ No breaking changes to existing commands
- ✅ New subcommands are additive only

### References

- [GitHub REST API: Releases](https://docs.github.com/en/rest/releases/releases)
- [Charmbracelet Bubbletea](https://github.com/charmbracelet/bubbletea)
- [Charmbracelet Lipgloss](https://github.com/charmbracelet/lipgloss)
- [Charmbracelet Glamour](https://github.com/charmbracelet/glamour)
- [Atmos Version Checking](https://atmos.tools/cli/commands/version)
