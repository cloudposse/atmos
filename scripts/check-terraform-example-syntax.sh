#!/usr/bin/env bash
set -euo pipefail

# Legacy CSV quote escaping begins with an outer double-quoted expression that
# contains doubled inner quotes. It is supported only by the parser fixture,
# the state-default compatibility fixture, and the modernization skill, which
# needs literal legacy examples so agents can identify and replace them.
legacy_pattern='!terraform\.(state|output)[[:space:]]+[^[:space:]]+([[:space:]]+[^[:space:]]+)?[[:space:]]+"[^"]*""'
matches=""
if matches="$(git grep -n -E "$legacy_pattern" -- ':!pkg/function/parser/**' ':!internal/exec/yaml_func_terraform_state_yq_defaults_test.go' ':!agent-skills/skills/atmos-modernization/SKILL.md')"; then
  :
else
  status=$?
  if [[ $status -ne 1 ]]; then
    exit "$status"
  fi
fi

if [[ -n "$matches" ]]; then
  echo "Legacy Terraform CSV quote escaping found outside compatibility tests:" >&2
  echo "$matches" >&2
  exit 1
fi

echo "Terraform examples use clean function syntax."
