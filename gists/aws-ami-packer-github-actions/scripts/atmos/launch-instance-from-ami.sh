#!/usr/bin/env bash
#
# launch-instance-from-ami.sh — launch a short-lived test instance from the AMI.
#
# Backs `atmos ami launch-instance <component> -s <stack> [--type <type>]`.
# The instance is tagged so it can be found and cleaned up later by
# `atmos ami list-instances` / `terminate-instances`.

set -euo pipefail
# shellcheck source=scripts/atmos/_lib.sh
source "$(dirname "$0")/_lib.sh"

require_cmd aws
require_env ATMOS_AMI_REGION
require_env ATMOS_AMI_INSTANCE_TYPE

ami_id="$(resolve_ami_id)"
subnet_id="${ATMOS_AMI_SUBNET_ID:-}"
security_group_ids="${ATMOS_AMI_SECURITY_GROUP_IDS:-}"

# Tag the instance with the AMI it came from so we can list/terminate it later.
tag_spec="ResourceType=instance,Tags=[{Key=Name,Value=ami-test-${ami_id}},{Key=LaunchedFromAMI,Value=${ami_id}},{Key=ManagedBy,Value=atmos-packer}]"

run_args=(
  --region "${ATMOS_AMI_REGION}"
  --image-id "${ami_id}"
  --instance-type "${ATMOS_AMI_INSTANCE_TYPE}"
  --count 1
  --tag-specifications "${tag_spec}"
)

# Only pass --subnet-id if the stack provided one; otherwise AWS uses the default VPC.
if [[ -n "${subnet_id}" ]]; then
  run_args+=(--subnet-id "${subnet_id}")
fi

# Only pass --security-group-ids if the stack provided them; otherwise AWS uses
# the default security group of the VPC. Set this when building into a custom VPC
# whose default SG is too restrictive for the instance to reach SSM/perform checks.
if [[ -n "${security_group_ids}" ]]; then
  # Space- or comma-separated list → array of IDs.
  IFS=', ' read -r -a sg_ids <<< "${security_group_ids}"
  run_args+=(--security-group-ids "${sg_ids[@]}")
fi

echo "==> Launching ${ATMOS_AMI_INSTANCE_TYPE} test instance from ${ami_id} in ${ATMOS_AMI_REGION}" >&2
instance_id="$(aws ec2 run-instances "${run_args[@]}" \
  --query 'Instances[0].InstanceId' --output text)"

echo "==> Launched ${instance_id}" >&2
# Print the bare instance ID on stdout so callers (CI) can capture it.
echo "${instance_id}"
