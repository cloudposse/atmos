#!/usr/bin/env bash
#
# get-ami-id.sh — print the AMI ID of the most recent Packer build.
#
# Backs `atmos ami get-ami-id <component> -s <stack>`.

set -euo pipefail
# shellcheck source=scripts/atmos/_lib.sh
source "$(dirname "$0")/_lib.sh"

resolve_ami_id
echo
