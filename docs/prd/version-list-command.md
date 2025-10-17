# Version List Command

## Overview

This document describes the design and implementation of `atmos version list`, a new subcommand that queries the GitHub API to display recent Atmos releases in an interactive terminal UI with pagination support. Users can browse releases, view detailed release notes with markdown rendering, and inspect release artifacts.

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

**As a contributor**, I want to:
- Quickly see recent releases without leaving terminal
- Verify a release was published successfully
- Check artifact sizes and names

## Solution: `atmos version list`

### Command Design

```bash
# List recent releases (default: 10, stable releases only)
atmos version list

# List specific number of releases with pagination
atmos version list --limit 20 --offset 10

# Include prerelease versions (beta, alpha, rc, etc.)
# By default, only stable releases are shown
atmos version list --include-prereleases

# Filter releases by date
atmos version list --since 2025-01-01
atmos version list --since 30d
atmos version list --since 1w

# Output in machine-readable format
atmos version list --format json
atmos version list --format yaml

# View details for a specific release
atmos version show v1.95.0
atmos version show latest

# Search release notes for specific content
atmos version search "template functions"
atmos version search "terraform validation"
```

### Interactive TUI Features

The default TUI view provides:

1. **List View**
   - Version number (e.g., `v1.95.0`)
   - Release title (markdown-rendered inline)
   - Publication date (relative: "2 days ago")
   - Status indicators:
     - `[Current]` badge for installed version
     - `[Prerelease]` badge for pre-releases
     - `[Draft]` badge for draft releases
     - `⬆ Update Available` indicator
   - Pagination info: "Showing 1-10 of 156 releases"

2. **Detail View** (press Enter on release)
   - Full release notes (markdown-rendered with glamour)
   - Release metadata:
     - Version
     - Published date
     - Author
     - GitHub URL
   - Artifact list:
     - File names
     - File sizes
     - Download counts
     - Checksums (SHA256)

3. **Navigation**
   - `j/k` or `↑/↓`: Navigate list
   - `Enter`: View release details
   - `Esc`: Back to list
   - `n/p`: Next/previous page
   - `o`: Open release in browser
   - `d`: Download artifact (prompt for selection)
   - `q` or `Ctrl+C`: Quit
   - `?`: Show help

### Flags

<dl>
  <dt><code>--limit</code>, <code>-l</code></dt>
  <dd>Number of releases to fetch per page (default: 10, max: 100)</dd>

  <dt><code>--offset</code>, <code>-o</code></dt>
  <dd>Number of releases to skip for pagination (default: 0)</dd>

  <dt><code>--since</code>, <code>-s</code></dt>
  <dd>Filter releases published on or after this date. Supports ISO 8601 dates (YYYY-MM-DD) or relative dates (30d, 1w, 6m)</dd>

  <dt><code>--include-prereleases</code></dt>
  <dd>Include prerelease versions in results (default: false)</dd>

  <dt><code>--format</code>, <code>-f</code></dt>
  <dd>Output format: <code>tui</code>, <code>json</code>, <code>yaml</code>, <code>text</code> (default: tui)</dd>

  <dt><code>--no-cache</code></dt>
  <dd>Bypass cache and fetch fresh data from GitHub (default: false)</dd>
</dl>

### Environment Variables

- `ATMOS_GITHUB_TOKEN` / `GITHUB_TOKEN`: GitHub personal access token for increased API rate limits
  - Unauthenticated: 60 requests/hour
  - Authenticated: 5,000 requests/hour
  - Required for private repositories

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    atmos version list                            │
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
│  │ - Call exec layer                                          │  │
│  └───────────────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │ show.go - Show subcommand                                 │  │
│  │ - Parse version argument                                   │  │
│  │ - Handle "latest" keyword                                  │  │
│  │ - Call exec layer                                          │  │
│  └───────────────────────────────────────────────────────────┘  │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│         internal/exec/ (Business Logic)                          │
│  - version_list.go (list functionality)                          │
│  - version_show.go (show functionality)                          │
│  - Fetch releases from GitHub API                                │
│  - Filter drafts/prereleases                                     │
│  - Compare with current version                                  │
│  - Format output (JSON/YAML/Text)                                │
│  - Launch TUI if interactive                                     │
└────────────────────────┬────────────────────────────────────────┘
                         │
            ┌────────────┴────────────┐
            ▼                         ▼
┌──────────────────────┐   ┌──────────────────────────────────────┐
│ pkg/utils/           │   │ internal/tui/version/                 │
│ github_utils.go      │   │ - model.go (bubbletea model)          │
│                      │   │ - view.go (rendering)                 │
│ - GetGitHubReleases()│   │ - keys.go (keybindings)               │
│ - GetReleaseDetails()│   │ - list_item.go (custom delegate)      │
│ - Authentication     │   │                                        │
│ - Rate limiting      │   │ Features:                              │
└──────────────────────┘   │ - List view with releases              │
                           │ - Detail view with markdown notes      │
                           │ - Artifact browser                     │
                           │ - Pagination controls                  │
                           └────────────────────────────────────────┘
```

### Command Registry Pattern Integration

The `version` command will be refactored to use Atmos's **Command Registry Pattern** (introduced in PR #1643). This provides:

- ✅ **Self-registering commands** - Package init() auto-registers with registry
- ✅ **Modular organization** - Each command family in its own package
- ✅ **Type-safe interfaces** - CommandProvider interface ensures consistency
- ✅ **Custom command compatibility** - Works seamlessly with atmos.yaml commands

**Package Structure:**

```
cmd/version/
├── version.go          # Parent command + VersionCommandProvider
├── version_test.go     # Tests for provider
├── list.go             # List subcommand
├── list_test.go        # List tests
├── show.go             # Show subcommand
└── show_test.go        # Show tests
```

**Registration in cmd/root.go:**

```go
import (
    // Blank import for side-effect registration
    _ "github.com/cloudposse/atmos/cmd/version"
)
```

The `version` command package registers itself during initialization, and `internal.RegisterAll()` adds it to RootCmd.

### Output Formats

#### TUI (Default)

Interactive terminal UI with list and detail views (described above).

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
      "current": true,
      "artifacts": [
        {
          "name": "atmos_1.95.0_darwin_amd64.tar.gz",
          "size": 15728640,
          "download_count": 1523,
          "url": "https://github.com/..."
        }
      ]
    }
  ],
  "pagination": {
    "limit": 10,
    "offset": 0,
    "total": 156
  }
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
    artifacts:
      - name: atmos_1.95.0_darwin_amd64.tar.gz
        size: 15728640
        download_count: 1523
        url: https://github.com/...
pagination:
  limit: 10
  offset: 0
  total: 156
```

#### Text

```
v1.95.0  [Current]  Enhanced Vendoring and Bug Fixes  (Apr 15, 2025)
v1.94.0             Performance Improvements          (Apr 8, 2025)
v1.93.0             New Template Functions            (Apr 1, 2025)
```

### Release Details View (`atmos version show`)

Additional subcommand for viewing a single release:

```bash
$ atmos version show v1.95.0

╭─────────────────────────────────────────────────────────────╮
│ Atmos v1.95.0                                               │
│ Published: April 15, 2025 (2 days ago)                     │
│ Author: @osterman                                           │
│ URL: https://github.com/cloudposse/atmos/releases/tag/...  │
╰─────────────────────────────────────────────────────────────╯

Release Notes
─────────────

## What Changed

Enhanced vendoring capabilities with support for...

## Artifacts (8 files)

atmos_1.95.0_darwin_amd64.tar.gz        15.0 MB  1,523 downloads
atmos_1.95.0_darwin_arm64.tar.gz        14.8 MB    987 downloads
atmos_1.95.0_linux_amd64.tar.gz         15.2 MB  2,341 downloads
...

[o] Open in browser  [d] Download artifact  [q] Quit
```

### Caching Strategy

To minimize API calls and respect rate limits:

1. **Cache location**: `~/.atmos/cache/releases.json`
2. **Cache TTL**: 1 hour (configurable via `ATMOS_RELEASE_CACHE_TTL`)
3. **Cache key**: `releases:{owner}:{repo}:{limit}:{offset}:{include_prereleases}`
4. **Invalidation**: `--no-cache` flag or expired TTL
5. **Cache format**: JSON with metadata

```json
{
  "cached_at": "2025-04-17T14:30:00Z",
  "expires_at": "2025-04-17T15:30:00Z",
  "query": {
    "limit": 10,
    "offset": 0,
    "include_prereleases": false
  },
  "data": [...]
}
```

### Error Handling

**Rate Limit Exceeded:**
```
Error: GitHub API rate limit exceeded (60 requests/hour for unauthenticated requests)

To increase your rate limit:
1. Set ATMOS_GITHUB_TOKEN or GITHUB_TOKEN environment variable
2. Create a token at https://github.com/settings/tokens

Authenticated requests get 5,000 requests/hour.
```

**Network Error:**
```
Error: Failed to connect to GitHub API

Please check your internet connection and try again.
Use --no-cache to bypass cache if outdated.
```

**Invalid Token:**
```
Error: GitHub authentication failed

Your ATMOS_GITHUB_TOKEN appears to be invalid or expired.
Generate a new token at https://github.com/settings/tokens
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
   - Special handling for `atmos version show latest`

**Authentication:**
- Optional OAuth2 token via `ATMOS_GITHUB_TOKEN` or `GITHUB_TOKEN`
- Graceful degradation to unauthenticated mode
- Clear error messages on rate limit

**Response Parsing:**
- Parse GitHub API JSON responses
- Extract: tag_name, name, body, published_at, prerelease, draft, assets
- Transform to internal schema

**Filtering Logic:**
- **Drafts**: Always excluded (not published releases)
- **Prereleases**: Excluded by default, included with `--include-prereleases` flag
- **Stable releases**: Always included
- Filter applied after fetching from GitHub API to preserve pagination accuracy

### Testing Strategy

#### Unit Tests (Target: 85%+ coverage)

1. **GitHub API Client** (`pkg/utils/github_utils_test.go`)
   - Mock GitHub API responses
   - Test pagination logic
   - Test authentication (with/without token)
   - Test rate limit handling
   - Test error scenarios
   - Test filtering (drafts, prereleases)

2. **Business Logic** (`internal/exec/version_list_test.go`)
   - Test output formatting (JSON, YAML, text)
   - Test version comparison logic
   - Test current version detection
   - Test artifact parsing
   - Mock GitHub client

3. **CLI Command** (`cmd/version_list_test.go`)
   - Test flag parsing
   - Test validation (limit range, offset >= 0)
   - Test command registration
   - Mock exec layer

4. **TUI Components** (`internal/tui/version/*_test.go`)
   - Test model state transitions
   - Test keybindings
   - Test pagination logic
   - Test list item rendering
   - Mock bubbletea infrastructure

#### Integration Tests

1. **Real GitHub API** (`tests/version_list_integration_test.go`)
   - Use `tests.RequireGitHubAccess()` precondition
   - Test actual API calls with token
   - Test pagination with real data
   - Skip if rate limits low

2. **End-to-End CLI** (`tests/version_list_cli_test.go`)
   - Build atmos binary
   - Execute `atmos version list --format json`
   - Parse and validate output
   - Test all flags

### Implementation Phases

#### Phase 0: Refactor Version Command to Use Registry Pattern (2-3 hours)

**Critical first step:** Migrate existing `cmd/version.go` to command registry pattern.

**Steps:**
1. Create `cmd/version/` package directory
2. Move `cmd/version.go` → `cmd/version/version.go`
3. Implement `VersionCommandProvider` struct
4. Add provider methods: `GetCommand()`, `GetName()`, `GetGroup()`
5. Register in `init()` via `internal.Register()`
6. Add blank import to `cmd/root.go`: `_ "github.com/cloudposse/atmos/cmd/version"`
7. Migrate existing flags (`--check`, `--format`)
8. Move `internal/exec/version.go` logic (no changes needed)
9. Create `cmd/version/version_test.go` for provider tests
10. Verify existing functionality: `atmos version`, `atmos version --check`

**Files created/modified:**
- `cmd/version/version.go` (new - migrated from cmd/version.go)
- `cmd/version/version_test.go` (new - provider tests)
- `cmd/root.go` (modify - add blank import)
- `cmd/version.go` (delete - replaced by package)

#### Phase 1: GitHub API Integration (2-3 hours)
- Extend `pkg/utils/github_utils.go`
- Add `GetGitHubRepoReleases()` with pagination
  - `includePrereleases` parameter (default: false)
  - Always filter out drafts
  - Return stable releases by default
- Add `GetGitHubReleaseByTag()` for details
- Add `GetGitHubLatestRelease()` for "latest" keyword (stable releases only)
- Environment variable binding (viper): `ATMOS_GITHUB_TOKEN` / `GITHUB_TOKEN`
- Rate limit handling with descriptive errors
- Unit tests with mocked GitHub client
  - Test prerelease filtering (included/excluded)
  - Test draft exclusion (always)

#### Phase 2: Caching Layer (1-1.5 hours)
- Create `pkg/cache/release_cache.go`
- Implement cache with TTL (default: 1 hour)
- Cache location: `~/.atmos/cache/releases/`
- Support `--no-cache` flag
- Unit tests for cache operations

#### Phase 3: TUI Implementation (4-5 hours)
- Create `internal/tui/version/` package
- Implement bubbletea model with state machine
- List view with custom delegate
- Detail view with markdown rendering (glamour)
- Artifact browser view
- Pagination controls
- Keybindings (j/k, n/p, o, d, q)
- Use theme colors from `pkg/ui/theme/colors.go`
- Unit tests for state management

#### Phase 4: Business Logic - List Command (2-3 hours)
- Implement `internal/exec/version_list.go`
- Fetch releases with caching support
- Filter drafts/prereleases based on flags
- **Date filtering implementation (`--since`)**
  - Parse ISO 8601 dates (YYYY-MM-DD)
  - Parse relative dates (30d, 1w, 6m)
  - Filter releases by publishedAt date
- Output formatters (JSON, YAML, text, TUI)
- Version comparison logic
- Launch TUI for interactive mode
- Unit tests with mocked dependencies

#### Phase 5: Business Logic - Show Command (1.5-2 hours)
- Implement `internal/exec/version_show.go`
- Fetch single release details
- Handle "latest" keyword special case
- Reuse TUI detail view
- Support all output formats
- Unit tests

#### Phase 6: Business Logic - Search Command (2-2.5 hours)
- Implement `internal/exec/version_search.go`
- Fetch all releases (with caching)
- Full-text search across release notes and titles
- Case-insensitive search by default
- Optional case-sensitive flag
- Context lines around matches
- Highlight matches in TUI
- Support all output formats
- Unit tests with mock data

#### Phase 7: CLI Commands (3-3.5 hours)
- Create `cmd/version/list.go` (subcommand)
  - Flags: `--limit`, `--offset`, `--since`, `--include-prereleases`, `--format`, `--no-cache`
  - Validation: limit 1-100, offset >= 0, date parsing
  - Attach to version parent command
- Create `cmd/version/show.go` (subcommand)
  - Argument: version string or "latest"
  - Flag: `--format`
  - Attach to version parent command
- Create `cmd/version/search.go` (subcommand)
  - Argument: search query
  - Flags: `--case-sensitive`, `--context`, `--format`
  - Attach to version parent command
- Unit tests for all three subcommands

#### Phase 8: Documentation (2.5-3 hours)
- Create `website/docs/cli/commands/version/list.mdx`
- Create `website/docs/cli/commands/version/show.mdx`
- Create `website/docs/cli/commands/version/search.mdx`
- Update `website/docs/cli/commands/version.mdx` (add subcommand links)
- Add usage examples and screenshots
- Build website: `cd website && npm run build`
- Fix any broken links or errors

#### Phase 9: Integration & Testing (3-4 hours)
- Create integration tests in `tests/`
  - `version_list_integration_test.go` (with real GitHub API)
  - `version_list_cli_test.go` (CLI execution tests)
  - Use `tests.RequireGitHubAccess()` precondition
- Manual testing checklist (all formats, pagination, caching)
- Test with/without GitHub token
- Verify rate limit handling
- Run full test suite: `make testacc-cover`
- Verify 80%+ coverage on new/changed lines

**Total Estimated Time: 22-30 hours**

### Success Criteria

- ✅ Version command migrated to command registry pattern
- ✅ `atmos version list` displays releases in TUI
- ✅ `--limit` and `--offset` pagination works correctly
- ✅ `--since` date filtering works (ISO 8601 and relative dates)
- ✅ `--include-prereleases` flag excludes/includes prereleases
- ✅ Release notes render with markdown formatting
- ✅ Artifacts displayed with sizes and download counts
- ✅ `atmos version show <version>` displays single release
- ✅ `atmos version search <query>` searches release notes
- ✅ Search highlights matches in TUI
- ✅ All output formats work (tui, json, yaml, text)
- ✅ GitHub token authentication works
- ✅ Rate limiting handled gracefully
- ✅ Caching reduces API calls
- ✅ 80-90% test coverage achieved
- ✅ Documentation complete and builds successfully
- ✅ All linting passes
- ✅ Follows all MANDATORY conventions from CLAUDE.md

### Future Enhancements

#### `atmos version diff` (Future PR)
Compare changes between two releases:

```bash
# Compare two specific versions
$ atmos version diff v1.94.0 v1.95.0

# Compare with current version
$ atmos version diff v1.95.0

# Show in TUI or output formats
$ atmos version diff v1.94.0 v1.95.0 --format json
```

**Features:**
- Side-by-side comparison of release notes
- Highlight breaking changes, features, bug fixes
- Compare artifact changes (added/removed files)
- Show commit count between versions
- Visual diff in TUI with color-coded sections

#### Toolchain-Managed Features (Separate PRs)
These features will be handled by the Atmos toolchain:

- **`atmos version download`** - Download and verify release artifacts
- **`atmos version upgrade`** - Interactive upgrade wizard with rollback support

### Security Considerations

1. **Token Storage**: Never log or display GitHub tokens
2. **Cache Permissions**: Restrict cache file to user-only (0600)
3. **URL Validation**: Validate GitHub URLs before opening browser
4. **Download Verification**: Verify checksums when downloading artifacts
5. **Rate Limit Protection**: Respect GitHub rate limits, don't spam API

### Backward Compatibility

- ✅ `atmos version` continues to work unchanged
- ✅ `atmos version --check` continues to work unchanged
- ✅ No breaking changes to existing commands
- ✅ New subcommands are additive only

### Open Questions

1. **Should we support private repositories?**
   - Yes, if ATMOS_GITHUB_TOKEN is set and has access
   - Error clearly if token lacks permissions

2. **Should we cache artifact metadata?**
   - Yes, include in release cache
   - Separate cache key for artifact details

3. **Should we auto-download on select in TUI?**
   - No, require explicit action (press 'd')
   - Show download progress bar

4. **Should we support filtering by semantic version range?**
   - Not in v1, add as future enhancement
   - Example: `atmos version list --range ">=1.90.0 <2.0.0"`

### References

- [GitHub REST API: Releases](https://docs.github.com/en/rest/releases/releases)
- [Charmbracelet Bubbletea](https://github.com/charmbracelet/bubbletea)
- [Charmbracelet Glamour](https://github.com/charmbracelet/glamour)
- [Atmos Version Checking](https://atmos.tools/cli/commands/version)
