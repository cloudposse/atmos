#!/usr/bin/env bash
#
# install-scan-agent.sh — OPTIONAL: install a vulnerability-scanning agent.
#
# This is the integration point for a commercial security scanner (e.g. Qualys,
# Tenable, Wiz, CrowdStrike). Such agents are distributed from a private package
# repository and require a subscription, so this step is DISABLED by default and
# the Lab runs fine without it.
#
# Enable it from the stack by setting these `provisioner_env_vars`:
#
#   ENABLE_SCAN_AGENT=true
#   SCAN_AGENT_REPO_URL=https://packages.example.com/scan-agent   (your repo)
#
# The body below is a generic, vendor-neutral skeleton: add a temporary dnf
# repo, install the agent package, activate it with your credentials, then
# remove the repo so the credentials are not baked into the AMI. Replace the
# placeholder package name and activation command with your scanner's specifics.
#
# Executed as root by the Packer shell provisioner (see main.pkr.hcl).

set -euo pipefail

ENABLE_SCAN_AGENT="${ENABLE_SCAN_AGENT:-false}"
SCAN_AGENT_REPO_URL="${SCAN_AGENT_REPO_URL:-}"

if [[ "${ENABLE_SCAN_AGENT}" != "true" ]]; then
  echo "==> Scan agent disabled (ENABLE_SCAN_AGENT is not true); skipping"
  exit 0
fi

if [[ -z "${SCAN_AGENT_REPO_URL}" ]]; then
  echo "ERROR: ENABLE_SCAN_AGENT=true but SCAN_AGENT_REPO_URL is empty" >&2
  exit 1
fi

# Do not echo SCAN_AGENT_REPO_URL — it may embed credentials (e.g. a token in the
# URL) that would leak into the build log.
echo "==> Installing vulnerability scan agent from configured private repository"

# 1) Add a temporary repo pointing at your private package server. Remove it on
#    exit — even if a later step fails — so the private repo URL/credentials are
#    never baked into the AMI.
repo_file="/etc/yum.repos.d/scan-agent.repo"
trap 'rm -f "${repo_file}"' EXIT
cat > "${repo_file}" <<EOF
[scan-agent]
name=Vulnerability Scan Agent
baseurl=${SCAN_AGENT_REPO_URL}
enabled=1
# Keep signature verification ON. Point gpgkey at your vendor's signing key, or
# pre-import the key into the image trust store before installing.
gpgcheck=1
# gpgkey=https://packages.example.com/keys/RPM-GPG-KEY-scan-agent
EOF

# 2) Install the agent package. Replace 'scan-agent' with your vendor's package.
dnf -y install scan-agent

# 3) Activate the agent with your subscription credentials. Pass these as build
#    secrets (env vars), never hardcode them. Replace with your vendor's command.
#    Example:
#    scan-agent-ctl activate --customer-id "${SCAN_CUSTOMER_ID}" \
#      --activation-id "${SCAN_ACTIVATION_ID}"

echo "==> Scan agent installed"
# The temporary repo file is removed by the EXIT trap registered above.
