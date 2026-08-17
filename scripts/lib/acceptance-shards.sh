#!/usr/bin/env bash

# Shared acceptance-shard planning. Keep package selection here so the runner
# and the completeness verifier cannot silently drift apart.

acceptance_tests_package="github.com/cloudposse/atmos/tests"
acceptance_cmd_package="github.com/cloudposse/atmos/cmd"
acceptance_exec_package="github.com/cloudposse/atmos/internal/exec"
acceptance_testhelpers_package="github.com/cloudposse/atmos/tests/testhelpers"

acceptance_validate_target() {
	case "$1" in
	linux | macos | windows) ;;
	*)
		echo "invalid acceptance target: $1" >&2
		return 1
		;;
	esac
}

acceptance_validate_shard() {
	local shard="$1"
	local shard_count="$2"

	if ! [[ "$shard" =~ ^[0-9]+$ && "$shard_count" =~ ^[0-9]+$ ]] ||
		[ "$shard" -lt 1 ] || [ "$shard" -gt "$shard_count" ]; then
		echo "invalid shard $shard/$shard_count" >&2
		return 1
	fi
}

acceptance_general_packages() {
	go list ./... | awk \
		-v tests="$acceptance_tests_package" \
		-v cmd="$acceptance_cmd_package" \
		-v exec="$acceptance_exec_package" \
		-v helpers="$acceptance_testhelpers_package" '
		$0 == tests || $0 == cmd || $0 == exec || $0 == helpers { next }
		{ print }
	'
}

acceptance_shard_packages() {
	local target="$1"
	local shard="$2"
	local shard_count="$3"

	acceptance_validate_target "$target"
	acceptance_validate_shard "$shard" "$shard_count"

	acceptance_general_packages | awk -v shard="$shard" -v count="$shard_count" '
		{
			eligible++
			if (eligible % count == shard - 1) print
		}
	'

	# Windows executes internal/exec from the precompiled test binary instead.
	if [ "$target" != "windows" ] && [ "$shard" = "2" ]; then
		printf '%s\n' "$acceptance_exec_package"
	fi
	if [ "$shard" = "3" ]; then
		printf '%s\n' "$acceptance_testhelpers_package"
	fi
}

acceptance_go_test_args() {
	local shard="$1"
	if [ "$shard" = "1" ]; then
		printf '%s\n' '-skip=^TestTerraformRegistryCache$'
	else
		printf '%s\n' '-run=^TestCLICommands$ -skip=^TestTerraformRegistryCache$'
	fi
}

acceptance_binary_test_args() {
	local shard="$1"
	if [ "$shard" = "1" ]; then
		printf '%s\n' '-test.skip=^TestTerraformRegistryCache$'
	else
		printf '%s\n' '-test.run=^TestCLICommands$ -test.skip=^TestTerraformRegistryCache$'
	fi
}
