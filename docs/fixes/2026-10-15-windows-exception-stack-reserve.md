# Fix: Reserve enough Windows exception-stack space in CI

**Date:** 2026-10-15

## Summary

Apply a temporary, version-checked Go runtime overlay to Windows CI builds and
tests. The overlay increases the Windows exception reserve from 4 KiB to 16 KiB
without modifying the installed toolchain. Release workflows are unaffected.

## Context

Go 1.26.9 crashed with `found pointer to free object` in both
`internal/ci/acceptance` and `pkg/component/helm` on Windows 2022 Intel Xeon
Platinum 8573C runners. Pinning the Windows image does not constrain the CPU or
its enabled extended state. [Go issue 81238](https://github.com/golang/go/issues/81238)
describes hardware-exception frames exceeding the runtime's fixed reserve.
The proposed dynamic-reserve fix in [CL 828724](https://go.dev/cl/828724) remains
under review; this overlay is a bounded CI workaround, not that upstream patch.

An isolated Go 1.26.9 stress probe recovered ordinary nil-pointer writes while
other goroutines allocated. On one Intel Xeon 6973P-C host with an 11,567-byte
exception context, the stock binary crashed in all three runs and the overlay
binary passed all three. Three AMD controls with 1,663-byte contexts passed both.
See [the diagnostic run](https://github.com/cloudposse/atmos/actions/runs/38751148055).

## Changes

- Require Windows/amd64, Go 1.26.9, and the verified SHA256 of its runtime source.
- Measure the host's exception context and reject sizes that exceed the reserve
  after allowing 2 KiB for dispatch frames.
- Write the one-expression replacement and Go overlay into a private temporary
  directory. Preserve existing Go flags and reject conflicting overlays.
- Apply the same reserve to precompiled CI artifacts and source-package tests,
  because build and test jobs can run on different CPU models.
- Retain test selection, concurrency, runner hardening, and failure diagnostics.

The larger reserve raises minimum goroutine stack allocations from 8 KiB to
32 KiB on Windows. This costs CI memory but also preserves sufficient exception
headroom after Go 1.26.9's quarter-full stack-shrinking rule.

## Validation

Eight local safeguard tests, actionlint, and `git diff --check` passed. The
diagnostic overlay cross-compiled a Windows executable successfully.

The first diagnostic's ten acceptance repetitions exceeded its 15-minute
timeout. That run is not a passing acceptance result. A corrected comparison
with three repetitions of stock acceptance, modified acceptance, race, and
checkptr tests plus focused runtime regressions passed on all eight Windows
2022/2025 hosts in [the corrected diagnostic](https://github.com/cloudposse/atmos/actions/runs/38754137258).
Those hosts had exception contexts of 1,663–3,263 bytes; none reproduced the
large-context condition. The initial comparison above supplies the affected-host
evidence, while this run checks acceptance and runtime regressions.

[The exact composite action](https://github.com/cloudposse/atmos/actions/runs/38755429683)
also passed its eight safeguard tests and built and ran the probe through its
exported Go flags on Windows. A local cross-build additionally verified an
overlay path containing spaces.

## Follow-ups

Remove the overlay when the supported Go release includes a Windows exception
reserve fix. The version and source-integrity checks intentionally require that
reassessment on a toolchain upgrade. Track validation in the existing stack work;
no new issue is needed for this unmerged change.
