# Fix: Pin Windows acceptance runners while investigating runtime corruption

**Date:** 2026-10-15

## Summary

Use Windows Server 2022 for the existing Windows build, acceptance-shard, registry-cache, and demo jobs. Keep their Go toolchain, tests, security hardening, and concurrency limits unchanged.

## Context

Go 1.26.9 crashed inside `internal/ci/acceptance` with `fatal error: found pointer to free object` on two current stack heads:

- [PR #3322 Windows shard](https://github.com/cloudposse/atmos/actions/runs/38671220369/job/116218880500): Windows Server 2025, Intel Xeon Platinum 8573C.
- [PR #3100 Windows shard](https://github.com/cloudposse/atmos/actions/runs/38673183860/job/116225771350): Windows Server 2025, Intel Xeon 6973P-C.

Those exact CPU models and the failure signature match the affected hosts in [Go issue #81238](https://github.com/golang/go/issues/81238). That report demonstrates Windows exception frames exceeding the runtime's stack reserve on AMX hosts, including Go 1.26. The matching evidence supports a runner/runtime mitigation; it does not establish that the same exception path caused these two Atmos crashes.

The existing subprocess limit is already one. Lowering it or retrying unchanged jobs would not address the recurring failure.

## Changes

Pin the Windows entries of the existing build, acceptance-shard, registry-cache, and demo matrices to `windows-2022`, a supported GitHub runner image. Keep every shard, timeout, test command, and security policy. Document the upstream issue beside the runner selection.

## Validation

`actionlint .github/workflows/test.yml`, `git diff --check`, and `GOTOOLCHAIN=go1.26.9 go test -race ./internal/ci/acceptance -run 'TestVerify(Linux|Windows)Target' -count=1` passed.

[Windows Server 2022 diagnostics](https://github.com/cloudposse/atmos/actions/runs/38732362292) passed five repetitions each in baseline, race, and checkptr modes with Go 1.26.9 against unchanged stack source `3755e7a0dd50fc1ae9bfb25647e178a8533cf450`. The [Server 2025 race/checkptr comparison](https://github.com/cloudposse/atmos/actions/runs/38731671788) also passed five repetitions each; the intermittent corruption was not reproduced in that diagnostic run. Full stack CI remains the final validation of the mitigation.

## Follow-ups

Re-evaluate `windows-latest` after the upstream runtime fix is available and validated on affected Server 2025 AMX hosts. Verify full CI and coverage on the final published stack heads.
