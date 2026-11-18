#!/usr/bin/env bash
# Run the scanner unchanged while recording resource pressure for slow CI jobs.
set -u

monitor() {
	local sleeper=""
	trap 'kill "$sleeper" 2>/dev/null || :; exit 0' TERM INT
	while :; do
		{
			echo "--- govulncheck resources $(date -u +%FT%TZ) ---"
			free -m || :
			for resource in cpu memory io; do
				if [[ -r "/proc/pressure/$resource" ]]; then
					echo "$resource pressure:"
					cat "/proc/pressure/$resource" || :
				fi
			done
			# Show the largest processes and the scanner even if it is not among them.
			ps -eo pid,ppid,stat,pcpu,rss,vsz,comm --sort=-rss | head -n 26 || :
			for pid in $(pgrep -x govulncheck || :); do
				echo "govulncheck pid $pid:"
				awk '/^(State|VmRSS|VmSwap):/ { print }' "/proc/$pid/status" 2>/dev/null || :
			done
		} >&2
		sleep 60 &
		sleeper=$!
		wait "$sleeper" || :
		sleeper=""
	done
}

monitor &
monitor_pid=$!
cleanup() {
	kill "$monitor_pid" 2>/dev/null || :
	wait "$monitor_pid" 2>/dev/null || :
}
trap cleanup EXIT

"$@"
