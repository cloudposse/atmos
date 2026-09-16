#!/usr/bin/env bash

set -euo pipefail

LOCAL_DIR="${1:?local directory required (e.g., ./website/build)}"
S3_URI="${2:?S3 URI required with trailing slash (e.g., s3://my-bucket/pr-123/)}"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ARGS=("${LOCAL_DIR}" "${S3_URI}")
while IFS= read -r pattern; do
	[[ -n "${pattern}" ]] && ARGS+=(--protect "${pattern}")
done <<< "${PROTECTED_PATTERNS:-}"

echo "::group::Identity"
aws sts get-caller-identity
echo "::endgroup::"

exec python3 "${SCRIPT_DIR}/deploy.py" "${ARGS[@]}"
