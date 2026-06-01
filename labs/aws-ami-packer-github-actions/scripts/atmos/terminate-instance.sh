#!/usr/bin/env bash
#
# terminate-instance.sh — terminate a single EC2 instance by ID.
#
# Backs `atmos ami terminate-instance <component> -s <stack> --instance-id <id>`.

set -euo pipefail
# shellcheck source=scripts/atmos/_lib.sh
source "$(dirname "$0")/_lib.sh"

require_cmd aws
require_env ATMOS_AMI_REGION
require_env ATMOS_AMI_INSTANCE_ID

echo "==> Terminating ${ATMOS_AMI_INSTANCE_ID} in ${ATMOS_AMI_REGION}"
aws ec2 terminate-instances \
  --region "${ATMOS_AMI_REGION}" \
  --instance-ids "${ATMOS_AMI_INSTANCE_ID}" \
  --query 'TerminatingInstances[].{InstanceId:InstanceId,State:CurrentState.Name}' \
  --output table
