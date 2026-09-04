# Non-fix: Windows Defender exclusions on CI Windows legs were a no-op

**Date:** 2026-09-04

## Summary

An earlier draft of `docs/fixes/2026-09-03-restore-only-toolchain-cache-on-shards.md` (then titled
"Windows Defender exclusions and restore-only toolchain cache on shards") added
`Add-MpPreference -ExclusionPath` calls for the workspace, hosted tool cache, Go caches, and temp
directories on every Windows leg of `test.yml` and `setup-go-cache-warmup.yml`, on the theory that Windows
Defender's real-time scanner inspecting every extracted file was the cause of `Set up Go` averaging 5.2 min
(max 10.5) restoring a 1.9 GB go-build+mod cache, and of `testing.TempDir` cleanup failures
(`unlinkat ... being used by another process`).

That theory was wrong. GitHub's own `actions/runner-images` build script
(`images/windows/scripts/build/Configure-WindowsDefender.ps1`, which builds the `windows-latest` image these
jobs actually run on) already disables real-time monitoring and excludes `C:\` and `D:\` - the entire
filesystem - at the image level, before any workflow step runs:

```powershell
@{DisableRealtimeMonitoring = $true}
@{ScanAvgCPULoadFactor = 5; ExclusionPath = @("D:\", "C:\")}
```

Confirmed live, not just from the build script, by adding a temporary diagnostic step to
`setup-go-cache-warmup.yml` and running it on an actual `windows-latest` runner (queried via
`Get-MpComputerStatus`/`Get-MpPreference` immediately before and after the exclusion step):

```
--- before our exclusion step ---
RealTimeProtectionEnabled : False
DisableRealtimeMonitoring : True
ExclusionPath              : {C:\, D:\}

--- after our exclusion step ---
DisableRealtimeMonitoring : True
ExclusionPath              : {C:\, C:\hostedtoolcache\windows, C:\Users\runneradmin\...\go-build, ...}
```

Every path our step added was already a subpath of the pre-existing `C:\` exclusion. Real-time protection
was off, and the entire drive was already excluded, before the step ran; the step changed nothing
observable about Defender's actual scanning behavior. Since Defender was never doing real-time scanning to
begin with, it cannot be the cause of the measured Windows slowdown - that has a different, still-unknown
root cause (Windows itself is roughly 2-3x slower than Linux/macOS runners in this repo's CI regardless of
this change; NTFS semantics, process-spawn overhead, and tar/zstd extraction cost are more likely
candidates than antivirus).

## Fix

Removed the `Exclude the workspace and Go caches from Windows Defender` step (and its explanatory comment)
from `build`, `terraform-registry-cache`, `test`, `mock` in `.github/workflows/test.yml`, and from
`.github/workflows/setup-go-cache-warmup.yml`. The restore-only-toolchain-cache change in the same original
PR is unaffected and unrelated - see `docs/fixes/2026-09-03-restore-only-toolchain-cache-on-shards.md`.

## Validation

- `atmos ci validate .github/workflows/test.yml .github/workflows/setup-go-cache-warmup.yml`: both valid.
- Diagnostic run: `gh run view 33877782841` (branch
  `osterman/ci-windows-defender-restore-only-cache`, `setup-go-cache-warmup.yml`, `Cache warmup (windows)`
  job) - live evidence quoted above.

## Follow-ups

- The actual cause of Windows CI being ~2-3x slower than Linux/macOS in this repo remains open. Worth a
  dedicated investigation rather than another blind antivirus-shaped guess.
