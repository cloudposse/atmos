#!/usr/bin/env bash
#
# harden.sh — baseline OS hardening for the image.
#
# Applies a conservative, widely-applicable hardening baseline (SSH, sysctl,
# auditd). Heavier controls that can break connectivity or workloads — host
# firewall and SELinux enforcing — are OFF by default and gated behind toggles
# passed from the stack (`provisioner_env_vars`):
#
#   ENABLE_FIREWALL=true            install + enable firewalld
#   ENABLE_SELINUX_ENFORCING=true   switch SELinux from permissive to enforcing
#
# This is a teaching baseline, not a certified CIS benchmark — adapt it to your
# own compliance requirements.
#
# Executed as root by the Packer shell provisioner (see main.pkr.hcl).

set -euo pipefail

# Toggles default to "false" if the stack did not set them.
ENABLE_FIREWALL="${ENABLE_FIREWALL:-false}"
ENABLE_SELINUX_ENFORCING="${ENABLE_SELINUX_ENFORCING:-false}"

echo "==> Hardening SSH daemon"
sshd_config="/etc/ssh/sshd_config.d/99-hardening.conf"
cat > "${sshd_config}" <<'EOF'
# Disable password and root login; key-based auth only.
PasswordAuthentication no
PermitRootLogin no
PermitEmptyPasswords no
# Reasonable session limits.
ClientAliveInterval 300
ClientAliveCountMax 2
MaxAuthTries 4
EOF
chmod 0600 "${sshd_config}"

echo "==> Applying sysctl network hardening"
cat > /etc/sysctl.d/99-hardening.conf <<'EOF'
# Ignore ICMP redirects and source-routed packets.
net.ipv4.conf.all.accept_redirects = 0
net.ipv4.conf.all.send_redirects = 0
net.ipv4.conf.all.accept_source_route = 0
# Enable reverse-path filtering and log martians.
net.ipv4.conf.all.rp_filter = 1
net.ipv4.conf.all.log_martians = 1
# SYN flood protection.
net.ipv4.tcp_syncookies = 1
EOF
sysctl --system >/dev/null

echo "==> Enabling the audit daemon"
dnf -y install audit >/dev/null
systemctl enable auditd

if [[ "${ENABLE_FIREWALL}" == "true" ]]; then
  echo "==> ENABLE_FIREWALL=true: installing and enabling firewalld"
  dnf -y install firewalld >/dev/null
  systemctl enable firewalld
else
  echo "==> Skipping host firewall (ENABLE_FIREWALL is not true)"
fi

if [[ "${ENABLE_SELINUX_ENFORCING}" == "true" ]]; then
  echo "==> ENABLE_SELINUX_ENFORCING=true: setting SELinux to enforcing"
  # Persist for future boots; AL2023 ships SELinux in permissive mode.
  sed -i 's/^SELINUX=.*/SELINUX=enforcing/' /etc/selinux/config
else
  echo "==> Leaving SELinux at its default mode (ENABLE_SELINUX_ENFORCING is not true)"
fi

echo "==> Hardening complete"
