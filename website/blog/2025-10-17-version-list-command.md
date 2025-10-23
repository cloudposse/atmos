---
slug: version-list-show-commands
title: "Browse and Explore Atmos Releases from Your Terminal"
authors: [osterman]
tags: [feature, atmos-core]
date: 2025-10-17
---

We're introducing two new commands for exploring Atmos releases: `atmos version list` and `atmos version show`. Browse release history with date filtering, inspect artifacts, and keep your infrastructure tooling up-to-date—all from your terminal with beautiful formatted output.

<!--truncate-->

## What's New

### `atmos version list`: Browse All Releases

The new `atmos version list` command displays recent Atmos releases in a clean, formatted table:

```bash
$ atmos version list
```

**Features:**
- 📋 **Clean table view** - Borderless table with header separator
- 📖 **Markdown-rendered titles** - Release titles displayed with proper formatting and colors
- 📅 **Date filtering** - Filter releases with `--since` (ISO 8601 dates)
- 📄 **Pagination support** - Browse through extensive release history with `--limit` and `--offset`
- ✨ **Current version indicator** - Green bullet (●) marks your installed version
- 🔄 **Spinner feedback** - Visual feedback during GitHub API calls
- 📱 **Terminal width detection** - Automatically adapts to your terminal size

### `atmos version show`: Dive into Release Details

Want to see what's in a specific release? Use `atmos version show`:

```bash
$ atmos version show v1.95.0
```

This displays:
- **Full release notes** rendered in Markdown with colors preserved
- **Release metadata** (version, publication date, title)
- **Platform-specific artifacts** - Only shows assets matching your OS and architecture
- **File sizes and download URLs** - Styled links for easy access

```bash
# View the latest release
$ atmos version show

# View a specific version
$ atmos version show v1.95.0

# Works without 'v' prefix too
$ atmos version show 1.95.0
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
# View releases in a formatted table
# Read release notes with 'atmos version show'
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

# Inspect release artifacts and download URLs
atmos version show v1.95.0
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
# Show releases since specific date (ISO 8601 format)
$ atmos version list --since 2025-01-01
```

Include prerelease versions (beta, alpha, rc):

```bash
# By default, only stable releases are shown
# Use this flag to include prereleases
$ atmos version list --include-prereleases
```

### Machine-Readable Output

Perfect for scripting:

```bash
# JSON output
$ atmos version list --format json

# YAML output
$ atmos version list --format yaml
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
      "current": true
    }
  ]
}
```

## Performance & Rate Limits

### GitHub API Rate Limits

**Without authentication:** 60 requests/hour
**With authentication:** 5,000 requests/hour

To increase your rate limit, set a GitHub token:

```bash
# Get your token from GitHub CLI
export ATMOS_GITHUB_TOKEN=$(gh auth token)

# Or set GITHUB_TOKEN directly
export GITHUB_TOKEN="ghp_xxxxxxxxxxxx"
```

No special scopes are needed for public repositories.

## Examples

### Find Recent Features

```bash
# List last 5 releases
$ atmos version list --limit 5

# View details on the latest
$ atmos version show
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
# View release details with platform-specific artifacts
$ atmos version show v1.95.0

# See artifact sizes and download URLs
```

## Technical Details

### Implementation Highlights

- **Formatted table output** using [Charmbracelet lipgloss/table](https://github.com/charmbracelet/lipgloss) with automatic word wrapping
- **Markdown rendering** powered by [Glamour](https://github.com/charmbracelet/glamour) preserving ANSI colors
- **Loading spinner** built with [Charmbracelet Bubbletea](https://github.com/charmbracelet/bubbletea) for TTY detection
- **GitHub API integration** using [go-github](https://github.com/google/go-github) with OAuth2 authentication
- **Platform-specific filtering** matches assets to runtime.GOOS and runtime.GOARCH
- **Terminal width detection** using Atmos's existing utilities
- **Command registry pattern** for modular organization

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
atmos version show
```

## Get Involved

We're building Atmos in the open and welcome your feedback:
- 💬 **Discuss** - Share thoughts in [GitHub Discussions](https://github.com/cloudposse/atmos/discussions)
- 🐛 **Report Issues** - Found a bug? [Open an issue](https://github.com/cloudposse/atmos/issues)
- 🚀 **Contribute** - Want to add features? Review our [contribution guide](https://atmos.tools/community/contributing).

---

**Want to learn more?** Read the full [Version List Command PRD](https://github.com/cloudposse/atmos/blob/main/docs/prd/version-list-command.md) for detailed technical information.
