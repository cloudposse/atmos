---
slug: macos-xdg-cli-conventions
title: 'Breaking Change: macOS Now Uses ~/.config for XDG Paths'
authors:
  - osterman
tags:
  - breaking-change
date: 2025-10-24T00:00:00.000Z
release: v1.196.0
---

Atmos now follows CLI tool conventions on macOS, using `~/.config`, `~/.cache`, and `~/.local/share` instead of `~/Library/Application Support`. This ensures seamless integration with Geodesic and consistency with other DevOps tools.

<!--truncate-->

## What Changed

Starting with this release, Atmos on **macOS** uses different default paths for XDG Base Directory Specification:

**Before:**
- Config: `~/Library/Application Support/atmos/`
- Cache: `~/Library/Caches/atmos/`
- Data: `~/Library/Application Support/atmos/`

**After:**
- Config: `~/.config/atmos/`
- Cache: `~/.cache/atmos/`
- Data: `~/.local/share/atmos/`

**Note:** This only affects macOS. Linux and Windows paths remain unchanged.

## Why This Change?

### The Problem

When we implemented XDG Base Directory Specification support, we used the `github.com/adrg/xdg` library which defaults to `~/Library/Application Support` on macOS. This follows macOS conventions for **GUI applications**.

However, this created problems:

1. **Geodesic Incompatibility**: Geodesic mounts `~/.config` by default, not `~/Library/Application Support`
2. **Ecosystem Inconsistency**: Other CLI tools (gh, git, packer, stripe, op, kubectl, docker, terraform) all use `~/.config` on macOS
3. **Platform Fragmentation**: Different paths on Linux vs macOS made cross-platform workflows confusing

### CLI Tools vs GUI Applications

Research into the CLI tool ecosystem revealed a clear pattern:

**CLI Tools** (command-line only):
- Use `~/.config`, `~/.cache`, `~/.local/share` on **all platforms** including macOS
- Examples: GitHub CLI (`gh`), HashiCorp Packer, Stripe CLI, 1Password CLI
- Benefits: Consistent paths across Linux/macOS, works with containerized environments

**GUI Applications** (native Mac apps):
- Use `~/Library/Application Support`, `~/Library/Caches`
- Provides better macOS system integration
- Standard for applications in `/Applications`

Since **Atmos is a CLI tool**, it should follow CLI conventions, not GUI conventions.

## Impact on Users

### Most Users Not Affected

If you're upgrading from versions **prior to v1.195.0**, you're not affected because:
- Old versions used `~/.aws/atmos/` (legacy path)
- The `~/Library/Application Support` path was never released in a stable version

### macOS Users Running Unreleased Versions

If you were using Atmos auth on macOS from the main branch between v1.195.0 and this release:

### Option 1: Use new path (recommended)
```bash
# Re-login to store credentials in new location
atmos auth login
```

### Option 2: Keep existing location
```bash
# Add to ~/.zshrc or ~/.bash_profile
export ATMOS_XDG_CONFIG_HOME="$HOME/Library/Application Support"
```

**Note**: This keeps credentials in the old location but affects **all** Atmos XDG paths (config, cache, data), not just credentials. This may cause issues with Geodesic which expects credentials in `~/.config`. We recommend Option 1 (re-login) instead.

### Option 3: Move credentials
```bash
if [ -d "$HOME/Library/Application Support/atmos" ]; then
    mkdir -p ~/.config
    mv "$HOME/Library/Application Support/atmos" ~/.config/
fi
```

## Benefits

### Seamless Geodesic Integration

Geodesic automatically mounts these directories:
- `~/.aws`
- `~/.config` ← Atmos credentials now stored here
- `~/.ssh`
- `~/.kube`
- `~/.terraform.d`

**Configuration needed for Geodesic users:** Geodesic sets system-wide XDG environment variables (`XDG_CONFIG_HOME=/etc/xdg_config_home`) that need to be overridden. Add to your Geodesic Dockerfile:

```dockerfile
# Override Geodesic's system XDG paths to use home directory
ENV ATMOS_XDG_CONFIG_HOME=$HOME/.config
ENV ATMOS_XDG_DATA_HOME=$HOME/.local/share
ENV ATMOS_XDG_CACHE_HOME=$HOME/.cache
```

This ensures Atmos credentials are stored in mounted directories (`~/.config`) rather than non-mounted system directories (`/etc/xdg_config_home`). See [Configuring Geodesic](/cli/commands/auth/tutorials/configuring-geodesic) for details.

### Consistent Cross-Platform Paths

```bash
# Linux
~/.config/atmos/aws/provider/credentials

# macOS (new)
~/.config/atmos/aws/provider/credentials

# Same path on both platforms!
```

### Ecosystem Alignment

Your `~/.config` directory now contains configuration for all your CLI tools:
```text
~/.config/
├── atmos/          # Atmos (now!)
├── gh/             # GitHub CLI
├── git/            # Git
├── packer/         # HashiCorp Packer
├── stripe/         # Stripe CLI
└── op/             # 1Password CLI
```

## Technical Implementation

We override the `adrg/xdg` library's macOS defaults using an `init()` function:

```go
func init() {
    if runtime.GOOS == "darwin" {
        xdg.ConfigHome = filepath.Join(homeDir, ".config")
        xdg.DataHome = filepath.Join(homeDir, ".local", "share")
        xdg.CacheHome = filepath.Join(homeDir, ".cache")
    }
}
```

This ensures **all code** in Atmos (even code that directly imports `github.com/adrg/xdg`) gets CLI tool conventions on macOS.

## Documentation Updates

- [Configuring Geodesic with Atmos Auth](/cli/commands/auth/tutorials/configuring-geodesic) - Simplified configuration (no setup needed!)
- [Auth Usage Guide](/cli/commands/auth/usage) - Updated with correct macOS paths.

## Migration Support

If you encounter issues:

1. Check your current credentials location:
   ```bash
   ls -la ~/Library/Application\ Support/atmos
   ls -la ~/.config/atmos
   ```

2. Open an issue on [GitHub](https://github.com/cloudposse/atmos/issues) if you need help

## References

- [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html)
- [Stack Overflow discussion on XDG equivalents on macOS](https://stackoverflow.com/questions/3373948/equivalents-of-xdg-config-home-and-xdg-data-home-on-mac-os-x)
- [Geodesic](https://github.com/cloudposse/geodesic)

---

This change aligns Atmos with CLI tool best practices and ensures seamless integration with containerized development environments. macOS users now enjoy the same consistent experience as Linux users!
