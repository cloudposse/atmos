#!/usr/bin/env bash
#
# share-ami.sh — share the built AMI (and its snapshots) with other AWS accounts.
#
# Backs `atmos ami share <component> -s <stack> [--accounts <ids>] [--kms-grant]`.
#
# In a governed pipeline this runs only AFTER the approval gate has flipped
# ScanStatus to `approved`. It:
#   1. adds launch permission on the AMI for each target account,
#   2. adds create-volume permission on each backing snapshot, and
#   3. (optional) creates a KMS grant so target accounts can decrypt the
#      snapshots when the AMI is encrypted with a customer-managed key.

set -euo pipefail
# shellcheck source=scripts/atmos/_lib.sh
source "$(dirname "$0")/_lib.sh"

require_cmd aws
require_env ATMOS_AMI_REGION

ami_id="$(resolve_ami_id)"

# Accounts: prefer the --accounts flag; fall back to the stack's share_account_ids.
accounts_csv="${ATMOS_AMI_SHARE_ACCOUNTS_FLAG:-}"
if [[ -z "${accounts_csv}" ]]; then
  accounts_csv="${ATMOS_AMI_SHARE_ACCOUNTS_DEFAULT:-}"
fi

if [[ -z "${accounts_csv}" ]]; then
  echo "ERROR: no target accounts. Pass --accounts or set 'share_account_ids' in the stack." >&2
  exit 1
fi

# Split the comma-separated list into an array.
IFS=',' read -r -a accounts <<< "${accounts_csv}"

kms_grant="${ATMOS_AMI_KMS_GRANT:-false}"
kms_key_arn="${ATMOS_AMI_KMS_KEY_ARN:-}"

# Warn about the default-key sharing trap: if the AMI is encrypted with the
# account's default AWS-managed key (no CMK set), target accounts will be granted
# launch permission here but will NOT be able to launch it — AWS does not allow
# sharing the AWS-managed key. A customer-managed key (kms_key_arn) is required.
if [[ -z "${kms_key_arn}" ]]; then
  echo "WARNING: no kms_key_arn set — if this AMI is encrypted with the default" \
    "AWS-managed key, the target accounts will not be able to launch it. Use a" \
    "customer-managed key (CMK) for cross-account sharing. See stacks/al2023.yaml." >&2
fi

echo "==> Sharing ${ami_id} (${ATMOS_AMI_REGION}) with: ${accounts[*]}"

# 1) Launch permission on the AMI.
launch_perms=""
for acct in "${accounts[@]}"; do
  acct="${acct// /}" # trim spaces.
  [[ -z "${acct}" ]] && continue
  launch_perms+="{\"UserId\":\"${acct}\"},"
done
launch_perms="[${launch_perms%,}]" # strip trailing comma, wrap in array.

aws ec2 modify-image-attribute \
  --region "${ATMOS_AMI_REGION}" \
  --image-id "${ami_id}" \
  --launch-permission "Add=${launch_perms}"

# 2) Find the snapshots backing the AMI and share each one.
mapfile -t snapshot_ids < <(aws ec2 describe-images \
  --region "${ATMOS_AMI_REGION}" \
  --image-ids "${ami_id}" \
  --query 'Images[0].BlockDeviceMappings[].Ebs.SnapshotId' \
  --output text | tr '\t' '\n')

for snap in "${snapshot_ids[@]}"; do
  [[ -z "${snap}" || "${snap}" == "None" ]] && continue
  echo "==> Sharing snapshot ${snap}"
  for acct in "${accounts[@]}"; do
    acct="${acct// /}"
    [[ -z "${acct}" ]] && continue
    aws ec2 modify-snapshot-attribute \
      --region "${ATMOS_AMI_REGION}" \
      --snapshot-id "${snap}" \
      --attribute createVolumePermission \
      --operation-type add \
      --user-ids "${acct}"
  done
done

# 3) Optional KMS grants so target accounts can use the encryption key.
if [[ "${kms_grant}" == "true" ]]; then
  if [[ -z "${kms_key_arn}" ]]; then
    echo "==> --kms-grant set but the stack has no kms_key_arn (AMI uses the default key); skipping grants"
  else
    for acct in "${accounts[@]}"; do
      acct="${acct// /}"
      [[ -z "${acct}" ]] && continue
      echo "==> Creating KMS grant on ${kms_key_arn} for ${acct}"
      aws kms create-grant \
        --region "${ATMOS_AMI_REGION}" \
        --key-id "${kms_key_arn}" \
        --grantee-principal "arn:aws:iam::${acct}:root" \
        --operations Decrypt DescribeKey CreateGrant \
        --query 'GrantId' --output text
    done
  fi
fi

echo "==> Share complete"
