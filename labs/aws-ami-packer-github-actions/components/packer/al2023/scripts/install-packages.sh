#!/usr/bin/env bash
#
# install-packages.sh — install the runtime packages the image should ship with.
#
# This is the list you most likely want to edit. It installs a small, generic
# set of utilities and enables the SSM agent (which ships with AL2023 but is
# worth enabling explicitly). Replace the PACKAGES list with whatever your
# workloads need (language runtimes, agents, CLIs, etc.).
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
  amazon-ssm-agent # Remote management without SSH (preinstalled on AL2023).
)

echo "==> Installing packages: ${PACKAGES[*]}"
dnf -y install "${PACKAGES[@]}"

echo "==> Enabling baseline services"
# Enable now so they start on first boot of instances launched from the AMI.
systemctl enable --now chronyd
systemctl enable --now crond
systemctl enable --now amazon-ssm-agent

echo "==> Package installation complete"
