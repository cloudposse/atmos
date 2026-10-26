#!/bin/sh
set -eu

test -n "${DATADOG_API_KEY:-}"
test -n "${DB_PASSWORD:-}"

echo "Component command received declared secrets"
