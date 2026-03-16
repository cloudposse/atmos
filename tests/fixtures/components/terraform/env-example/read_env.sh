#!/bin/sh
# Read and discard stdin (required by terraform external data source protocol).
cat > /dev/null

# Escape backslashes for valid JSON output (Windows paths contain backslashes).
escape_json() {
  printf '%s' "$1" | sed 's/\\/\\\\/g'
}

# Output environment variables as JSON.
printf '{"atmos_base_path":"%s","atmos_cli_config_path":"%s","example":"%s"}\n' \
  "$(escape_json "$ATMOS_BASE_PATH")" \
  "$(escape_json "$ATMOS_CLI_CONFIG_PATH")" \
  "$(escape_json "$EXAMPLE")"
