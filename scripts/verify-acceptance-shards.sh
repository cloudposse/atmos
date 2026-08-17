#!/usr/bin/env bash
set -euo pipefail

target="${1:-}"
shard_count="${2:-${TEST_SHARD_COUNT:-}}"
binary_dir="${3:-build}"
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"
case "$binary_dir" in
/*) ;;
*) binary_dir="$repo_root/$binary_dir" ;;
esac

# shellcheck source=scripts/lib/acceptance-shards.sh
source "$repo_root/scripts/lib/acceptance-shards.sh"

acceptance_validate_target "$target"
acceptance_validate_shard 1 "$shard_count"

plan_dir=$(mktemp -d)
cleanup() {
	rm -rf "$plan_dir"
}
trap cleanup EXIT

expected_packages="$plan_dir/expected"
assigned_packages="$plan_dir/assigned"
: > "$expected_packages"
: > "$assigned_packages"

all_packages=$(go list ./...)
while IFS= read -r package; do
	[ -n "$package" ] || continue
	case "$package" in
	"$acceptance_tests_package" | "$acceptance_cmd_package")
		# ./tests has an explicit filtered route below; cmd runs as a binary.
		;;
	"$acceptance_exec_package")
		if [ "$target" != "windows" ]; then
			printf '%s\n' "$package" >> "$expected_packages"
		fi
		;;
	*) printf '%s\n' "$package" >> "$expected_packages" ;;
	esac
done <<< "$all_packages"

shard=1
while [ "$shard" -le "$shard_count" ]; do
	planned_packages=$(acceptance_shard_packages "$target" "$shard" "$shard_count")
	while IFS= read -r package; do
		[ -n "$package" ] || continue
		printf '%s\n' "$package" >> "$assigned_packages"
	done <<< "$planned_packages"
	shard=$((shard + 1))
done

failed=0
LC_ALL=C sort -u "$expected_packages" -o "$expected_packages.sorted"
LC_ALL=C sort "$assigned_packages" -o "$assigned_packages.sorted"

duplicates=$(uniq -d "$assigned_packages.sorted")
missing=$(comm -23 "$expected_packages.sorted" "$assigned_packages.sorted")
unexpected=$(comm -13 "$expected_packages.sorted" "$assigned_packages.sorted")
if [ -n "$duplicates" ]; then
	echo "packages assigned more than once:" >&2
	echo "$duplicates" >&2
	failed=1
fi
if [ -n "$missing" ]; then
	echo "packages with no shard assignment:" >&2
	echo "$missing" >&2
	failed=1
fi
if [ -n "$unexpected" ]; then
	echo "unexpected package assignments:" >&2
	echo "$unexpected" >&2
	failed=1
fi

extension=""
if [ "$target" = "windows" ]; then
	extension=".exe"
fi
if [ ! -f "$binary_dir/cmd.test$extension" ]; then
	echo "missing precompiled cmd test binary: $binary_dir/cmd.test$extension" >&2
	failed=1
fi
if [ "$target" = "windows" ]; then
	for binary in tests.test.exe internal-exec.test.exe; do
		if [ ! -f "$binary_dir/$binary" ]; then
			echo "missing precompiled Windows test binary: $binary_dir/$binary" >&2
			failed=1
		fi
	done

	if [ -f "$binary_dir/tests.test.exe" ]; then
		tests_list=$(cd tests && "$binary_dir/tests.test.exe" -test.list '^Test')
		for required_test in TestCLICommands TestTerraformRegistryCache; do
			if ! grep -Fxq "$required_test" <<< "$tests_list"; then
				echo "precompiled tests binary does not contain $required_test" >&2
				failed=1
			fi
		done
	fi
fi

# These filters are the routing contract for ./tests. Shard 1 runs every
# top-level test except the separately routed registry-cache test; every other
# shard runs its internally partitioned TestCLICommands cases only.
if [ "$(acceptance_go_test_args 1)" != '-skip=^TestTerraformRegistryCache$' ] ||
	[ "$(acceptance_binary_test_args 1)" != '-test.skip=^TestTerraformRegistryCache$' ]; then
	echo "shard 1 ./tests routing no longer covers every non-registry top-level test" >&2
	failed=1
fi
if [ "$shard_count" -gt 1 ] &&
	{ [ "$(acceptance_go_test_args 2)" != '-run=^TestCLICommands$ -skip=^TestTerraformRegistryCache$' ] ||
		[ "$(acceptance_binary_test_args 2)" != '-test.run=^TestCLICommands$ -test.skip=^TestTerraformRegistryCache$' ]; }; then
	echo "shards 2+ ./tests routing no longer selects TestCLICommands exclusively" >&2
	failed=1
fi

# The matrix is intentionally explicit in GitHub Actions. Prove it still spans
# every shard in TEST_SHARD_COUNT, otherwise valid CLI workdir assignments can
# be calculated for a shard that no job ever executes.
matrix_line=$(sed -n -E 's/^[[:space:]]*shard: \[([^]]+)\][[:space:]]*$/\1/p' .github/workflows/test.yml)
if [ "$(printf '%s\n' "$matrix_line" | sed '/^$/d' | wc -l | tr -d ' ')" -ne 1 ]; then
	echo "could not identify exactly one explicit acceptance shard matrix" >&2
	failed=1
else
	IFS=',' read -r -a matrix_shards <<< "$matrix_line"
	if [ "${#matrix_shards[@]}" -ne "$shard_count" ]; then
		echo "workflow has ${#matrix_shards[@]} shards; expected $shard_count" >&2
		failed=1
	else
		for ((index = 0; index < shard_count; index++)); do
			actual="${matrix_shards[$index]//[[:space:]]/}"
			expected=$((index + 1))
			if [ "$actual" != "$expected" ]; then
				echo "workflow shard position $expected contains $actual" >&2
				failed=1
			fi
		done
	fi
fi

if ! grep -Fq "run: go test ./tests -run '^TestTerraformRegistryCache$'" .github/workflows/test.yml; then
	echo "TestTerraformRegistryCache has no dedicated workflow route" >&2
	failed=1
fi

if [ "$failed" -ne 0 ]; then
	exit 1
fi

printf 'Verified %d package assignments across %s shard(s) for %s\n' \
	"$(wc -l < "$expected_packages.sorted" | tr -d ' ')" "$shard_count" "$target"
