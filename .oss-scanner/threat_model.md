# Threat model

## What this project does and where untrusted input enters
Atmos is a Go CLI that orchestrates Terraform/OpenTofu, Helmfile, Packer and workflows from YAML stack configuration.
Untrusted input includes: stack manifests and imports (local and remote, via go-getter URLs), vendor manifests and
vendored sources, Go templates (Sprig/Gomplate) and YAML functions (`!exec`, `!include`, `!terraform.output`, `!env`),
`atmos.yaml` and `atmos.d/` fragments, custom commands/workflows, downloaded toolchain archives, and API/JSON responses
from remote services. Configuration from a pull request in a third-party repository is a realistic attacker-controlled input.

## Components that matter most / least
Most: stack/config parsing and merging, template and YAML-function evaluation, remote import and vendoring (path
traversal, archive extraction, SSRF, symlink handling), toolchain download/verification, auth/credential handling and
keyring/config files, and anything that shells out. Least: docs website (`website/`), examples, demos, test fixtures.

## How to exercise it
`go build -o atmos .`; test suites under `pkg/` and `internal/`; fixtures in `tests/` and `examples/`.

## How you rate severity
Arbitrary command execution or file write outside the intended directory triggered by merely processing config that is
not expected to be trusted (e.g. a vendored or remotely imported manifest) is high/critical. Credential leakage into
logs, files or the process environment is high. Command execution by explicitly authored `!exec`, custom commands or
workflows in the user's own config is intended behavior and out of scope. DoS from malformed input is low/medium.

## Anything to leave alone
Do not report findings that require the attacker to already control the local `atmos.yaml` or the user's shell.
