#!/usr/bin/env bash
# Example greeting script for the `hello` custom component type.
#
# This file is illustrative — the custom command in atmos.yaml inlines the
# equivalent steps. In a real custom component type, your command logic would
# live here (or in any tool) and publish results by appending KEY=VALUE lines
# (or a JSON object) to the file Atmos exports as $ATMOS_OUTPUTS:
#
#   echo "greeting=$GREETING" >> "$ATMOS_OUTPUTS"
#
# Atmos reads $ATMOS_OUTPUTS after the command succeeds and makes those values
# available to the after.hello.greeting hook via leading-dot output references.

set -euo pipefail

echo "greeting=${GREETING:-hello, world}" >>"$ATMOS_OUTPUTS"
echo "greeted_component=${COMPONENT:-world}" >>"$ATMOS_OUTPUTS"
