#!/bin/sh
# Read and discard stdin (required by terraform external data source protocol).
cat > /dev/null

# Output environment variables as JSON.
# Values are filesystem paths, safe for direct JSON interpolation.
cat <<EOF
{
  "atmos_base_path": "${ATMOS_BASE_PATH}",
  "atmos_cli_config_path": "${ATMOS_CLI_CONFIG_PATH}",
  "example": "${EXAMPLE}"
}
EOF
