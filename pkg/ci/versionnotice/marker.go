package versionnotice

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// jobMarker returns an identity-scoped marker shared by independent invocations.
// GitHub cleans RUNNER_TEMP between jobs, including separate matrix workers.
// Generic CI without job identity falls back to the frequency cache and sentinel.
const markerPermissions = 0o600

func jobMarker() string {
	directory, identity := jobIdentity()
	if directory == "" || identity == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(identity))
	return filepath.Join(directory, "atmos-ci-version-"+hex.EncodeToString(digest[:])+".shown")
}

func jobIdentity() (string, string) {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		if os.Getenv("GITHUB_RUN_ID") == "" || os.Getenv("GITHUB_JOB") == "" {
			return "", ""
		}
		identity := strings.Join([]string{"github", os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID"), os.Getenv("GITHUB_RUN_ATTEMPT"), os.Getenv("GITHUB_JOB")}, "\x00")
		return os.Getenv("RUNNER_TEMP"), identity
	}
	for _, key := range []string{"CI_JOB_ID", "BUILDKITE_JOB_ID", "CIRCLE_WORKFLOW_JOB_ID", "BUILD_TAG", "ATMOS_CI_RUN_ID"} {
		if value := os.Getenv(key); value != "" {
			identity := strings.Join([]string{key, value, os.Getenv("CI_SERVER_URL"), os.Getenv("CI_PROJECT_PATH"), os.Getenv("ATMOS_CI_REPOSITORY")}, "\x00")
			return os.TempDir(), identity
		}
	}
	return "", ""
}

func alreadyShown(marker string) bool {
	if os.Getenv(sentinel) != "" {
		return true
	}
	if marker == "" {
		return false
	}
	_, err := os.Stat(marker)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	debugError(err)
	return true
}

// claimMarker uses exclusive creation so concurrent commands cannot both emit.
// There is no lock retry or wait on the command completion path.
func claimMarker(path string) bool {
	if path == "" {
		return true
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, markerPermissions)
	if err != nil {
		if !os.IsExist(err) {
			debugError(err)
		}
		return false
	}
	if err := file.Close(); err != nil {
		debugError(err)
		return false
	}
	return true
}
