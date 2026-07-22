# SBOM Provenance

## Status

Initial implementation: Atmos sources, OCI source artifacts, Terraform provider
locks, and Terraform module graphs. This is a provenance/build-input SBOM, not
a claim that every deployed workload has been inventoried.

## Goal

Atmos must be able to emit CycloneDX JSON and SPDX JSON from one provenance
graph, with the NTIA minimum elements required by the selected scope: supplier,
component name, version, unique identifier or immutable evidence, dependency
relationship, author, and timestamp. The reference baseline is the [NTIA
minimum elements](https://www.ntia.gov/report/2021/minimum-elements-software-bill-materials-sbom),
the [NIST supply-chain guidance](https://www.nist.gov/itl/executive-order-14028-improving-nations-cybersecurity/software-supply-chain-security-guidance-20),
and the [CycloneDX model](https://cyclonedx.org/specification/overview/).

## Evidence Model

Each adapter emits a canonical source, a credential-free declared source, an
immutable identity, relationships, and coverage state. Git commits, OCI
manifest digests, and content-tree SHA-256 values are integrity evidence;
ETags and modification times are cache metadata only. Missing evidence is
reported as incomplete, never fabricated as a package URL or checksum.

Domain locks remain authoritative: `vendor.lock.yaml`,
`.tools/toolchain.lock.yaml`, and `versions.lock.yaml`. Workdir receipts remain
local metadata and are never presented as a committed project lock. See
[`docs/prd/vendor-lock.md`](./vendor-lock.md) for `vendor.lock.yaml`'s own
schema, coverage, and enforcement design — this document only covers how it
feeds the "Atmos sources" adapter below, not its full semantics.

## Initial Adapter Matrix

| Adapter | Evidence | Coverage rule |
| --- | --- | --- |
| Atmos sources | `vendor.lock.yaml` file inventory and immutable source identity | Complete when the lock parses. |
| OCI sources | Selected OCI descriptor digest | Complete when represented by a source receipt. |
| Terraform providers | `.terraform.lock.hcl` version and `zh:` SHA-256 archive hashes | Incomplete when an installed provider lacks a SHA-256 archive hash. |
| Terraform modules | `terraform modules -json` plus common-source resolution | Requires Terraform 1.10+ and immutable module evidence. |

`atmos sbom generate --scope terraform --mode provenance` emits available
evidence and coverage diagnostics. `--mode ntia` requires an explicit
subject name, version, and supplier, and fails if any required adapter is not
complete. It therefore makes no whole-environment compliance claim.

## CI Publication

`atmos sbom generate --upload` delegates publication to an optional native CI
provider capability. The generated document remains the authoritative output;
providers only determine how it is retained. GitHub Actions stores it as a
workflow artifact via the Actions runtime API. GitHub's Dependency Graph SBOM
endpoints can export or request GitHub-generated SPDX reports, but do not
accept arbitrary submitted SBOMs, so an artifact upload must not be described
as dependency-graph ingestion.

## Deferred Adapters

Helm and Helmfile lock/chart adapters, typed deployed-image discovery from
Atmos components/workflows/devcontainers/rendered Kubernetes, OCI SBOM
attestation or scanner integration for image package contents, and an OpenTofu
module adapter once it offers a stable structured module-graph interface. No
repository-wide `image:` scan or parsing of Terraform's internal
`.terraform/modules` directory is an acceptable substitute.
