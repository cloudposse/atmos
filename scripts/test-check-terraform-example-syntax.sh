#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo="$(mktemp -d)"
trap 'rm -rf "$repo"' EXIT

mkdir -p "$repo/scripts"
cp "$root/scripts/check-terraform-example-syntax.sh" "$repo/scripts/"
git -C "$repo" init --quiet

run_guard() {
  (cd "$repo" && scripts/check-terraform-example-syntax.sh)
}

stage() {
  git -C "$repo" add -A
}

mkdir -p "$repo/stacks"
printf '%s\n' 'value: !terraform.output vpc .id // "fallback"' > "$repo/stacks/clean.yaml"
stage
run_guard

printf '%s\n' 'value: !terraform.output vpc ".id // ""fallback"""' > "$repo/stacks/legacy.yaml"
stage
if run_guard; then
  echo "expected legacy Terraform syntax to fail" >&2
  exit 1
fi
rm "$repo/stacks/legacy.yaml"

mkdir -p "$repo/pkg/function/parser" "$repo/internal/exec" "$repo/agent-skills/skills/atmos-modernization"
printf '%s\n' 'legacy: !terraform.output vpc ".id // ""fallback"""' > "$repo/pkg/function/parser/parser_test.go"
printf '%s\n' 'legacy: !terraform.state vpc dev ".id // ""fallback"""' > "$repo/internal/exec/yaml_func_terraform_state_yq_defaults_test.go"
printf '%s\n' 'legacy: !terraform.output vpc ".id // ""fallback"""' > "$repo/agent-skills/skills/atmos-modernization/SKILL.md"
stage
run_guard

mkdir -p "$repo/bin"
printf '%s\n' '#!/usr/bin/env bash' 'exit 2' > "$repo/bin/git"
chmod +x "$repo/bin/git"
if PATH="$repo/bin:$PATH" run_guard; then
  echo "expected git grep failure to propagate" >&2
  exit 1
fi

echo "Terraform example syntax guard regression tests passed."
