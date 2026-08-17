#!/usr/bin/env bash
set -euo pipefail

mode="${1:-}"
target="${2:-}"
shard="${ATMOS_TEST_SHARD:-}"
shard_count="${ATMOS_TEST_SHARD_COUNT:-${TEST_SHARD_COUNT:-}}"
atmos_bin="${ATMOS_BIN:-atmos}"
workspace=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$workspace"

if [ "$mode" != "test" ] && [ "$mode" != "coverage" ]; then
	echo "usage: $0 <test|coverage> <linux|macos|windows>" >&2
	exit 1
fi
if [ "$target" != "linux" ] && [ "$target" != "macos" ] && [ "$target" != "windows" ]; then
	echo "usage: $0 <test|coverage> <linux|macos|windows>" >&2
	exit 1
fi
if [ -z "$shard" ] || [ -z "$shard_count" ]; then
	echo "ATMOS_TEST_SHARD and ATMOS_TEST_SHARD_COUNT are required" >&2
	exit 1
fi
if ! [[ "$shard" =~ ^[0-9]+$ && "$shard_count" =~ ^[0-9]+$ ]] ||
	[ "$shard" -lt 1 ] || [ "$shard" -gt "$shard_count" ]; then
	echo "invalid shard $shard/$shard_count" >&2
	exit 1
fi

# TestTerraformRegistryCache runs in its own dedicated workflow job, so every
# shard excludes it here. Shard 1 runs the other top-level acceptance tests.
# Shards 2+ run only TestCLICommands; otherwise roughly 280 unsharded tests run
# ten times, multiplying runtime and exposure to toolchain/network flakes.
if [ "$shard" = "1" ]; then
	tests_args="-skip=^TestTerraformRegistryCache$"
else
	tests_args="-run=^TestCLICommands$ -skip=^TestTerraformRegistryCache$"
fi

# Every non-tests package is round-robined across the same shard count. cmd is
# excluded because its cold Windows test-binary link takes 15+ minutes; the
# build job precompiles it after warming the Go cache, and shard 1 runs it.
# internal/exec and tests/testhelpers are pinned to shards 2 and 3 because real
# Windows runs measured them at 930s and 413s, respectively, and both otherwise
# landed on already-heavy shard 1.
shard_packages=$(go list ./... | awk -v shard="$shard" -v count="$shard_count" '
	$0 ~ /\/tests$/ ||
	$0 == "github.com/cloudposse/atmos/cmd" ||
	$0 == "github.com/cloudposse/atmos/internal/exec" ||
	$0 == "github.com/cloudposse/atmos/tests/testhelpers" { next }
	{
		eligible++
		if (eligible % count == shard - 1) print
	}
')

append_package() {
	if [ -n "$shard_packages" ]; then
		shard_packages+=$'\n'
	fi
	shard_packages+="$1"
}

if [ "$shard" = "2" ]; then
	append_package "github.com/cloudposse/atmos/internal/exec"
fi
if [ "$shard" = "3" ]; then
	append_package "github.com/cloudposse/atmos/tests/testhelpers"
fi

coverage_root="$workspace/coverage"
coverage_inputs=()
temporary_cmd_cover_dir=""

cleanup() {
	if [ -n "$temporary_cmd_cover_dir" ]; then
		rm -rf "$temporary_cmd_cover_dir"
	fi
}
trap cleanup EXIT

run_test_group() {
	local name="$1"
	local packages="$2"
	local args="$3"

	if [ "$mode" = "coverage" ]; then
		local data_dir="$coverage_root/shard-$shard-$name"
		COVERAGE_DIR="$coverage_root/shard-$shard-$name-work" \
			COVERAGE_DATA_OUT="$data_dir" COVERAGE_OUT="" \
			TEST="$packages" TESTARGS="$args" \
			"$atmos_bin" test acceptance --cover
		coverage_inputs+=("$data_dir")
	else
		TEST="$packages" TESTARGS="$args" "$atmos_bin" test acceptance
	fi
}

run_cmd_tests() {
	local cmd_test_bin="${CMD_TEST_BIN:-$workspace/cmd.test}"
	if [ -z "${CMD_TEST_BIN:-}" ] && [ "$target" = "windows" ]; then
		cmd_test_bin+=".exe"
	fi
	if [ "$target" != "windows" ]; then
		chmod +x "$cmd_test_bin"
	fi

	local cmd_cover_dir
	if [ "$mode" = "coverage" ]; then
		cmd_cover_dir="$coverage_root/shard-$shard-cmd"
		mkdir -p "$cmd_cover_dir"
		coverage_inputs+=("$cmd_cover_dir")
	else
		temporary_cmd_cover_dir=$(mktemp -d)
		cmd_cover_dir="$temporary_cmd_cover_dir"
	fi

	# The environment collects re-executed subprocesses. The explicit test flag
	# flushes the parent binary's counters when coverage is retained.
	if [ "$mode" = "coverage" ]; then
		(cd cmd && GOCOVERDIR="$cmd_cover_dir" "$cmd_test_bin" \
			-test.v -test.timeout=10m -test.gocoverdir="$cmd_cover_dir")
	else
		(cd cmd && GOCOVERDIR="$cmd_cover_dir" "$cmd_test_bin" \
			-test.v -test.timeout=10m)
	fi
}

# Keep ./tests separate from the package slice. Applying TestCLICommands' -run
# filter to a combined go test invocation would silently skip package tests.
run_test_group "tests" "./tests" "$tests_args"
if [ -n "$shard_packages" ]; then
	run_test_group "pkgs" "$shard_packages" ""
fi
if [ "$shard" = "1" ]; then
	run_cmd_tests
fi

if [ "$mode" = "coverage" ]; then
	# Preserve binary counters through artifact upload. The coverage job converts
	# to a Codecov text profile only after merging all downloaded shard data.
	COVERAGE_DATA_OUT="$coverage_root/shard-$shard" COVERAGE_OUT="" \
		scripts/merge-coverage.sh "${coverage_inputs[@]}"
fi
