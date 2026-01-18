#!/bin/bash
# Setup mock terraform states for demo recordings
# This allows atmos commands that read terraform state to work without actual infrastructure

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ACME_DIR="$(dirname "$SCRIPT_DIR")"
COMPONENTS_DIR="$ACME_DIR/components/terraform"

# List of stacks to create mock state for
STACKS=(
  "plat-dev-ue2"
  "plat-dev-uw2"
  "plat-prod-euw1"
  "plat-prod-ue2"
  "plat-staging-ue2"
)

# Components that need mock state
COMPONENTS=(
  "vpc"
  "cluster"
  "database"
  "api"
  "cdn"
)

echo "Setting up mock terraform states..."

for component in "${COMPONENTS[@]}"; do
  COMPONENT_DIR="$COMPONENTS_DIR/$component"
  TF_DIR="$COMPONENT_DIR/terraform.tfstate.d"

  # Create terraform.tfstate.d directory for workspaces
  mkdir -p "$TF_DIR"

  for stack in "${STACKS[@]}"; do
    WORKSPACE_DIR="$TF_DIR/$stack"
    mkdir -p "$WORKSPACE_DIR"

    # Copy mock state file
    if [ -f "$SCRIPT_DIR/${component}.tfstate" ]; then
      cp "$SCRIPT_DIR/${component}.tfstate" "$WORKSPACE_DIR/terraform.tfstate"
      echo "  Created: $component ($stack)"
    fi
  done
done

echo "Mock states setup complete!"
