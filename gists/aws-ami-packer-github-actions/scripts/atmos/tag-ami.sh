#!/usr/bin/env bash
#
# tag-ami.sh — add or update a tag on the built AMI.
#
# Backs `atmos ami tag <component> -s <stack> --key <k> --value <v>`.
# The pipeline uses this to flip ScanStatus from `pending` to `approved` after
# the manual approval gate.

set -euo pipefail
# shellcheck source=scripts/atmos/_lib.sh
source "$(dirname "$0")/_lib.sh"

require_cmd aws
require_env ATMOS_AMI_REGION
require_env ATMOS_AMI_TAG_KEY
require_env ATMOS_AMI_TAG_VALUE

ami_id="$(resolve_ami_id)"

echo "==> Tagging ${ami_id} with ${ATMOS_AMI_TAG_KEY}=${ATMOS_AMI_TAG_VALUE} in ${ATMOS_AMI_REGION}"
aws ec2 create-tags \
  --region "${ATMOS_AMI_REGION}" \
  --resources "${ami_id}" \
  --tags "Key=${ATMOS_AMI_TAG_KEY},Value=${ATMOS_AMI_TAG_VALUE}"

echo "==> Done"
