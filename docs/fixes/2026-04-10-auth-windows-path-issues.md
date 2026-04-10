# Fix: Atmos Auth path issues on Windows

**Date:** 2026-04-10

**Issues:**

- SAML browser storage state fails to save on Windows due to mixed path
  separators (`C:\Users\user/.aws/saml2aws/storageState.json`)
- Storage strategy should prefer XDG paths directly instead of symlink
  workarounds that break on Windows

## Status

**Fixed.** Both issues share the same root cause: `saml2aws` hardcodes the
storage path with `fmt.Sprintf` and forward slashes. The fix replaces the
symlink strategy with a plain `os.MkdirAll` for the directory `saml2aws`
expects.

### Progress checklist

- [x] Root-cause analysis.
- [x] Chose approach: create `~/.aws/saml2aws/` as a real directory on
  all platforms. Drop the symlink strategy entirely.
- [x] Rewrite `setupBrowserStorageDir` to use `os.MkdirAll`.
- [x] Remove dead symlink code: `ensureStorageSymlink`,
  `isCorrectSymlink`, `stageExistingPath`, `restoreStagedPath`.
- [x] Update tests — removed all symlink-dependent tests, added
  cross-platform directory-creation tests (idempotent, preserves
  existing state, handles `.aws` as file, verifies not-a-symlink).
  `setupBrowserStorageDir` at 88.9% (only `homedir.Dir()` error
  path uncovered). `setupBrowserAutomation` at 100%.
- [ ] Full regression suite passes.

---

## Issue 1 — SAML browser storage state path broken on Windows

### Problem

After successful SAML authentication on Windows, `saml2aws` fails to save the
browser storage state file with the error:

```text
Error saving storage stateopen C:\\Users\\user/.aws/saml2aws/storageState.json:
The system cannot find the path specified.
```

Authentication itself succeeds (credentials are obtained), but the storage
state (which enables session reuse for subsequent logins) is lost. The next
`atmos auth login` requires a full interactive browser flow instead of
reusing the saved session.

### Root Cause

**Upstream bug in saml2aws** (`github.com/versent/saml2aws/v2`).

In `pkg/provider/browser/browser.go:118`, `saml2aws` constructs the storage
state path using `fmt.Sprintf` with hardcoded forward slashes:

```go
userHomeDir, err := os.UserHomeDir()
storageStatePath := fmt.Sprintf("%s/.aws/saml2aws/storageState.json", userHomeDir)
```

On Windows, `os.UserHomeDir()` returns a path with backslashes
(`C:\Users\user`), and the format string appends `/.aws/saml2aws/...`
with forward slashes. The resulting mixed-separator path
`C:\Users\user/.aws/saml2aws/storageState.json` cannot be resolved by
the Windows filesystem.

### Why the current symlink workaround doesn't help

Atmos creates `~/.aws/saml2aws` as a symlink to an XDG-compliant cache
directory via `setupBrowserStorageDir()`. This symlink is created using
`filepath.Join` (correct backslashes on Windows). However:

1. On Windows, `os.Symlink` requires Developer Mode or admin privileges.
   Without these, the symlink creation fails silently (our staging code
   restores the original directory).
2. Even if the symlink is created, `saml2aws's` mixed-separator path
   construction means the OS receives a path it cannot parse — the
   symlink is never reached.

### Fix options

**Option A — Create a plain directory instead of symlink on Windows.**

On Windows, skip the symlink strategy entirely and create
`%USERPROFILE%\.aws\saml2aws` as a regular directory. This ensures:

- The directory exists at the path `saml2aws` will look for.
- No symlink privilege requirements.
- `saml2aws's` forward-slash path still works because Windows `os.Stat`
  and file operations DO accept forward slashes in some contexts — the
  issue is specifically with the mixed-separator path through a parent
  directory that uses backslashes.

Actually, the real question is: does `os.OpenFile` on Windows accept
`C:\Users\user/.aws/saml2aws/storageState.json`? Testing shows that Go's
`os` package normalizes forward slashes on Windows for basic file
operations. The issue may be specifically with Playwright's `StorageState`
method which may use native Windows APIs that don't normalize.

**Option B — Ensure the directory exists at both path variants.**

Create `%USERPROFILE%\.aws\saml2aws\` using `filepath.Join` (backslashes)
AND verify the directory exists. Since Go's `os.MkdirAll` normalizes
paths, the directory should be accessible regardless of separator style.
The key insight is that the directory must exist as a real directory (not
a symlink) for `saml2aws's` mixed-separator path to resolve.

**Option C — Patch saml2aws upstream.**

Submit a PR to `github.com/versent/saml2aws` to use `filepath.Join`
instead of `fmt.Sprintf`. This is the correct long-term fix but doesn't
help users on current versions.

### Recommended approach

**Option B for immediate fix + Option C for upstream.** On Windows:

- Skip the symlink strategy in `setupBrowserStorageDir`.
- Create `%USERPROFILE%\.aws\saml2aws\` as a regular directory using
  `os.MkdirAll(filepath.Join(homeDir, ".aws", "saml2aws"), 0o700)`.
- The XDG-compliant cache location is still used on Linux/macOS where
  symlinks work reliably.
- File an upstream PR against `saml2aws` for the `filepath.Join` fix.

---

## Issue 2 — Storage strategy should use XDG paths directly, not symlinks

### Problem

The current `setupBrowserStorageDir` creates an XDG-compliant cache
directory (`~/.cache/atmos/aws-saml/<provider>`) and then symlinks
`~/.aws/saml2aws` to it. This is an indirect workaround for `saml2aws's`
hardcoded `~/.aws/saml2aws/storageState.json` path.

This symlink strategy is problematic:

1. **Windows:** `os.Symlink` requires Developer Mode or admin privileges.
   Most users don't have this. The staging/restore code handles the failure
   gracefully, but the result is that the storage directory doesn't exist
   at the path `saml2aws` expects → storage state is lost every session.

2. **Conceptual layering:** the symlink exists only to satisfy `saml2aws's`
   hardcoded path. Atmos should own its own storage locations (XDG) and
   work around `saml2aws's` limitations at the integration boundary — not by
   mutating the user's `~/.aws` directory with symlinks.

3. **Multi-provider conflicts:** the symlink points to ONE provider's
   directory. If a user has multiple SAML providers, the symlink gets
   replaced on each `atmos auth login`, losing the previous provider's
   storage state.

### Why we can't use XDG for saml2aws storage

XDG itself works fine on all platforms — `pkg/xdg` resolves to proper
paths like `%LOCALAPPDATA%\atmos\aws-saml\` on Windows. The problem is
that **saml2aws doesn't expose a configurable storage path**. The path
`~/.aws/saml2aws/storageState.json` is hardcoded in
`pkg/provider/browser/browser.go:118` with no way to override it.

The symlink strategy was an attempt to redirect `saml2aws's` hardcoded
path to an XDG-compliant location, but symlinks require elevated
privileges on Windows and the mixed-separator path construction in
`saml2aws` breaks even when the symlink exists.

XDG remains the right choice for **Atmos-owned data** (Playwright driver
cache detection, config, etc.) — it just can't be used for `saml2aws's`
storage state until upstream supports a configurable path.

### Recommended approach

**On all platforms:**

1. **Create `~/.aws/saml2aws/` as a real directory** (not a symlink).
   This is the path `saml2aws` hardcodes. `os.MkdirAll` works on all
   platforms without privilege requirements.

2. **Drop the symlink strategy entirely.** Remove `ensureStorageSymlink`,
   `isCorrectSymlink`, `stageExistingPath`, and `restoreStagedPath`.

3. **Accept that saml2aws owns `storageState.json`** — the filename and
   location are not ours to control. Multi-provider state sharing is an
   upstream limitation.

4. **Long-term: upstream PR** to `saml2aws` to:
  - Use `filepath.Join` instead of `fmt.Sprintf` (fixes Windows).
  - Accept a configurable storage path (enables XDG integration).

### Implementation

Rewrite `setupBrowserStorageDir` to:

```go
func (p *samlProvider) setupBrowserStorageDir() error {
  homeDir, err := homedir.Dir()
  if err != nil {
    return fmt.Errorf("failed to get user home directory: %w", err)
  }
}

// Create ~/.aws/saml2aws/ as a real directory.
// saml2aws hardcodes this path in pkg/provider/browser/browser.go:118.
  saml2awsDir := filepath.Join(homeDir, ".aws", "saml2aws")
  if err := os.MkdirAll(saml2awsDir, 0o700); err != nil {
      return fmt.Errorf("failed to create saml2aws storage directory: %w", err)
  }

  return nil
}
```

Remove `ensureStorageSymlink`, `isCorrectSymlink`, `stageExistingPath`,
and `restoreStagedPath` — they become dead code.

---

## End-to-end auth flow analysis: storageState.json vs AWS credentials

### Two separate storage systems

Atmos Auth uses two independent storage mechanisms. The Windows path
issue only affects one of them.

**1. AWS credentials (NOT affected by the Windows path issue)**

```text
atmos auth login
  → saml2aws browser login (Playwright opens browser, user authenticates)
  → saml2aws returns SAML assertion (base64-encoded XML)
  → Atmos calls AWS STS AssumeRoleWithSAML
    → returns AccessKeyID, SecretAccessKey, SessionToken, Expiration
  → PostAuthenticate() writes credentials to INI files:
      ~/.config/atmos/aws/{provider}/credentials
      (uses filepath.Join — correct separators on all platforms)
  → Credentials also stored in keyring (system keyring or file-based)
```

All credential paths use `filepath.Join` and Go's `os` package, which
normalizes separators on Windows. This pipeline works correctly on all
platforms.

**2. Playwright browser session state (AFFECTED by the Windows path issue)**

```text
saml2aws browser provider (after authentication completes):
  → context.StorageState(storageStatePath)
    → storageStatePath = fmt.Sprintf("%s/.aws/saml2aws/storageState.json", userHomeDir)
    → on Windows: "C:\Users\user/.aws/saml2aws/storageState.json" (mixed separators)
    → Windows cannot resolve this path → save fails
    → error logged: "Error saving storage state"
```

`storageState.json` contains Playwright browser session data (cookies,
localStorage). It is used exclusively for **browser session reuse** — so
the next `atmos auth login` can skip the interactive browser flow by
replaying saved cookies/session.

### Impact assessment

| What                                       | Affected? | Why                                                                              |
|--------------------------------------------|-----------|----------------------------------------------------------------------------------|
| `atmos auth login` (authentication itself) | No        | SAML assertion and STS call succeed regardless of storageState.json              |
| `atmos terraform plan/apply`               | No        | Reads credentials from INI files via `AWS_SHARED_CREDENTIALS_FILE` env var       |
| `atmos describe stacks`                    | No        | Uses credentials from INI files / keyring via AuthManager                        |
| `atmos auth whoami`                        | No        | Reads credential metadata from keyring / INI files                               |
| Browser session reuse on next login        | **Yes**   | Without saved storageState.json, user must re-authenticate in browser every time |

### How `atmos terraform plan` uses stored credentials

When a user runs `atmos terraform plan -s my-stack`:

1. `TerraformPreHook()` (`pkg/auth/hooks.go`) decodes the stack's auth
   config and creates an AuthManager.
2. The AuthManager loads previously stored credentials from:
  - INI files: `~/.config/atmos/aws/{provider}/credentials`
  - Keyring: system keyring or file-based fallback
3. `PrepareShellEnvironment()` returns environment variables:
  - `AWS_SHARED_CREDENTIALS_FILE` → path to the INI credentials file
  - `AWS_CONFIG_FILE` → path to the INI config file
  - `AWS_PROFILE` → identity name as profile section
  - `AWS_SDK_LOAD_CONFIG=1` / `AWS_EC2_METADATA_DISABLED=true`
4. These env vars are passed to terraform's subprocess. Terraform reads
   credentials via standard AWS SDK env vars pointing to Atmos-managed
   INI files — NOT via `AWS_ACCESS_KEY_ID` directly.

### Why our fix is correct

The `setupBrowserStorageDir` fix (creating `~/.aws/saml2aws/` as a real
directory via `os.MkdirAll`) ensures the directory exists at the path
`saml2aws` expects. Go's `os.MkdirAll` normalizes forward slashes on
Windows, so the directory creation succeeds. When `saml2aws` later writes
`storageState.json` using its mixed-separator path, Go's `os` package
handles the normalization internally — so the file write also succeeds.

The credential storage pipeline (`PostAuthenticate` → INI files →
`TerraformPreHook`) uses `filepath.Join` throughout and is unaffected
by this fix. It already works correctly on Windows.

---

## Related

- `docs/prd/saml-browser-driver-integration.md` — SAML browser driver
  integration PRD (covers Playwright driver download, XDG storage, and
  the original symlink strategy).
- `saml-driver-install` branch / PR #1747 — the branch where the symlink
  strategy was originally implemented.
- `pkg/auth/providers/aws/saml.go:setupBrowserStorageDir` — the function
  that now creates a plain directory (was previously a symlink).
- `pkg/auth/providers/aws/saml.go:Authenticate` — the SAML auth entry
  point that calls `setupBrowserAutomation` → `setupBrowserStorageDir`.
- `pkg/auth/cloud/aws/files.go:WriteCredentials` — writes AWS
  credentials to INI files (uses `filepath.Join` — correct on Windows).
- `pkg/auth/hooks.go:TerraformPreHook` — sets up env vars for terraform
  subprocess from stored credentials.
