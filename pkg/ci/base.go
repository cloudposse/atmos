package ci

import (
	"encoding/hex"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// MinSHALength is the minimum length of a short git commit SHA.
	minSHALength = 7
	// MaxSHALength is the maximum length of a full git commit SHA.
	maxSHALength = 40
)

// IsCommitSHA returns true if s looks like a git commit SHA (7-40 hex chars).
func IsCommitSHA(s string) bool {
	defer perf.Track(nil, "ci.IsCommitSHA")()

	if len(s) < minSHALength || len(s) > maxSHALength {
		return false
	}
	_, err := hex.DecodeString(s)
	// hex.DecodeString requires even-length strings, so handle odd-length manually.
	if err != nil && len(s)%2 != 0 {
		_, err = hex.DecodeString("0" + s)
	}
	return err == nil
}
