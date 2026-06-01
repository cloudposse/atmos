#!/usr/bin/env bash
#
# _lib.sh — shared helpers for the `atmos ami` custom-command scripts.
#
# Source this from the other scripts in this directory:
#   source "$(dirname "$0")/_lib.sh"
#
# These helpers wrap `atmos packer output` and the AWS CLI. They read inputs
# from ATMOS_AMI_* environment variables set by the custom-command definitions
# in atmos.yaml.

set -euo pipefail

# require_cmd <name> — fail with a clear message if a required tool is missing.
require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: required command '$1' not found in PATH" >&2
    exit 1
  fi
}

# require_env <VAR> — fail if an expected environment variable is empty/unset.
require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "ERROR: required environment variable '${name}' is not set" >&2
    exit 1
  fi
}

# resolve_ami_id — print the AMI ID of the most recent Packer build.
#
# Reads the build manifest via `atmos packer output` and selects the build whose
# packer_run_uuid matches the manifest's top-level last_run_uuid (the build that
# ran most recently), then extracts the AMI ID from its "region:ami-id"
# artifact_id. Robust to multiple historical builds in the manifest.
resolve_ami_id() {
  require_cmd atmos
  require_env ATMOS_AMI_COMPONENT
  require_env ATMOS_AMI_STACK

  local ami_id
  ami_id="$(atmos packer output "${ATMOS_AMI_COMPONENT}" -s "${ATMOS_AMI_STACK}" \
    -q '(.last_run_uuid) as $u | .builds[] | select(.packer_run_uuid == $u) | .artifact_id | split(":")[1]')"

  if [[ -z "${ami_id}" || "${ami_id}" == "null" ]]; then
    echo "ERROR: could not resolve an AMI ID from the Packer manifest for" \
      "component '${ATMOS_AMI_COMPONENT}' stack '${ATMOS_AMI_STACK}'." \
      "Has 'atmos packer build' run successfully?" >&2
    exit 1
  fi

  printf '%s' "${ami_id}"
}
