# PRD: Isolated Browser Sessions for `atmos auth console`

**Status:** Draft
**Author:** Cloud Posse
**Created:** 2026-03-18
**Parent PRD:** [auth-console-command.md](auth-console-command.md)

## Overview

Add isolated browser session support to `atmos auth console` so users can have multiple cloud provider console sessions open simultaneously — one per identity — without logout conflicts. Each identity gets its own Chrome browser context via `--user-data-dir`, eliminating the "You must first log out" friction when switching between accounts.

## Problem Statement

### User Experience Today

A user sets up `atmos auth console` and authenticates into `plat-staging/AdministratorAccess`:

```bash
atmos auth console --identity plat-staging/AdministratorAccess
```

This works great — the AWS console opens in their default browser. But when they try to open a second account:

```bash
atmos auth console --identity cards-staging/AdministratorAccess
```

AWS blocks them:

> **You must first log out before logging into a different AWS account.**
>
> To logout, click here

The user must:
1. Log out of the first AWS console session
2. Lose their place in whatever they were doing
3. Re-run the command for the second account

This also means the AWS "Switch Role" browser plugin no longer works, since federation URLs bypass the plugin's session management.

### Root Cause

All `atmos auth console` invocations open URLs in the same browser profile. AWS federation cookies are stored in that shared profile, and AWS enforces single-session per browser context.

### Why a Browser Extension Isn't the Answer

Users previously relied on the AWS Switch Role browser extension for multi-account access. While convenient, requiring a browser extension:
- Adds a dependency outside Atmos's control
- Doesn't work with federation-based authentication
- Varies by browser (Chrome, Firefox, Arc each need different extensions)
- Can't be managed or configured through `atmos.yaml`

## Solution

Launch each console session in an isolated Chrome browser context using `--user-data-dir` with a per-identity directory. This gives each identity its own cookies, storage, and session state.

```bash
# These two commands open in separate, isolated browser windows
atmos auth console --identity plat-staging/AdministratorAccess --isolated
atmos auth console --identity cards-staging/AdministratorAccess --isolated
```

Both sessions run simultaneously without conflict.

### How It Works

Under the hood, instead of:
```bash
# Current: opens in default browser profile (shared cookies)
open "https://signin.aws.amazon.com/federation?..."
```

Atmos launches:
```bash
# New: opens in isolated Chrome profile (per-identity cookies)
open -na "Google Chrome" --args \
  --user-data-dir=~/.local/share/atmos/console/sessions/<hash> \
  "https://signin.aws.amazon.com/federation?..."
```

Each identity gets a deterministic directory based on the realm and identity name. Reopening the same identity reuses its session (no re-login needed within the AWS session lifetime). Different identities are fully isolated.

## User Stories

### US-1: Multi-Account Console Access
**As** a developer working across multiple AWS accounts
**I want** to have multiple AWS console sessions open simultaneously
**So that** I can work in plat-staging and cards-staging at the same time without logging out

### US-2: Persistent Session Reuse
**As** a developer who frequently opens the same account's console
**I want** my browser session to persist across `atmos auth console` invocations
**So that** I don't have to re-authenticate when reopening the same identity

### US-3: Team-Wide Configuration
**As** a platform team lead
**I want** to enable isolated sessions for my entire team via `atmos.yaml`
**So that** everyone gets multi-account support by default without passing extra flags

## Configuration

### Global Console Settings

```yaml
auth:
  console:
    isolated_sessions: true  # Enable isolated browser sessions for all identities
```

### CLI Flag Override

```bash
# Per-invocation override (takes precedence over config)
atmos auth console --identity prod-admin --isolated

# Config says true, but skip isolation for this one
atmos auth console --identity prod-admin --no-isolated
```

### Precedence

1. `--isolated` / `--no-isolated` CLI flag (highest)
2. `auth.console.isolated_sessions` in `atmos.yaml`
3. Default: `false` (backward-compatible)

## Platform Support

Isolated sessions require a Chromium-based browser (Chrome or Chromium). The feature is supported on every platform where Chrome can be launched with `--user-data-dir`:

| Platform | Launch Mechanism | Status |
|----------|-----------------|--------|
| macOS | `open -na "Google Chrome" --args --user-data-dir=<dir> <url>` | Supported |
| Linux | `google-chrome --user-data-dir=<dir> <url>` | Supported |
| Windows | `chrome.exe --user-data-dir=<dir> <url>` | Supported |

**Graceful fallback:** If Chrome/Chromium is not installed, Atmos warns the user and falls back to the default browser (existing behavior). The feature degrades gracefully — it doesn't block console access.

### Chrome Detection

Per platform:
- **macOS:** `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`, then Chromium
- **Linux:** `google-chrome`, `google-chrome-stable`, `chromium-browser`, `chromium` (via `PATH`)
- **Windows:** Program Files paths, `%LocalAppData%\Google\Chrome\Application\chrome.exe`

## Session Directory Strategy

Session directories are stored under XDG data paths for consistency with credential storage:

```
~/.local/share/atmos/console/sessions/<hash>/
```

Where `<hash>` is derived from `sha256(realm + "/" + identityName)[:8]`.

**Why realm + identity?**
- The realm provides credential isolation between repositories (same identity name in different repos gets different sessions)
- The identity name provides per-account isolation
- Together they match the same isolation boundary used by the keyring credential store

**Why deterministic (not UUID)?**
- Reopening the same identity reuses the Chrome profile, so the user stays logged in within the AWS session lifetime
- No accumulation of abandoned temp directories
- Users can clean up with `rm -rf ~/.local/share/atmos/console/sessions/`

## Trade-offs

| Trade-off | Impact | Mitigation |
|-----------|--------|------------|
| No bookmarks/extensions in isolated sessions | Users lose browser customizations | These are temporary console sessions, not daily browsing |
| Chrome-only (initially) | Firefox/Safari users can't use isolation | Graceful fallback to default browser; Chrome is the most common browser |
| Disk usage from Chrome profiles | ~50-100MB per identity profile | Stored in XDG data dir; users can clean up manually |
| Cold start slower than shared profile | First launch per identity takes a few seconds longer | Subsequent launches reuse the profile and are fast |

## Technical Design

### New Package: `pkg/browser/`

```go
// Opener opens URLs in a browser.
type Opener interface {
    Open(url string) error
}

// Option configures browser opening behavior.
type Option func(*config)

// WithIsolatedSession enables isolated browser sessions keyed by realm+identity.
func WithIsolatedSession(realm, identityName string) Option

// New returns an Opener configured with the given options.
func New(opts ...Option) Opener
```

### Schema Addition

```go
// AuthConsoleConfig defines global console behavior settings.
type AuthConsoleConfig struct {
    IsolatedSessions *bool `yaml:"isolated_sessions,omitempty" json:"isolated_sessions,omitempty" mapstructure:"isolated_sessions"`
}
```

Added to `AuthConfig.Console` (top-level, not per-provider).

### Files

| File | Purpose |
|------|---------|
| `pkg/browser/browser.go` | Interface, options, factory |
| `pkg/browser/detect.go` | Cross-platform Chrome detection |
| `pkg/browser/isolated.go` | Isolated session opener |
| `pkg/browser/browser_test.go` | Unit tests |
| `pkg/schema/schema_auth.go` | AuthConsoleConfig struct |
| `cmd/auth_console.go` | --isolated flag, opener integration |

## Testing Strategy

### Unit Tests
- Session directory hashing: same realm+identity produces same dir, different produces different
- Chrome detection: mock `exec.LookPath` and filesystem checks
- Command construction: verify correct args per platform without launching
- Fallback: verify graceful degradation when Chrome is not found

### Integration Tests
- Flag registration and precedence (flag > config > default)
- End-to-end with `--print-only` to verify URL generation still works

## Implementation Plan

### Phase 1: Core
1. Add `ErrChromeNotFound` sentinel error
2. Add `AuthConsoleConfig` to schema
3. Create `pkg/browser/` package

### Phase 2: Integration
1. Add `--isolated` flag to `cmd/auth_console.go`
2. Wire up `browser.Opener` in `handleBrowserOpen`
3. Update usage examples

### Phase 3: Documentation
1. Update CLI docs
2. Update JSON schema

## Success Criteria

1. Users can open multiple AWS console sessions simultaneously with `--isolated`
2. Same identity reuses its browser session across invocations
3. Chrome not installed → graceful fallback with helpful warning
4. Zero breaking changes to existing `atmos auth console` behavior
5. Works on macOS and Linux; Windows best-effort

## References

- [Parent PRD: Web Console Access](auth-console-command.md)
- [AWS Federation Console Access](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_enable-console-custom-url.html)
- [Chrome --user-data-dir documentation](https://www.chromium.org/developers/creating-and-using-profiles/)
- [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/latest/)
