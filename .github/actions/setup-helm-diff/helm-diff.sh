#!/usr/bin/env bash
set -euo pipefail

mode="${1:?Expected prepare or restore}"
version="${2:?Expected pinned version}"
archive="${3:?Expected archive path}"

case "$mode" in
	prepare | restore) ;;
	*) echo "Unknown Helm Diff mode: $mode" >&2; exit 1 ;;
esac

# Use a private directory for installation, retries, and each restored copy.
# Convert paths explicitly for native Helm on Windows and tar under Git Bash.
temp_root="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
if command -v cygpath >/dev/null 2>&1; then
	temp_root="$(cygpath -u "$temp_root")"
	archive="$(cygpath -u "$archive")"
fi
plugin_root="$(mktemp -d "$temp_root/helm-diff.XXXXXX")"
keep_plugins=false
trap 'if [[ "$keep_plugins" != true ]]; then rm -rf "$plugin_root"; fi' EXIT
export HELM_PLUGINS="$plugin_root/plugins"
if command -v cygpath >/dev/null 2>&1; then
	HELM_PLUGINS="$(cygpath -w "$HELM_PLUGINS")"
fi

verify_version() {
	local actual
	actual="$(helm diff version)"
	actual="${actual//$'\r'/}"
	if [[ "${actual#v}" != "${version#v}" ]]; then
		echo "Helm Diff version mismatch: expected $version, got $actual" >&2
		return 1
	fi
}

if [[ "$mode" == prepare ]]; then
	for attempt in 1 2 3; do
		# Helm may leave a registered but incomplete plugin after a failed hook.
		# Only remove this action's own staging directory before retrying.
		rm -rf "$plugin_root/plugins"
		mkdir -p "$plugin_root/plugins"
		if helm plugin install https://github.com/databus23/helm-diff --version "$version"; then
			break
		else
			status=$?
			if [[ "$attempt" == 3 ]]; then
				echo "Helm Diff installation failed after $attempt attempts" >&2
				exit "$status"
			fi
			sleep "$((attempt * 15))"
		fi
	done
	verify_version
	mkdir -p "$(dirname "$archive")"
	# Artifact uploads normalize file modes; a tarball preserves the executable.
	tar --exclude=.git -czf "$archive" -C "$plugin_root/plugins" .
else
	mkdir -p "$plugin_root/plugins"
	tar -xzf "$archive" -C "$plugin_root/plugins"
	verify_version
	printf 'HELM_PLUGINS=%s\n' "$HELM_PLUGINS" >> "${GITHUB_ENV:?Expected GitHub environment file}"
	keep_plugins=true
fi
