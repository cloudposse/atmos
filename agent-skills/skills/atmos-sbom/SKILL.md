---
name: atmos-sbom
description: "Atmos SBOM provenance: CycloneDX and SPDX generation from vendor and Terraform evidence, coverage diagnostics, NTIA validation, and native CI workflow-artifact publication"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - ../atmos-ci/references/native-ci.md
  - ../atmos-vendoring/references/component-updater.md
---

# Atmos SBOM Provenance

## Purpose

Use this skill to generate, review, or publish an Atmos software bill of materials (SBOM).
Atmos produces a **provenance/build-input SBOM**: it records evidence used to build or run the
selected infrastructure scope. It is not a claim that every deployed workload has been inventoried.

The default scope is `terraform`. It includes Atmos-managed source receipts, OCI source
artifacts, Terraform provider locks, and Terraform modules when the configured command exposes a
stable module graph. Pass `--scope dependencies` instead to inventory Atmos's own toolchain
(`.tools/toolchain.lock.yaml`) and version-track (`versions.lock.yaml`) evidence rather than
Terraform; Atmos-managed source receipts are always included regardless of `--scope`. Helm,
Helmfile, discovered deployed images, image package contents, and OpenTofu module graphs are
outside this initial scope and must be reported as incomplete or unavailable rather than silently
treated as absent.

## Related Skills

| Need | Load |
|---|---|
| Immutable vendor receipts, reconciliation, Component Updater PRs | [atmos-vendoring](../atmos-vendoring/SKILL.md) |
| Just-in-time component source workdirs and source receipts | [atmos-components](../atmos-components/SKILL.md) |
| Native CI workflow structure and GitHub Actions runtime setup | [atmos-ci](../atmos-ci/SKILL.md) |
| Toolchain locks and configured Terraform/OpenTofu commands | [atmos-toolchain](../atmos-toolchain/SKILL.md) |
| Version-track locks | [atmos-version](../atmos-version/SKILL.md) |

## Generate an SBOM

Generate CycloneDX JSON to stdout, or write either supported format to a reviewed output file:

```shell
atmos sbom generate --format cyclonedx-json
atmos sbom generate --format spdx-json --output sbom.spdx.json
atmos sbom generate --include-files --output sbom.cyclonedx.json
```

`--include-files` expands the output with the vendor receipt's installation-file relationships;
omit it for the normal artifact-oriented output.

The result is derived from evidence, not guesses:

- `vendor.lock.yaml` records immutable Git commits, OCI manifest digests, or verified content hashes
  and the installed-file inventory for `vendor.yaml`, `component.yaml`, and mixin installs.
- Just-in-time component workdirs contribute local immutable-resolution receipts; they do not create
  a committed vendor lock.
- `.terraform.lock.hcl` is authoritative for the selected provider versions and checksums.
- Atmos invokes the configured Terraform command (or `terraform` when none is configured) as
  `modules -json`; this requires Terraform 1.10+ and an initialized component directory.

Never insert a checksum, purl, supplier, source URL, or module inventory by inference. Missing
evidence must remain `NOASSERTION` or appear as an explicit coverage diagnostic.

## Provenance and NTIA Modes

The default is `--mode provenance`, which renders available evidence with coverage diagnostics:

```shell
atmos sbom generate --scope terraform --mode provenance --format cyclonedx-json
```

Use `--mode ntia` only when a declared subject is available and every adapter required for the
selected Terraform scope reports complete coverage:

```shell
atmos sbom generate \
  --scope terraform \
  --mode ntia \
  --subject-name infra-live \
  --subject-version "$(git rev-parse --short HEAD)" \
  --subject-supplier "Cloud Posse, LLC" \
  --format spdx-json \
  --output infra-live.sbom.spdx.json
```

NTIA mode must fail—not downgrade silently—when the subject is incomplete, a provider lock lacks
the required SHA-256 evidence, a module is not immutably resolved, or the module graph command is
unavailable. A configured OpenTofu command is honored, but its module inventory is unavailable
unless it implements the same stable JSON interface.

## Native CI Publication

`--upload` publishes the already-generated document through the detected native CI provider. The
document remains the source of truth and is still written to stdout or `--output`.

In GitHub Actions, publication stores a workflow artifact via the Actions runtime API. Add the
runtime action before the Atmos command:

```yaml
permissions:
  contents: read

steps:
  - uses: actions/checkout@v6
  - uses: cloudposse/atmos/actions/github-runtime@v1
    with:
      mode: env
  - run: atmos sbom generate --format spdx-json --output sbom.spdx.json --upload
    env:
      GITHUB_TOKEN: ${{ github.token }}
```

Do not claim that this uploads dependencies to GitHub Dependency Graph. GitHub's SBOM APIs can
export or request GitHub-generated reports, but do not accept arbitrary submitted SBOMs. The
correct result is a retained, auditable workflow artifact.

## Safe Output Rules

- SBOM output must not contain absolute filesystem paths, credentials, tokens, or signed URLs.
- Source references are credential-free and use immutable resolved evidence where available.
- File-level data is opt-in through `--include-files`.
- Preserve coverage diagnostics in the output; a missing domain is a finding, not a zero-dependency result.
