#!/usr/bin/env bash
#
# list-ami-tags.sh — print all tags on the built AMI as a table.
#
# Backs `atmos ami list-tags <component> -s <stack>`.

set -euo pipefail
# shellcheck source=scripts/atmos/_lib.sh
source "$(dirname "$0")/_lib.sh"

require_cmd aws
require_env ATMOS_AMI_REGION

ami_id="$(resolve_ami_id)"

echo "==> Tags on ${ami_id} (${ATMOS_AMI_REGION}):"
aws ec2 describe-tags \
  --region "${ATMOS_AMI_REGION}" \
  --filters "Name=resource-id,Values=${ami_id}" \
  --query 'Tags[].{Key:Key,Value:Value}' \
  --output table
