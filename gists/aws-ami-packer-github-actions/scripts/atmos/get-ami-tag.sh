#!/usr/bin/env bash
#
# get-ami-tag.sh — print the value of a single tag on the built AMI.
#
# Backs `atmos ami get-tag <component> -s <stack> --key <k>`.
# Useful in CI to gate on ScanStatus, e.g.:
#   [[ "$(atmos ami get-tag al2023 -s al2023 --key ScanStatus)" == "approved" ]]

set -euo pipefail
# shellcheck source=scripts/atmos/_lib.sh
source "$(dirname "$0")/_lib.sh"

require_cmd aws
require_env ATMOS_AMI_REGION
require_env ATMOS_AMI_TAG_KEY

ami_id="$(resolve_ami_id)"

aws ec2 describe-tags \
  --region "${ATMOS_AMI_REGION}" \
  --filters "Name=resource-id,Values=${ami_id}" "Name=key,Values=${ATMOS_AMI_TAG_KEY}" \
  --query 'Tags[0].Value' \
  --output text
