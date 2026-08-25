#!/usr/bin/env sh
set -eu

# Run from the repo (or worktree) root regardless of the caller's cwd. This is
# a standalone script as well as an Atmos custom-command entrypoint, so it
# resolves its own working directory.
cd "$(git rev-parse --show-toplevel)"

target="${1:-${ATMOS_BUILD_TARGET:-default}}"
version="${2:-${ATMOS_BUILD_VERSION:-test}}"
# Full commit SHA, so the scaffold catalog can pin distributable templates to
# the exact commit this binary was built from (works from any pushed branch,
# not just tagged releases). Empty when not building from a git checkout.
commit="$(git rev-parse HEAD 2>/dev/null || true)"

case "$target" in
	default)
		goos="${GOOS:-}"
		goarch="${GOARCH:-}"
		output="build/atmos"
		;;
	linux)
		goos="linux"
		goarch="${GOARCH:-}"
		output="build/atmos"
		;;
	windows)
		goos="windows"
		goarch="${GOARCH:-}"
		output="build/atmos.exe"
		;;
	macos)
		goos="darwin"
		goarch="${GOARCH:-}"
		output="build/atmos"
		;;
	macos-intel)
		goos="darwin"
		goarch="amd64"
		output="build/atmos"
		;;
	*)
		echo "Unsupported build target: $target" >&2
		echo "Expected one of: default, linux, windows, macos, macos-intel" >&2
		exit 1
		;;
esac

export CGO_ENABLED="${CGO_ENABLED:-0}"
if git_dir="$(git rev-parse --git-dir 2>/dev/null)" &&
	git_common_dir="$(git rev-parse --git-common-dir 2>/dev/null)" &&
	[ "$git_dir" != "$git_common_dir" ]; then
	export GOFLAGS="${GOFLAGS:--buildvcs=false}"
fi

# proxy.golang.org occasionally drops a request mid-stream (HTTP/2 "stream
# error: ... INTERNAL_ERROR received from peer") -- a transient CDN/network
# blip, not a real dependency problem -- which otherwise fails the whole
# build outright. Retry a few times with a short cooldown, matching the
# 3-attempt/15s-backoff convention already used for artifact downloads (see
# .github/actions/download-artifact-retry).
attempt=1
max_attempts=3
until go mod download; do
	if [ "$attempt" -ge "$max_attempts" ]; then
		echo "go mod download failed after $max_attempts attempts" >&2
		exit 1
	fi
	echo "go mod download failed (attempt $attempt/$max_attempts), retrying in 15s..." >&2
	sleep 15
	attempt=$((attempt + 1))
done
mkdir -p build

if [ -n "$goos" ] && [ -n "$goarch" ]; then
	GOOS="$goos" GOARCH="$goarch" go build -o "$output" -v \
		-ldflags "-X 'github.com/cloudposse/atmos/pkg/version.Version=$version' -X 'github.com/cloudposse/atmos/pkg/version.Commit=$commit'"
elif [ -n "$goos" ]; then
	GOOS="$goos" go build -o "$output" -v \
		-ldflags "-X 'github.com/cloudposse/atmos/pkg/version.Version=$version' -X 'github.com/cloudposse/atmos/pkg/version.Commit=$commit'"
elif [ -n "$goarch" ]; then
	GOARCH="$goarch" go build -o "$output" -v \
		-ldflags "-X 'github.com/cloudposse/atmos/pkg/version.Version=$version' -X 'github.com/cloudposse/atmos/pkg/version.Commit=$commit'"
else
	go build -o "$output" -v \
		-ldflags "-X 'github.com/cloudposse/atmos/pkg/version.Version=$version' -X 'github.com/cloudposse/atmos/pkg/version.Commit=$commit'"
fi
