# Fix: `isDangerousPath` failed on Windows CI (`TestIsDangerousPath/root` and `/windows_drive_root,_no_separator`)

**Date:** 2026-08-11

## Summary

The `Acceptance Tests (windows)` GitHub Actions job failed two subtests of
`TestIsDangerousPath` in `pkg/toolchain`: `root` (input `"/"`) and
`windows_drive_root,_no_separator` (input `"C:"`), both expecting `true` (dangerous) but getting
`false`. `isDangerousPath` (added in an earlier coverage-fix pass) normalized the input with
`filepath.Clean` and then compared the result against hardcoded, Unix-flavored literals —
`cleaned == "/"` and a `len(cleaned) == 2 && cleaned[1] == ':'` check — but `filepath.Clean` is
OS-native: on Windows, `Clean("/")` returns `\` (not `/`), and `Clean("C:")` returns `C:.` (a
3-character string, not the 2-character bare volume name the length check expected). Both checks
silently failed to recognize their target inputs as dangerous on Windows, even though the
function's own test table explicitly names Windows-specific cases and was clearly meant to catch
this on every platform, not just the one it happened to be authored on.

## Context

Verified via reading Go's `internal/filepathlite` source (the actual implementation behind
`path/filepath.Clean`) rather than guessing: `Clean` calls `FromSlash` on its result, which
converts every `/` to the OS separator, and a bare drive-letter input with no path component gets
padded with a trailing `.` (`originalPath + "."`) to avoid ambiguity with a relative path — both
are windows-specific quirks with no equivalent on Unix, which is exactly why this bug only
surfaced on the Windows CI leg and passed cleanly on Linux/macOS.

## Changes

- `pkg/toolchain/clean.go`, `isDangerousPath` — rewritten to be platform-independent by
  construction rather than by accident:
  - The root check now matches both slash forms directly (`cleaned == "/" || cleaned == "\"`)
    instead of assuming the native separator ever appears in a hardcoded literal.
  - The Windows-drive-root check now operates on the **raw, un-`Clean`'d** input (trimmed of
    trailing slashes) rather than `Clean`'s OS-dependent output, since `Clean("C:")`'s exact
    shape differs by host OS but the raw input's `<letter>:` prefix does not.
  - This also restores the original intent (visible in the test's own naming) that Windows-style
    drive-root paths are rejected as dangerous regardless of which OS actually runs the check —
    defense-in-depth, not a Windows-only code path.

## Validation

- `go test ./pkg/toolchain/ -run TestIsDangerousPath -v` — all 7 subtests pass (previously 2
  failed on Windows CI). No live Windows runner was available for this session, so the root
  cause was diagnosed by tracing `filepath.Clean`'s actual Windows-specific behavior in the Go
  standard library source (see Context above), not by reproducing the Windows failure directly;
  the fix was then validated by running the platform-independent test suite on macOS, which
  exercises the same code paths without depending on the host OS.
- `go test ./pkg/toolchain/...` — full package tree passes.
- `go build ./...` — clean.
- `./custom-gcl run --new-from-rev=<merge-base>` — 0 issues.

## Follow-ups

None.
