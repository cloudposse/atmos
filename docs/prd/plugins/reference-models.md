# Plugin reference models

**Status:** Design rationale for the [plugin platform](overview.md). Project observations are sourced below; Atmos decisions are proposed requirements. [All plugin PRDs](README.md).

## Helm: separate extension type from runtime

Helm 4 distinguishes versioned CLI, getter, and postrenderer plugin types from subprocess and Wasm runtimes. Its manifest separates plugin release version from API version. Atmos should likewise separate package identity, extension contracts, and runtime, while allowing several extension types per package. [Helm plugin model](https://helm.sh/docs/plugins/overview/)

Helm exposes version, type, API version, source, and provenance in listings. Atmos `plugin list` and `info` should show effective version, extensions, runtime, source, digest, verification status, and scope. Integrity verification and verified publisher identity must remain distinguishable. [Helm user guide](https://helm.sh/docs/plugins/user/)

Helm supports install/update/delete scripts for subprocess plugins. Atmos's project hook extensions are a different concept: package installation verifies and places artifacts without executing publisher setup scripts. Required tools use Atmos toolchain. Wasm remains a possible future runtime, outside this release. [Helm runtime configuration](https://helm.sh/docs/plugins/overview/)

Helm documents a transition period for its old manifest. Atmos should publish its own support/deprecation matrix and retain old fixture plugins in Atmos compatibility tests. [Helm migration guide](https://helm.sh/docs/plugins/migrate/)

## TFLint: make extensions easy to author and test

TFLint runs rulesets as separate gRPC processes with bidirectional communication: Atmos requests checks; plugins request configuration/evaluation services and report findings. Atmos should expose scoped API services for resolved stacks, credentials, processes, and diagnostics instead of making plugins reconstruct its configuration pipeline. [TFLint architecture](https://github.com/terraform-linters/tflint/blob/master/docs/developer-guide/architecture.md)

TFLint supplies a template, SDK, and release conventions. Atmos should generate the complete repository: handlers, mocks, fixtures, platform builds, and publication workflow. [TFLint authoring](https://github.com/terraform-linters/tflint/blob/master/docs/developer-guide/plugins.md)

SDK testing helpers create a runner from in-memory files and compare expected issues. Atmos should offer similarly small fixtures plus subprocess/protocol tests to catch failures hidden by direct unit tests. [TFLint testing helpers](https://github.com/terraform-linters/tflint-plugin-sdk/blob/master/helper/testing.go)

TFLint separates installation through `--init` from execution and enablement. Its documented installer uses GitHub release sources and exact versions. Atmos uses OCI repositories, optional discovery catalogs, constraints, and immutable locked selections, with automatic restoration of declared project packages when needed and explicit commands for lifecycle management. [TFLint configuration](https://github.com/terraform-linters/tflint/blob/master/docs/user-guide/plugins.md)

TFLint's experimental OPA ruleset permits Rego policies. This supports offering both data rule packs and executable validators; Atmos need not depend on that experimental ruleset to evaluate its own OPA policies. [TFLint OPA ruleset](https://github.com/terraform-linters/tflint-ruleset-opa)

## A consumer experience to exclude: custom Atmos builds

Golangci-lint's module-plugin flow declares `.custom-gcl.yml`, runs `golangci-lint custom`, then executes `custom-gcl`. It requires Go and Git. This is explicitly outside the desired Atmos experience. [Module plugins](https://golangci-lint.run/docs/plugins/module-plugins/)

Its Go shared-library path requires matching build environments and overlapping dependency versions. Atmos must not expose Go's shared-library ABI as its external contract. [Go plugins](https://golangci-lint.run/docs/plugins/go-plugins/)

Consumers keep the standard Atmos binary and install precompiled artifacts. Authors compile only their plugin against the public SDK/protocol, without matching Atmos's internal dependencies. [Distribution acceptance tests](distribution.md#acceptance-criteria) enforce this.
