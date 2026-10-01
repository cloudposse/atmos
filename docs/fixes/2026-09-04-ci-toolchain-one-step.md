# Fix: the CI toolchain is one step - `atmos toolchain install` from `.tool-versions`

**Date:** 2026-09-04 (supersedes the 2026-09-03 "ship the toolchain as a build artifact" design)

## Summary

Every acceptance shard, the `terraform-registry-cache` legs and the `mock` jobs need the external
tools the repository pins in `.tool-versions` (Terraform, OpenTofu, Packer, Helm, Helmfile, ...).
Two things had grown around that over time:

1. **Versions duplicated in the workflow.** `test.yml` carried `OPEN_TOFU_VERSION`,
    `PACKER_VERSION`, `HELM_VERSION`, `HELMFILE_VERSION` next to the pins in `.tool-versions`, and the
    two drifted (a stale `OPEN_TOFU_VERSION` of 1.12.2 behind the file's 1.12.5 went undetected;
    #3022 removed the env vars). The first version of this PR still fed those variables into its
    action, so after merging main it would have written empty pins.
2. **Hand-rolled caching.** Because the "Cache Atmos toolchain" step never hit (the ~5 GB of Go caches
    written per run churned the 10 GB Actions cache before a toolchain entry survived), the first
    version of this PR built its own mechanism: the build job tarred the installed tree with
    `cygpath`-aware shell, uploaded it as a `toolchain-<os>` artifact, and every consumer downloaded and
    unpacked it before installing. ~150 lines of shell doing what atmos already does.

## Fix

`.github/actions/ci-toolchain` is now exactly what a developer runs, plus the cache atmos already
knows how to describe:

1. `./actions/cache` (the Atmos Cache action, driven by `ci.cache` in `atmos.yaml`): `cache: save` on
    the one producer job per OS (the `build` job, with the atmos it just built), `restore-only` (the
    default) on every consumer, so ten shards restore one key and none of them races to write it
    (`restore-only` comes from #3038, merged into this branch).
2. `atmos toolchain install`: reads `.tool-versions`, installs what is not on disk, skips the rest.
    This is the from-file form, the only one that takes the "already installed" path; the per-tool
    `--default owner/repo@version` form always re-resolves and re-verifies.
3. `atmos toolchain env --format=github`: exports the tool directories to `PATH`.

Every job that needs the toolchain is one step: `uses: ./.github/actions/ci-toolchain` with the
token (and `cache: save` on the build job). Versions are pinned in `.tool-versions` only. No tar, no
artifact, no `cygpath`, no per-job tool list.

Why atmos's cache is enough now: the repository's Actions cache limit was raised from 10 GB to 50 GB
(org setting, 2026-09-04) after measuring that main's entries never survived a run at 10 GB, and
`restore-only` on consumers removes the ten-way save race. A toolchain entry per OS is ~400-600 MB.

## Validation

- `actionlint .github/workflows/test.yml`, pre-commit hooks clean.
- The PR's own `Tests` run: the build job saves `atmos-toolchain-<os>-<arch>-v2`; each shard's
  `Set up the CI toolchain` step restores it and `atmos toolchain install` logs every tool as already
  installed.

## Follow-ups

- `HELM_DIFF_VERSION` (a Helm plugin, not a toolchain tool) is still a workflow env var; the
  `helm plugin install` step stays as is.
- Jobs that install a single tool inline (`atmos toolchain install opentofu/opentofu` in the floci,
  kubernetes-e2e and container-step jobs) could use the same action; left as they are here.
