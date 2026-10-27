# Fix: Committed casts are validated in CI, and stale casts are re-recorded

**Date:** 2026-10-19

## Summary

Every committed website cast has a read-only validator under `demo/casts/atmos.d`, but no pull-request
workflow ran them. Only the manually dispatched screengrab workflow validated anything. Three cast
defects had accumulated on `main` unnoticed, and eight validators were not reachable from the
`casts validate all` entry point. A new `Validation (casts)` job in `test.yml` now runs
`atmos --chdir=demo/casts casts validate all` on every change, without recording or regenerating
anything. Two guards keep the gate complete. The stale casts were re-recorded through their pipelines.

## Context

- `atmos-config-get--help`, `atmos-stack-config-get--help`, and `atmos-stack-get--help` were committed
  in #3278 with a `dumb` terminal type and no color. They were re-recorded by hand outside the
  screengrab pipeline, so `casts validate screengrabs cli` failed on `main`. The `👽 test darwin/arm64`
  version banner in those casts is normal: 364 of the 371 screengrabs show it, because `casts setup`
  builds without release version flags.
- `examples/emulator-aws/lifecycle.cast` predated #2755, which changed the demo's stack from `dev` to
  `local` and updated the validator, but did not re-record the cast. Its validator also expected the
  image `floci/floci:1.5.23`, whereas the example now pins the image by digest, so `emulator ps` shows
  an image ID.
- `casts validate all` lists its children by hand. These validators were never reached:
  - `screengrabs cli about-help`
  - `demo fixtures ai-ask interactive-ask`
  - `demo fixtures tests`
  - `examples backend-provisioning lifecycle`
  - `examples packer-docker build`
  - `examples toolchain proxies`
- The existing `assert_no_dumb_term_recordings` check scanned only example casts, so it could not catch
  the screengrab defect.

## Changes

- Re-recorded `website/static/casts/screengrabs/atmos-config-get--help.cast`,
  `atmos-stack-config-get--help.cast`, and `atmos-stack-get--help.cast` with
  `atmos --chdir=demo/casts casts generate screengrabs cli --filter=…`. The substring filter also
  re-recorded `atmos-vendor-config-get--help.cast`; its plain text is unchanged.
- Re-recorded `website/static/casts/examples/emulator-aws/lifecycle.cast` with
  `casts generate examples emulator-aws lifecycle` against the local Floci emulator. The image needle
  was removed from the validator in `demo/casts/atmos.d/examples/emulator-aws/lifecycle.yaml`, with a
  comment explaining why.
- `demo/casts/atmos.d/examples/all.yaml`: the `examples` aggregate runs the backend-provisioning,
  packer-docker, and toolchain proxies validators.
- `demo/casts/atmos.d/all.yaml`: `casts validate all` runs the about-help, ai-ask, and tests validators.
  It ends with a coverage step that applies the dumb-terminal check to every `atmos.d` manifest and
  every committed cast. The same step fails when a leaf validator (one that calls no other validator)
  is unreachable from `casts validate all`.
- `demo/casts/cast_checks.py`:
  - New `assert_all_validators_reachable`.
  - `assert_no_dumb_term_recordings` now uses the `glob` module so `..` patterns work on every
    supported Python version, and its message no longer says "example".
- `.github/workflows/test.yml`: the new `validate-casts` job installs the run's Linux build artifact and
  runs `casts validate all`. Harden-runner uses the same blocked egress allowlist as
  `validate-affected`.

## Validation

- `atmos --chdir=demo/casts casts generate screengrabs cli --filter="stack get --help"` passed,
  including the full `casts validate screengrabs cli` step. The earlier `config get` run failed only
  because `stack get` had not yet been re-recorded.
- No committed cast has a `dumb` terminal header.
- Each of the 95 `casts validate` commands was run individually; only the emulator lifecycle validator
  failed before this change.
- `casts validate examples emulator-aws lifecycle` passed on the re-recorded cast, which contains no
  local paths.
- `atmos --chdir=demo/casts casts validate all` passed in 282 s in this workspace. It passed again in
  257 s under `env -i` with a minimal `PATH`, no TTY, and the system Python 3.9.6.
- Negative checks: a cast with a `dumb` header and a manifest with an orphaned validator both made the
  new guards fail with messages naming the offender.
- `actionlint .github/workflows/test.yml` reported no findings.
- Not run: the new job on GitHub Actions itself.

## Follow-ups

- `atmos emulator ps` prints the image ID instead of the configured image reference when an emulator is
  pinned by digest. This is pre-existing display behavior and needs a per-item decision.
- Each nested `atmos` invocation takes about 2.7 s to start in this workspace, which dominates the
  4–5 minutes `casts validate all` takes. This was not investigated.
