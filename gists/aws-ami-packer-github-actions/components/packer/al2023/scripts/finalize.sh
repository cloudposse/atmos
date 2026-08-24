#!/usr/bin/env bash
#
# finalize.sh — clean instance-specific state so the AMI boots clean.
#
# Runs last. Removes data that must NOT be baked into a golden image: cached
# package metadata, machine identity, SSH host keys, logs, and shell history.
# This ensures every instance launched from the AMI gets fresh identity and
# does not inherit the build instance's footprint.
#
# Executed as root by the Packer shell provisioner (see main.pkr.hcl).

set -euo pipefail

echo "==> Cleaning package manager caches"
dnf -y clean all
rm -rf /var/cache/dnf

echo "==> Resetting cloud-init so the image re-initializes on next boot"
# Lets cloud-init re-run on first boot of a launched instance (new hostname,
# SSH keys, instance metadata, etc.).
cloud-init clean --logs || true

echo "==> Removing SSH host keys (regenerated on first boot)"
rm -f /etc/ssh/ssh_host_*

echo "==> Removing the build machine identity"
# A fresh machine-id is generated on next boot.
truncate -s 0 /etc/machine-id || true
rm -f /var/lib/dbus/machine-id

echo "==> Truncating logs"
find /var/log -type f -exec truncate -s 0 {} + 2>/dev/null || true

echo "==> Clearing shell history"
rm -f /root/.bash_history
rm -f /home/ec2-user/.bash_history 2>/dev/null || true

# NOTE: if you enabled the optional scan agent, strip its host-specific ID here
# so each launched instance registers as a new host. Example (vendor-specific):
#   rm -f /etc/scan-agent/hostid

echo "==> Finalize complete; image is ready"
