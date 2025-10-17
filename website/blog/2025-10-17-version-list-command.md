---
slug: version-list-show-search-commands
title: "Browse, Search, and Explore Atmos Releases from Your Terminal"
authors: [atmos]
tags: [feature, atmos-core]
---

We're introducing three powerful new commands for exploring Atmos releases: `atmos version list`, `atmos version show`, and `atmos version search`. Browse release history with date filtering, search through release notes, inspect artifacts, and keep your infrastructure tooling up-to-date—all from a beautiful interactive terminal interface.

<!--truncate-->

## What's New

### `atmos version list`: Browse All Releases

The new `atmos version list` command displays recent Atmos releases in an interactive terminal UI powered by [Charmbracelet](https://charm.sh/):

```bash
$ atmos version list
```

![Atmos Version List TUI](./img/version-list-tui.png)

**Features:**
- 📋 **Interactive list view** - Navigate with keyboard or mouse
- 📖 **Markdown-rendered titles** - Release titles displayed with proper formatting
- 📅 **Date filtering** - Filter releases with `--since` (ISO dates or relative like "30d")
- 📄 **Pagination support** - Browse through extensive release history
- ✨ **Status indicators** - Clearly see current version, prereleases, and updates
- 🔍 **Release details** - Press Enter to view full release notes and artifacts

### `atmos version show`: Dive into Release Details

Want to see what's in a specific release? Use `atmos version show`:

```bash
$ atmos version show v1.95.0
```

This displays:
- **Full release notes** rendered in beautiful markdown
- **Release metadata** (author, publication date, GitHub URL)
- **Artifact list** with file sizes and download counts
- **Quick actions** to open in browser or download artifacts

```bash
# View the latest release
$ atmos version show latest

# View a specific version
$ atmos version show v1.95.0
```

### `atmos version search`: Find Releases by Content

Need to find when a specific feature was added or bug was fixed? Search through all release notes:

```bash
$ atmos version search "template functions"
```

**Features:**
- 🔍 **Full-text search** - Search across all release notes and titles
- 💡 **Highlighted matches** - See matches highlighted in TUI
- 📝 **Context display** - View surrounding text around matches
- ⚙️ **Case control** - Case-insensitive by default, optional `--case-sensitive`
- 📊 **Multiple formats** - Output as JSON, YAML, or interactive TUI

```bash
# Find releases mentioning Terraform
$ atmos version search "terraform validation"

# Case-sensitive search
$ atmos version search "AWS" --case-sensitive

# Show context around matches
$ atmos version search "bug fix" --context 3
```

## Why This Matters

### For Platform Engineers

Before, discovering Atmos releases meant context-switching to GitHub:

```bash
# Old workflow
$ atmos version
👽 Atmos 1.94.0 on darwin/arm64

# Now open browser, navigate to GitHub releases...
# Scroll through releases, click around...
# Copy version number...
```

Now, everything stays in your terminal:

```bash
# New workflow
$ atmos version list
# Interactive TUI opens
# Navigate with j/k, press Enter for details
# Read release notes, see artifacts
# All without leaving your terminal
```

### For Infrastructure Teams

**Release Auditing:**
```bash
# Export release data for compliance
atmos version list --format json > releases.json

# Script version discovery in CI/CD
VERSION=$(atmos version list --format json | jq -r '.releases[0].version')
```

**Changelog Review:**
```bash
# Quickly review recent changes before upgrading
atmos version list --limit 5

# Compare current version to latest
atmos version show latest
```

### For Contributors

**Verify Releases:**
```bash
# Check that your release published correctly
atmos version show v1.95.0

# Inspect release artifacts
atmos version show v1.95.0
# Press 'd' to download an artifact
```

## How to Use It

### Basic Usage

List the last 10 releases (default):

```bash
$ atmos version list
```

List more releases with pagination:

```bash
# Show 20 releases
$ atmos version list --limit 20

# Skip first 10, show next 10
$ atmos version list --limit 10 --offset 10
```

Filter by date:

```bash
# Show releases from last 30 days
$ atmos version list --since 30d

# Show releases since specific date
$ atmos version list --since 2025-01-01

# Show releases from last week
$ atmos version list --since 1w
```

Include prerelease versions (beta, alpha, rc):

```bash
# By default, only stable releases are shown
# Use this flag to include prereleases
$ atmos version list --include-prereleases
```

Search release notes:

```bash
# Find releases mentioning specific features
$ atmos version search "template functions"

# Search with context
$ atmos version search "bug fix" --context 3
```

### Machine-Readable Output

Perfect for scripting and CI/CD:

```bash
# JSON output
$ atmos version list --format json

# YAML output
$ atmos version list --format yaml

# Plain text (one per line)
$ atmos version list --format text
```

**Example JSON output:**

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
          "download_count": 1523
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

### Interactive TUI Navigation

The interactive terminal UI supports:

| Key | Action |
|-----|--------|
| `↑/↓` or `j/k` | Navigate list |
| `Enter` | View release details |
| `Esc` | Back to list |
| `n/p` | Next/previous page |
| `o` | Open release in browser |
| `d` | Download artifact |
| `q` or `Ctrl+C` | Quit |
| `?` | Show help |

## Performance & Rate Limits

### Caching

To minimize GitHub API calls, release data is cached for 1 hour:

```bash
# Use cached data
$ atmos version list

# Force fresh data
$ atmos version list --no-cache
```

Cache location: `~/.atmos/cache/releases.json`

### GitHub API Rate Limits

**Without authentication:** 60 requests/hour
**With authentication:** 5,000 requests/hour

To increase your rate limit, set a GitHub token:

```bash
export ATMOS_GITHUB_TOKEN="ghp_xxxxxxxxxxxx"
# or
export GITHUB_TOKEN="ghp_xxxxxxxxxxxx"
```

[Generate a token at GitHub](https://github.com/settings/tokens) (no special scopes needed for public repositories).

## Examples

### Find Recent Features

```bash
# List last 5 releases
$ atmos version list --limit 5

# View details on the latest
$ atmos version show latest
```

### Audit Release History for Compliance

```bash
# Export all releases to JSON
$ atmos version list --limit 100 --format json > all-releases.json

# Parse with jq for specific info
$ atmos version list --format json | \
  jq -r '.releases[] | "\(.version) - \(.published_at)"'
```

### Check Release Artifacts Before Downloading

```bash
# View release details
$ atmos version show v1.95.0

# See artifact sizes and download counts
# Press 'd' to download interactively
```

### CI/CD Integration

```bash
# Get latest version in CI pipeline
LATEST_VERSION=$(atmos version list --format json | \
  jq -r '.releases[0].version')

echo "Latest Atmos version: $LATEST_VERSION"

# Check if update available
CURRENT_VERSION=$(atmos version --format json | jq -r '.version')
if [ "$CURRENT_VERSION" != "$LATEST_VERSION" ]; then
  echo "Update available: $CURRENT_VERSION -> $LATEST_VERSION"
fi
```

## What's Coming Next

### Release Comparison (Future PR)

We're planning to add `atmos version diff` to compare changes between releases:

```bash
# Compare two specific versions
$ atmos version diff v1.94.0 v1.95.0

# Compare with current version
$ atmos version diff v1.95.0
```

Side-by-side comparison with highlighted breaking changes, new features, and bug fixes.

### Toolchain Features (Separate PRs)

Version management features will be handled by the Atmos toolchain:

- **`atmos version download`** - Download and verify release artifacts with checksum validation
- **`atmos version upgrade`** - Interactive upgrade wizard with rollback support

Want to see something else? [Share your ideas in GitHub Discussions](https://github.com/cloudposse/atmos/discussions)!

## Technical Details

### Implementation Highlights

- **Interactive TUI** built with [Charmbracelet Bubbletea](https://github.com/charmbracelet/bubbletea)
- **Markdown rendering** powered by [Glamour](https://github.com/charmbracelet/glamour)
- **GitHub API integration** using [go-github](https://github.com/google/go-github)
- **Smart caching** to respect API rate limits
- **Full test coverage** (85%+) with unit and integration tests

### For Developers

If you're interested in the implementation details, check out:
- **[PRD: Version List Command](https://github.com/cloudposse/atmos/blob/main/docs/prd/version-list-command.md)** - Complete design document
- **[CLI Documentation](https://atmos.tools/cli/commands/version/list)** - Usage reference

## Try It Now

Upgrade to the latest Atmos release and try it yourself:

```bash
# Check your current version
atmos version

# Browse available releases
atmos version list

# View details on latest
atmos version show latest
```

## Get Involved

We're building Atmos in the open and welcome your feedback:
- 💬 **Discuss** - Share thoughts in [GitHub Discussions](https://github.com/cloudposse/atmos/discussions)
- 🐛 **Report Issues** - Found a bug? [Open an issue](https://github.com/cloudposse/atmos/issues)
- 🚀 **Contribute** - Want to add features? Check out our [contribution guide](https://atmos.tools/community/contributing)

---

**Want to learn more?** Read the full [Version List Command PRD](https://github.com/cloudposse/atmos/blob/main/docs/prd/version-list-command.md) for detailed technical information.
