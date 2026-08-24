#!/usr/bin/env bash
#
# list-instances-by-ami.sh — list running/pending instances launched from the AMI.
#
# Backs `atmos ami list-instances <component> -s <stack>`.

set -euo pipefail
# shellcheck source=scripts/atmos/_lib.sh
source "$(dirname "$0")/_lib.sh"

require_cmd aws
require_env ATMOS_AMI_REGION

ami_id="$(resolve_ami_id)"

echo "==> Instances launched from ${ami_id} (${ATMOS_AMI_REGION}):"
aws ec2 describe-instances \
  --region "${ATMOS_AMI_REGION}" \
  --filters \
    "Name=tag:LaunchedFromAMI,Values=${ami_id}" \
    "Name=instance-state-name,Values=pending,running,stopping,stopped" \
  --query 'Reservations[].Instances[].{InstanceId:InstanceId,State:State.Name,Type:InstanceType,LaunchTime:LaunchTime}' \
  --output table
