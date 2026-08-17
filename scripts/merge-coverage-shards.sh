#!/usr/bin/env bash
set -euo pipefail

shard_count="${TEST_SHARD_COUNT:-}"
shards_dir="${COVERAGE_SHARDS_DIR:-coverage/shards}"
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

if ! [[ "$shard_count" =~ ^[0-9]+$ ]] || [ "$shard_count" -lt 1 ]; then
	echo "TEST_SHARD_COUNT must be a positive integer" >&2
	exit 1
fi

coverage_inputs=()
shard=1
while [ "$shard" -le "$shard_count" ]; do
	shard_dir="$shards_dir/shard-$shard"
	shard_meta=$(find "$shard_dir" -type f -name 'covmeta.*' -print -quit)
	if [ -z "$shard_meta" ]; then
		echo "no native coverage metadata found for shard $shard" >&2
		exit 1
	fi
	coverage_inputs+=("$(dirname "$shard_meta")")
	shard=$((shard + 1))
done

COVERAGE_DATA_OUT="${COVERAGE_DATA_OUT:-coverage/merged}" \
	COVERAGE_OUT="${COVERAGE_OUT-coverage.out}" \
	scripts/merge-coverage.sh "${coverage_inputs[@]}"
