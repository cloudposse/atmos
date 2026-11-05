package verification

import "github.com/cloudposse/atmos/pkg/perf"

// DigestFile computes an artifact checksum independently of upstream sidecar
// availability, allowing a project lockfile to remain an integrity constraint.
func DigestFile(path, algorithm string) (string, error) {
	defer perf.Track(nil, "verification.DigestFile")()

	return digestFile(path, algorithm)
}
