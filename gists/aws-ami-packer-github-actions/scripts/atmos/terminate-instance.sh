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

# Safety guard: only terminate instances this gist launched. `atmos ami
# launch-instance` tags every test instance with ManagedBy=atmos-packer; refuse
# to terminate anything missing that tag, so a mistyped instance ID cannot take
# down an unrelated instance. (A non-existent ID makes describe-instances fail
# under `set -e`, which also aborts before any termination.)
managed_by="$(aws ec2 describe-instances \
  --region "${ATMOS_AMI_REGION}" \
  --instance-ids "${ATMOS_AMI_INSTANCE_ID}" \
  --query "Reservations[].Instances[].Tags[?Key=='ManagedBy'].Value | [0]" \
  --output text)"

if [[ "${managed_by}" != "atmos-packer" ]]; then
  echo "ERROR: refusing to terminate ${ATMOS_AMI_INSTANCE_ID} — it is not tagged" \
    "ManagedBy=atmos-packer (got '${managed_by}'). This guard prevents terminating" \
    "instances not launched by this gist." >&2
  exit 1
fi

echo "==> Terminating ${ATMOS_AMI_INSTANCE_ID} in ${ATMOS_AMI_REGION}"
aws ec2 terminate-instances \
  --region "${ATMOS_AMI_REGION}" \
  --instance-ids "${ATMOS_AMI_INSTANCE_ID}" \
  --query 'TerminatingInstances[].{InstanceId:InstanceId,State:CurrentState.Name}' \
  --output table
