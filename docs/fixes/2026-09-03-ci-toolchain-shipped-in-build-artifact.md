# Fix: CI installs the toolchain once per OS and ships it in a build artifact

**Date:** 2026-09-03

## Summary

Every one of the 30 acceptance shards in `test.yml` (`test` job, 10 shards × linux/windows/macos), plus the
three `terraform-registry-cache` legs and the `mock` jobs, ran `atmos toolchain install --default ...` for
Terraform, OpenTofu, Packer, Helm and Helmfile: ~400 MB of release downloads and ~2 minutes per job, with
a per-job dependency on releases.hashicorp.com, GitHub releases, get.helm.sh and Sigstore. The
"Cache Atmos toolchain" step meant to short-circuit that never hit, because the ~5 GB of Go caches written
per run churn the repository's 10 GB Actions cache before any toolchain entry survives. This change installs
the five tools once per OS in the `build` job, packages the installed tree as a `toolchain-<target>`
artifact (1-day retention, no cache quota), and has the consuming jobs unpack it into their own Atmos
toolchain cache before `atmos toolchain install`, which then skips every tool without a network request.
The network install stays as the safety net for anything the artifact lacks.

## Context

Measured from run logs: every shard logged `Cache not found for input keys: atmos-toolchain-...`, and the
Actions cache listing showed only PR-scoped Go cache entries minutes old. The plan
(Findings §4, Phase 2b) preferred shipping the toolchain in the build artifact over a hashed cache key,
because the cache quota problem is structural (Go caches) and would need the Go footprint to shrink first.

What the code says about the mechanics (read, then verified locally, see Validation):

- `atmos toolchain install` puts tools under `<cache root>/toolchain/bin/<owner>/<repo>/<version>/`
  (`pkg/toolchain/setup.go` `GetInstallPath`, `pkg/toolchain/installer/installer.go` `New`), where the
  cache root is the Atmos XDG cache dir (`~/.cache/atmos` on Linux/macOS, `%LOCALAPPDATA%\cache\atmos`
  on Windows). `atmos ci cache paths --format=env` prints that root (`ATMOS_CI_CACHE_PATHS=...`), so the
  workflow never hardcodes a per-OS path. The tree contains no symlinks and no absolute paths, so it is
  relocatable between machines with different home directories.
- The per-tool form the workflow used, `atmos toolchain install --default owner/repo@version`, never takes
  the "already installed" path: `RunInstall` (`pkg/toolchain/install.go`) always calls
  `InstallSingleTool`, which re-resolves the registry, re-downloads or re-verifies the release signature
  (checksums from releases.hashicorp.com, cosign against Sigstore for OpenTofu) and re-extracts. Only the
  from-file path (`installFromToolVersions` → `installOrSkipTool`) checks `FindBinaryPath` first and skips.
  With the network blocked, the per-tool form hung for the full 2-minute command timeout even with the
  download archive already cached; the from-file form skipped every tool in under a second.
- `actions/upload-artifact` zips lose executable bits, so the tree is shipped as a tar, not as loose files.
- `.tool-versions` handling is being consolidated separately (#3022); the pins still come from the
  workflow `env` (`OPEN_TOFU_VERSION`, `PACKER_VERSION`, `HELM_VERSION`, `HELMFILE_VERSION`), and Terraform
  from the repository's `.tool-versions`, exactly as before.

## Changes

- `.github/actions/ci-toolchain/action.yml` (new composite action, used by all four jobs so the tool list
  and the install command are identical everywhere):
  1. writes a job-local tool-versions file from the `tool-versions` input plus the repository's
     `hashicorp/terraform` pin;
  2. resolves the Atmos cache root with `atmos ci cache paths --format=env`;
  3. when `artifact` is set, downloads it (`download-artifact-retry`, `continue-on-error`) and untars
     `toolchain.tar` into the cache root (`cygpath` on Windows so GNU tar does not read `D:\...` as a
     remote `host:file`); a missing artifact is a `::warning::`, not a failure;
  4. `atmos toolchain install --tool-versions <file>` (skips what is on disk, installs the rest);
  5. `atmos toolchain env --tool-versions <file> --format=github`.
- `.github/workflows/test.yml`:
  - `build` job: after "Verify acceptance shard plan", puts `./build` on PATH so the install runs with the
    atmos just built (same on-disk layout the consumers' binary expects), installs the five tools via the
    action, tars `toolchain/bin` plus `toolchain.lock.yaml` and `aqua-registry-index.json` (the downloaded
    release archives next to them are left out), and uploads it as `toolchain-<target>` with
    `retention-days: 1`. Skipped on `macos-intel`, whose only consumer (the k3s macOS job) installs no
    toolchain. `get.helm.sh:443` added to the build job's harden-runner allowlist (Helm's release host).
  - `test` and `terraform-registry-cache` jobs: the six-line "Install Terraform, OpenTofu, Packer, Helm,
    and Helmfile" `run:` step is replaced by the action with `artifact: toolchain-<target>`. The
    "Cache Atmos toolchain" step is untouched (a sibling PR makes it restore-only).
  - `mock` job: same replacement for its Terraform + OpenTofu install.
  - Consumers no longer rewrite the repository's `.tool-versions` (the old `--default` calls did, on every
    job); the acceptance harness finds tools through PATH (`tests/testhelpers/toolchain.go`
    `ProvisionToolchain`) and gives every test case its own `XDG_CACHE_HOME`, so nothing reads that file.

Expected artifact size: the local tar of Terraform + OpenTofu + Helm + the cosign verifier that OpenTofu's
signature check bootstraps is 439 MB uncompressed; with Packer and Helmfile, roughly 600 MB per OS, which
`upload-artifact` compresses to an estimated 250–300 MB. Three of them per run, kept for one day. The
existing `build-artifacts-<target>` is unchanged (it is downloaded into `/usr/local/bin` by a dozen jobs
that need no toolchain, which is why the toolchain is a separate artifact).

## Validation

- `actionlint .github/workflows/test.yml`: clean. `yq` parses both files (actionlint does not lint
  composite actions; run on `action.yml` it reports the expected "not a workflow" errors only).
- Local mechanics on macOS with a build of this branch (`go build -o /tmp/atmos-ci .`), scratch caches under
  `/tmp`, and the network blocked for the offline steps with `HTTPS_PROXY=http://127.0.0.1:9`:

  ```text
  $ ATMOS_XDG_CACHE_HOME=/tmp/tc atmos ci cache paths --format=json --path toolchain
  { "key": "atmos-toolchain-darwin-arm64-v2", "paths": ["/tmp/tc/atmos/toolchain"], ... }

  $ ATMOS_XDG_CACHE_HOME=/tmp/tc atmos toolchain install --default hashicorp/terraform
  ✓ Installed hashicorp/terraform@1.15.8 to /tmp/tc/atmos/toolchain/bin/hashicorp/terraform/1.15.8/terraform (106mb)
  $ find /tmp/tc -type l; grep -rl /tmp/tc /tmp/tc          # no symlinks, no embedded absolute paths

  # per-tool form, archive cached, network blocked: hangs (killed at the 2 min command timeout)
  $ ATMOS_XDG_CACHE_HOME=/tmp/tc2 HTTPS_PROXY=http://127.0.0.1:9 atmos toolchain install --default hashicorp/terraform
  Command timed out after 2m 0s

  # from-file form against a tar restored into an EMPTY cache root, network blocked
  $ tar -C /tmp/tc/atmos -cf /tmp/tc-artifact/toolchain.tar toolchain/bin toolchain/toolchain.lock.yaml toolchain/aqua-registry-index.json
  $ tar -C /tmp/tc3/atmos -xf /tmp/tc-artifact/toolchain.tar
  $ ATMOS_XDG_CACHE_HOME=/tmp/tc3 HTTPS_PROXY=http://127.0.0.1:9 atmos toolchain install --tool-versions /tmp/ci.tool-versions
  ✓ Skipped opentofu/opentofu@1.12.5 (already installed)
  ✓ Skipped helm/helm@v3.19.2 (already installed)
  ✓ Skipped hashicorp/terraform@1.15.8 (already installed)
  ✓ Installed 0 tools, skipped 3
  $ ATMOS_XDG_CACHE_HOME=/tmp/tc3 HTTPS_PROXY=http://127.0.0.1:9 atmos toolchain env --tool-versions /tmp/ci.tool-versions --format=github
  /tmp/tc3/atmos/toolchain/bin/hashicorp/terraform/1.15.8
  /tmp/tc3/atmos/toolchain/bin/helm/helm/v3.19.2
  /tmp/tc3/atmos/toolchain/bin/opentofu/opentofu/1.12.5
  $ /tmp/tc3/atmos/toolchain/bin/hashicorp/terraform/1.15.8/terraform version | head -1   # Terraform v1.15.8
  $ /tmp/tc3/atmos/toolchain/bin/opentofu/opentofu/1.12.5/tofu version | head -1           # OpenTofu v1.12.5
  $ /tmp/tc3/atmos/toolchain/bin/helm/helm/v3.19.2/helm version --short                    # v3.19.2+g8766e71
  ```

- The exact `run:` snippets of the action and of the build job's "Package the CI toolchain" step, extracted
  with `yq` and executed with `RUNNER_TEMP`, `GITHUB_OUTPUT` and `GITHUB_PATH` pointed at scratch files and
  `ATMOS_XDG_CACHE_HOME` at an empty root: pins file written (Terraform pinned from `.tool-versions`, blank
  input lines dropped), root resolved, tar unpacked, install skipped all three tools offline, PATH appended
  to `$GITHUB_PATH`, tar re-packaged with the expected members, missing-tarball path exits 0 with a
  `::warning::`, and the repository's `.tool-versions` stayed clean.
- Not validated: Windows (no local Windows machine; the `cygpath` conversions and GNU tar under Git Bash
  are reasoned from the existing "Add GNU tar to PATH" steps), the actual artifact size and transfer time
  on the hosted runners, and a run of the workflow itself. The first CI run of this PR is the real test;
  a failed artifact download degrades to today's behaviour by design.
- No Go code or test fixtures were changed.

## Follow-ups

None.
