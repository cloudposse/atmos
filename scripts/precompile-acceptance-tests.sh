#!/usr/bin/env bash
set -euo pipefail

target="${1:-}"
output_dir="${2:-build}"
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

case "$target" in
linux | macos | windows) ;;
*)
	echo "usage: $0 <linux|macos|windows> [output-directory]" >&2
	exit 1
	;;
esac

mkdir -p "$output_dir"
extension=""
if [ "$target" = "windows" ]; then
	extension=".exe"
fi

# cmd is coverage-instrumented on every target so Linux can retain its native
# counters. Keep atomic mode aligned with scripts/collect-coverage.sh.
CGO_ENABLED=0 go test -c -covermode=atomic \
	-o "$output_dir/cmd.test$extension" ./cmd

if [ "$target" = "windows" ]; then
	# Windows spends most of each shard compiling these two large test packages
	# from a cold cache. Compile them once after the build job has warmed its Go
	# cache, then execute the binaries directly in the acceptance matrix.
	CGO_ENABLED=0 go test -c -o "$output_dir/tests.test.exe" ./tests
	CGO_ENABLED=0 go test -c -o "$output_dir/internal-exec.test.exe" ./internal/exec
fi
