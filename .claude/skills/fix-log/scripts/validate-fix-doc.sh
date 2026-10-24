#!/usr/bin/env bash
set -euo pipefail

if [[ "$#" -eq 0 ]]; then
  echo "usage: $0 <fix-doc> [<fix-doc> ...]" >&2
  exit 2
fi

status=0

for file in "$@"; do
  base="$(basename "$file")"

  if [[ "$base" == "README.md" ]]; then
    echo "skip: $file"
    continue
  fi

  if [[ ! -f "$file" ]]; then
    echo "error: not a file: $file" >&2
    status=1
    continue
  fi

  if [[ ! "$base" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}-[a-z0-9]+(-[a-z0-9]+)*\.md$ ]]; then
    echo "error: invalid filename: $file" >&2
    status=1
  fi

  for section in "Summary" "Context" "Changes" "Validation" "Follow-ups"; do
    if ! grep -Eq "^## ${section}$" "$file"; then
      echo "error: missing section ## ${section}: $file" >&2
      status=1
    fi
  done
done

exit "$status"
