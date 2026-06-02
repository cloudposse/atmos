#!/usr/bin/env bash
#
# install-packages.sh — install the runtime packages the image should ship with.
#
# This is the list you most likely want to edit. It installs a small, generic
# set of utilities. Replace the PACKAGES list with whatever your workloads need
# (language runtimes, agents, CLIs, etc.).
#
# Executed as root by the Packer shell provisioner (see main.pkr.hcl).

set -euo pipefail

# Edit this list for your image. Kept deliberately small and generic so the Lab
# builds quickly and has no proprietary dependencies.
PACKAGES=(
  jq          # JSON processing.
  unzip       # Common archive handling.
  chrony      # Time synchronization.
  cronie      # Cron daemon.
)

echo "==> Installing packages: ${PACKAGES[*]}"
dnf -y install "${PACKAGES[@]}"

echo "==> Enabling baseline services"
# `enable` (without `--now`) registers the units so they start on first boot of
# instances launched from this AMI. We deliberately do NOT start them on the
# build instance, to avoid baking runtime state into the image.
systemctl enable chronyd
systemctl enable crond

echo "==> Package installation complete"
