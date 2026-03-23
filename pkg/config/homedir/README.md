# go-homedir

This is a Go library for detecting the user's home directory without
the use of cgo, so the library can be used in cross-compilation environments.

Usage is incredibly simple, just call `homedir.Dir()` to get the home directory
for a user, and `homedir.Expand()` to expand the `~` in a path to the home
directory.

**Why not just use `os/user`?** The built-in `os/user` package requires
cgo on Darwin systems. This means that any Go code that uses that package
cannot cross compile. But 99% of the time the use for `os/user` is just to
retrieve the home directory, which we can do for the current user without
cgo. This library does that, enabling cross-compilation.

## Atmos Fork Enhancements

This is Atmos's vendored fork of the deprecated `github.com/mitchellh/go-homedir` package.
It includes important enhancements for test compatibility:

## Home Directory Precedence

### Unix / Linux / macOS

| Priority | Source | Notes |
|---|---|---|
| 1 | `$HOME` env var | Checked first; whitespace-only values are skipped. Wrapping single/double quotes are stripped (e.g., `HOME="/home/user"` works). |
| 2 | `os/user.Current().HomeDir` | Pure-Go `/etc/passwd` reader in CGO=0 builds; may not find NSS/LDAP-only users. |
| 3 | `dscl` (macOS only) | Reads `NFSHomeDirectory` from the macOS Directory Service. |
| 4 | Shell tilde expansion | Fetches the username via `id -un` → falls back to `$USER` → then `whoami`. Expands `~username` via `sh -c 'printf "%s\n" ~username'`. All three of `id`, `$USER`, and `whoami` may be absent in distroless/scratch containers; in that case `ErrIDUnavailable` is returned. `ErrShellUnavailable` is returned when `sh` itself is absent. |

### Windows

| Priority | Source | Notes |
|---|---|---|
| 1 | `%HOME%` | Wrapping quotes are stripped; forward slashes are converted to backslashes. **POSIX-style paths** (e.g., `/cygwin/home/user`) become **drive-relative** (`\cygwin\home\user`) unless `HOMEDRIVE` or `SystemDrive` is set. **UNC paths** (e.g., `\\server\share\user`) are returned unchanged. **Drive-letter-relative paths** (e.g., `C:Users\me`) are normalized to drive-absolute (`C:\Users\me`). |
| 2 | `%USERPROFILE%` | Same quoting, slash conversion, UNC, and drive-relative handling as `HOME`. |
| 3 | `%HOMEDRIVE%` + `%HOMEPATH%` | `HOMEPATH` is required to start with `\`. If it is missing, `\` is prepended automatically (e.g., `Users\foo` → `C:\Users\foo`). |

> **Windows note:** If `HOME` or `USERPROFILE` contain a POSIX-style path without a drive letter
> (e.g., `/home/user` from Cygwin, Git Bash, or WSL1), the result will be **drive-relative**
> (`\home\user`) when neither `HOMEDRIVE` nor `SystemDrive` is set, and **drive-absolute**
> (`C:\home\user`) when one of those env vars is set. Standard Windows environments always
> have at least `SystemDrive` set, so this is only an issue in containers or unusual setups.
> To guarantee an absolute path regardless, set `HOME` to a Windows drive-absolute value
> such as `C:\Users\username`.

## Environment Variable Support

Unlike the original package, this fork **prioritizes environment variables** when detecting
the home directory. This makes it compatible with `t.Setenv()` in Go tests:

```go
func dirUnix() (string, error) {
    // Try to get the home directory from the environment variable first
    if home := getHomeFromEnv(); home != "" {
        return home, nil
    }
    // ... fallback to OS-specific methods ...
}
```

## Cache Management

The library caches the home directory on the first call to `Dir()` for performance.
Without disabling caching, the cached value won't reflect environment changes.

### Disabling the cache (recommended for tests)

Use the thread-safe `SetDisableCache()` function:

```go
func TestSomethingWithCustomHome(t *testing.T) {
    homedir.SetDisableCache(true)
    defer homedir.SetDisableCache(false)

    t.Setenv("HOME", t.TempDir())

    dir, err := homedir.Dir()
    // dir == the value set by t.Setenv
}
```

Or assign the unexported `DisableCache` directly **before** any concurrent `Dir` calls
(safe at package init / `TestMain` level, but not during concurrent access):

```go
homedir.DisableCache = true   // safe only if no goroutines are calling Dir yet
```

> **Thread safety:** `SetDisableCache` acquires the internal cache lock and is safe from
> any goroutine. Direct assignment to `DisableCache` is **not** protected by a lock and
> must only be done before any parallel `Dir` calls begin.

### Reset cache

```go
homedir.Reset()  // Forces the next Dir() call to re-detect the home directory
```

## Timeout Tuning

All external subprocess calls (`id`, `dscl`, `sh`) use a shared timeout
(default: **5 seconds**). Override it via the `ATMOS_HOMEDIR_CMD_TIMEOUT` environment
variable — any value accepted by [`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration)
is valid. Zero or invalid values are silently ignored and the default is retained.

> **Important:** `ATMOS_HOMEDIR_CMD_TIMEOUT` is read **once at program init** and cannot
> be changed at runtime by modifying the environment variable after the process starts.
> Use `SetExternalCmdTimeout(d time.Duration)` for runtime adjustment — it is thread-safe
> and may be called from any goroutine, including concurrent callers of `Dir` or `Expand`.

```sh
# Tighter timeout for fast NSS backends (recommended default for most installs)
export ATMOS_HOMEDIR_CMD_TIMEOUT=2s

# Generous timeout for slow LDAP/NIS backends with high round-trip latency
export ATMOS_HOMEDIR_CMD_TIMEOUT=15s

# Sub-second timeout in containers where id should be instant
# (distroless Alpine, scratch images, Kubernetes init containers)
export ATMOS_HOMEDIR_CMD_TIMEOUT=250ms
```

See `pkg/config/homedir/homedir_test.go` for working examples.

## Odd-Case Examples

### Quoted HOME on Unix

Some shell init scripts write `HOME` with literal surrounding quotes. The library
strips them automatically:

```sh
export HOME='"/home/user"'   # literal quotes
# homedir.Dir() returns /home/user (quotes stripped)
```

### macOS dscl Fallback

When `$HOME` is unset and `os/user.Current()` fails (e.g., CGO=0 builds on macOS),
the library queries macOS Directory Services:

```sh
unset HOME
# dirUnix() → os/user.Current() → getDarwinHomeDir() → dscl -q . -read /Users/<user> NFSHomeDirectory
# Returns the NFSHomeDirectory value from the local user database.
```

### Windows POSIX-style HOME (Drive-Relative)

When `HOME` contains a POSIX-style path without a drive letter (e.g., from Cygwin,
Git Bash, or WSL1), it becomes **drive-relative** after Windows path conversion:

```sh
# Input:  HOME=/home/user
# Output: \home\user  (drive-relative, not absolute!)
#
# For a guaranteed absolute path on Windows, use a drive-absolute value:
set HOME=C:\home\user
# Output: C:\home\user  (absolute)
```

This behavior is by design to match the output of `filepath.FromSlash + filepath.Clean`
on Windows. See the Windows priority table above for details.
