# SAML Browser Driver Integration

## Overview

This document defines how Atmos integrates with the playwright-go browser automation library for SAML authentication, including automatic driver download, cache management, and testing strategy.

## Problem Statement

SAML authentication through Google Workspace and similar identity providers requires browser automation to handle the interactive authentication flow. The saml2aws library uses playwright-go for this automation, which requires:

1. Chromium browser driver installation (~140 MB)
2. Proper cache directory management
3. Environment-specific driver detection
4. Clear user guidance on installation options

Initial implementation had several issues:
- `download_browser_driver: true` configuration was set on `IDPAccount` but not propagated to `LoginDetails`, causing downloads to fail
- Incorrect cache directory paths (`ms-playwright-go` instead of `ms-playwright`) broke driver detection
- Documentation instructed manual installation using wrong command (`go run playwright install`)
- No integration tests validated actual driver download functionality

## Solution: Automatic Driver Download with Fallback Detection

### Architecture Decisions

**Automatic Download as Primary Method**
- Default to automatic driver download for best user experience
- Playwright-go handles all platform-specific installation details
- Drivers cached in standard platform locations (no PATH configuration needed)
- Users configure via simple `download_browser_driver: true` setting

**Manual Installation Discouraged**
- Manual installation requires understanding playwright-go vs playwright CLI differences
- Different tools install to different cache directories
- Documentation warns users this requires advanced knowledge
- Only recommended for specialized environments with driver pre-provisioning

**LoginDetails as Source of Truth**
- saml2aws checks `LoginDetails.DownloadBrowser`, not `IDPAccount.DownloadBrowser`
- Atmos must set both fields to ensure configuration works
- Provider's `shouldDownloadBrowser()` determines value from config

### Implementation Details

#### Configuration Propagation

```go
func (p *samlProvider) createLoginDetails() *creds.LoginDetails {
    return &creds.LoginDetails{
        URL:             p.url,
        Username:        p.config.Username,
        Password:        p.config.Password,
        DownloadBrowser: p.shouldDownloadBrowser(), // Critical: Must set this.
    }
}

func (p *samlProvider) shouldDownloadBrowser() bool {
    // First check explicit config setting.
    if p.config.DownloadBrowserDriver != nil {
        return *p.config.DownloadBrowserDriver
    }

    // If not configured, check if drivers already installed.
    return !p.hasPlaywrightDrivers()
}
```

**Why both IDPAccount and LoginDetails?**
- `IDPAccount.DownloadBrowser`: Used for environment variable (`SAML2AWS_DOWNLOAD_BROWSER`)
- `LoginDetails.DownloadBrowser`: Used by saml2aws authentication logic
- Both must be set to ensure downloads work

#### Cache Directory Management

Playwright-go uses hardcoded cache directories, NOT PATH:

| Platform | Cache Directory |
|----------|----------------|
| macOS | `~/Library/Caches/ms-playwright` |
| Linux | `~/.cache/ms-playwright` |
| Windows | `%USERPROFILE%\AppData\Local\ms-playwright` |

**Detection Logic**:
```go
func (p *samlProvider) hasPlaywrightDrivers() bool {
    homeDir, err := os.UserHomeDir()
    if err != nil {
        return false
    }

    // Check platform-specific cache directories.
    playwrightPaths := []string{
        filepath.Join(homeDir, ".cache", "ms-playwright"),            // Linux
        filepath.Join(homeDir, "Library", "Caches", "ms-playwright"), // macOS
        filepath.Join(homeDir, "AppData", "Local", "ms-playwright"),  // Windows
    }

    for _, path := range playwrightPaths {
        if hasValidBrowsers(path) {
            return true
        }
    }
    return false
}

func hasValidBrowsers(basePath string) bool {
    entries, err := os.ReadDir(basePath)
    if err != nil {
        return false
    }

    // Look for chromium-* directories (versioned browser installations).
    for _, entry := range entries {
        if entry.IsDir() && strings.HasPrefix(entry.Name(), "chromium-") {
            chromiumPath := filepath.Join(basePath, entry.Name())

            // Verify it's not an empty placeholder directory.
            if dirHasFiles(chromiumPath) {
                return true
            }
        }
    }
    return false
}
```

**Why Check for Files?**
- Empty version directories can exist without actual browser binaries
- Must verify directory contains actual installation (160+ files for Chromium)
- Prevents false positives from incomplete installations

#### User Configuration

**Recommended Configuration (atmos.yaml)**:
```yaml
auth:
  providers:
    my-saml:
      kind: aws/saml
      url: https://sso.example.com/saml
      download_browser_driver: true  # Automatic download on first use
```

**Explicit Disable**:
```yaml
auth:
  providers:
    my-saml:
      kind: aws/saml
      url: https://sso.example.com/saml
      download_browser_driver: false  # Assumes drivers pre-installed
```

**Environment Variable Override**:
```bash
export SAML2AWS_DOWNLOAD_BROWSER=true
atmos auth login --identity my-identity
```

### Testing Strategy

#### Unit Tests

**Test LoginDetails Population**:
```go
func TestSAMLProvider_createLoginDetails_DownloadBrowser(t *testing.T) {
    tests := []struct {
        name           string
        configValue    *bool
        driversExist   bool
        expectedResult bool
    }{
        {
            name:           "explicit true overrides detection",
            configValue:    ptr.To(true),
            driversExist:   true,
            expectedResult: true,
        },
        {
            name:           "explicit false overrides detection",
            configValue:    ptr.To(false),
            driversExist:   false,
            expectedResult: false,
        },
        {
            name:           "auto-detect: download when missing",
            configValue:    nil,
            driversExist:   false,
            expectedResult: true,
        },
        {
            name:           "auto-detect: skip when present",
            configValue:    nil,
            driversExist:   true,
            expectedResult: false,
        },
    }

    // Test implementation validates LoginDetails.DownloadBrowser matches expectations.
}
```

**Test Driver Detection**:
```go
func TestSAMLProvider_hasPlaywrightDrivers(t *testing.T) {
    // Validate detection logic across different scenarios:
    // - Empty cache directory
    // - Version directory without binaries
    // - Valid complete installation
    // - Multiple browser versions
}
```

#### Integration Tests

**Real Driver Download**:
```go
func TestPlaywrightDriverDownload_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test in short mode")
    }

    // Clean cache to force fresh download.
    cleanPlaywrightCache(t)

    // Download real Chromium (~140 MB).
    runOptions := playwright.RunOptions{
        SkipInstallBrowsers: false,
        Browsers:            []string{"chromium"},
    }
    err := playwright.Install(&runOptions)
    require.NoError(t, err)

    // Validate installation.
    chromiumPath := findChromiumInstallation(t)
    require.NotEmpty(t, chromiumPath, "Chromium should be installed")

    // Verify actual browser files exist (160+ files expected).
    fileCount := countFilesRecursive(t, chromiumPath)
    require.Greater(t, fileCount, 10, "Installation should contain many files")

    t.Logf("Successfully downloaded and validated Chromium driver (%d files)", fileCount)
}
```

**Why Integration Tests?**
- Unit tests with mocks don't catch playwright-go API changes
- Real downloads validate network handling and cache permissions
- File count verification prevents false positives from empty directories
- Ensures driver actually works for browser automation

**Test Skipping**:
- Skip by default (requires ~140 MB download)
- Run explicitly: `go test -v -run TestPlaywrightDriverDownload_Integration`
- CI can run on schedule rather than every commit

### Documentation Guidelines

**User Documentation (`website/docs/cli/commands/auth/usage.mdx`)**:

**Emphasize Automatic Download**:
```markdown
## SAML Browser Driver Installation

### Recommended: Automatic Download

Configure Atmos to automatically download browser drivers on first use:

yaml
auth:
  providers:
    my-saml:
      download_browser_driver: true

Atmos will download the Chromium browser driver (~140 MB) to your system's cache directory on first SAML authentication. No PATH configuration needed.
```

**Warn About Manual Installation**:
```markdown
### Advanced: Manual Installation

**Warning**: Manual installation requires understanding playwright-go internals and cache directory conventions. Only recommended for pre-provisioned environments.

Playwright-go caches drivers in platform-specific locations:
- macOS: `~/Library/Caches/ms-playwright`
- Linux: `~/.cache/ms-playwright`
- Windows: `%USERPROFILE%\AppData\Local\ms-playwright`

For manual installation details, see [playwright-go documentation](https://github.com/playwright-community/playwright-go).
```

**Developer Documentation (CLAUDE.md)**:

Already updated with:
- Interface-driven design requirements
- Integration testing strategy
- Mock generation patterns
- Comment preservation guidelines

### Cross-Platform Considerations

**Cache Directory Detection**:
- Check all three platform paths (macOS, Linux, Windows)
- Use `filepath.Join()` for path construction (never hardcoded separators)
- Handle missing home directory gracefully

**File System Permissions**:
- Cache directories created by playwright-go with appropriate permissions
- No special Atmos handling needed
- Document permission errors if they occur

**Binary Compatibility**:
- Playwright-go handles platform-specific binary downloads
- Chromium binaries are platform and architecture specific
- No cross-compilation concerns for Atmos

### Error Handling

**Download Failures**:
```go
// saml2aws will return clear error if download fails.
err := provider.Login()
if err != nil {
    // Error includes: "please install the driver (v1.47.2) and browsers first".
    return fmt.Errorf("%w: SAML authentication failed: %w",
        errUtils.ErrAuthenticationFailed, err)
}
```

**Detection Failures**:
- If detection incorrectly reports drivers missing, automatic download activates
- Better to re-download than fail authentication
- Logs at Debug level show detection results

**Cache Permission Issues**:
- Playwright-go handles cache creation
- Permission errors surface through saml2aws
- User must resolve OS-level permission issues

## Benefits

1. **Improved User Experience**: Users configure once and Atmos handles driver management
2. **Reduced Support Burden**: Automatic download eliminates "drivers not found" issues
3. **Platform Agnostic**: Works consistently across macOS, Linux, Windows
4. **Future Proof**: Integration tests catch playwright-go API changes
5. **Clear Documentation**: Users understand automatic vs manual installation trade-offs

## Future Enhancements

1. **Driver Update Detection**: Detect when playwright-go version requires newer drivers
2. **Offline Mode**: Support pre-downloaded driver bundles for air-gapped environments
3. **Alternative Browsers**: Support Firefox/WebKit if saml2aws adds support
4. **Driver Cleanup**: Command to remove outdated cached drivers
5. **Progress Indication**: Show download progress for large driver downloads

## Related PRDs

- `test-preconditions.md`: Defines test skipping for integration tests
- `error-handling-strategy.md`: Error wrapping patterns used
- `auth-context-multi-identity.md`: Overall authentication architecture
- `container-auth-fixes.md`: Containerized environment authentication considerations

## References

- saml2aws library: https://github.com/Versent/saml2aws
- playwright-go: https://github.com/playwright-community/playwright-go
- Playwright browser management: https://playwright.dev/docs/browsers#managing-browser-binaries
- PR #1747: Original implementation and fixes
