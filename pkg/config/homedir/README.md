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

### Environment Variable Support

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

On Unix-like systems (Linux, macOS), the library checks `$HOME` first before trying
OS-specific detection methods like `dscl` (macOS) or `getent passwd` (Linux).

On Windows, it checks `$HOME`, then `$USERPROFILE`, then `$HOMEDRIVE/$HOMEPATH`.

### Testing with t.Setenv()

To use `t.Setenv("HOME", ...)` in tests, you must disable caching:

```go
func TestSomethingWithCustomHome(t *testing.T) {
    homedir.DisableCache = true
    defer func() { homedir.DisableCache = false }()

    tmpHome := t.TempDir()
    t.Setenv("HOME", tmpHome)

    // Now homedir.Dir() will return tmpHome
    dir, err := homedir.Dir()
    // dir == tmpHome
}
```

Why disable caching? The library caches the home directory on first call for performance.
Without `DisableCache = true`, the cached value won't reflect environment changes made by
`t.Setenv()`.

### Cache Management

The library provides two ways to handle caching:

1. **Disable caching (recommended for tests)**:
   ```go
   homedir.DisableCache = true
   ```

2. **Reset cache (alternative approach)**:
   ```go
   homedir.Reset()  // Forces next Dir() call to re-detect
   ```

See `pkg/config/homedir/homedir_test.go` for working examples.
