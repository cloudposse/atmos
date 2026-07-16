#!/usr/bin/env bash
set -euo pipefail

# Legacy CSV quote escaping is supported only by parser/runtime compatibility
# tests and the modernization skill, which needs literal legacy examples so
# agents can identify and replace them. User-facing examples must use the
# first-class function syntax.
matches="$(git grep -n -E '!terraform\.(state|output).*""|!terraform\.(state|output).*\\"' -- ':!**/*_test.go' ':!pkg/function/parser/**' ':!agent-skills/skills/atmos-modernization/SKILL.md' || true)"

if [[ -n "$matches" ]]; then
  echo "Legacy Terraform CSV quote escaping found outside compatibility tests:" >&2
  echo "$matches" >&2
  exit 1
fi

echo "Terraform examples use clean function syntax."
