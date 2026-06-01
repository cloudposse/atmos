#!/usr/bin/env bash
#
# patch-os.sh — apply all available OS updates to the image.
#
# Runs first so every later step builds on a fully patched base.
#
# Note: we deliberately do NOT reboot the build instance. A new kernel pulled by
# the update is written to the image and becomes active when instances are
# launched from the resulting AMI — the build instance itself does not need to
# run it. (Rebooting mid-build would drop Packer's SSH connection; handling that
# would require splitting this into a separate provisioner with
# `expect_disconnect = true`, which this Lab keeps simple by avoiding.)
#
# Executed as root by the Packer shell provisioner (see main.pkr.hcl).

set -euo pipefail

echo "==> Applying OS updates"
dnf -y upgrade --refresh

running_kernel="$(uname -r)"
latest_kernel="$(rpm -q --last kernel | head -n 1 | sed 's/^kernel-//;s/ .*//')"
if [[ "${running_kernel}" != "${latest_kernel}" ]]; then
  echo "==> New kernel ${latest_kernel} installed; it activates when instances launch from this AMI"
else
  echo "==> Kernel is already current (${running_kernel})"
fi
