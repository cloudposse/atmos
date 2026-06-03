#!/usr/bin/env bash
#
# terminate-instances-by-ami.sh — terminate ALL instances launched from the AMI.
#
# Backs `atmos ami terminate-instances <component> -s <stack>`. Used for cleanup
# after health checks.

set -euo pipefail
# shellcheck source=scripts/atmos/_lib.sh
source "$(dirname "$0")/_lib.sh"

require_cmd aws
require_env ATMOS_AMI_REGION

ami_id="$(resolve_ami_id)"

# Collect non-terminated instances tagged with this AMI.
mapfile -t instance_ids < <(aws ec2 describe-instances \
  --region "${ATMOS_AMI_REGION}" \
  --filters \
    "Name=tag:LaunchedFromAMI,Values=${ami_id}" \
    "Name=instance-state-name,Values=pending,running,stopping,stopped" \
  --query 'Reservations[].Instances[].InstanceId' \
  --output text | tr '\t' '\n')

if [[ ${#instance_ids[@]} -eq 0 || -z "${instance_ids[0]}" ]]; then
  echo "==> No instances found for ${ami_id}; nothing to terminate"
  exit 0
fi

echo "==> Terminating ${#instance_ids[@]} instance(s) launched from ${ami_id}: ${instance_ids[*]}"
aws ec2 terminate-instances \
  --region "${ATMOS_AMI_REGION}" \
  --instance-ids "${instance_ids[@]}" \
  --query 'TerminatingInstances[].{InstanceId:InstanceId,State:CurrentState.Name}' \
  --output table
