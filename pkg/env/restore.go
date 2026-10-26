// Package env provides utilities for working with environment variables.
package env

import (
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

// savedEnvEntry stores the original state of an environment variable so it
// can be restored exactly — including the "was not set" case.
type savedEnvEntry struct {
	value   string
	existed bool
}

// setenvFn is a package-level hook for os.Setenv. Tests can override it to
// simulate setenv failures (which are practically impossible to trigger via
// the real os.Setenv on POSIX systems but the error path still exists in
// the standard library and must be covered).
var setenvFn = os.Setenv

// SetWithRestore sets the given environment variables on the parent process
// and returns a cleanup closure that reverts every variable to its original
// state (including unsetting variables that did not exist before the call).
//
// The returned closure is idempotent: calling it more than once is a no-op
// after the first invocation. Callers should `defer` it immediately after
// checking the error.
//
// On partial failure (os.Setenv returns an error mid-way), the cleanup
// closure still correctly restores the variables that were already set
// before the failure, so callers can rely on `defer cleanup()` for safety
// even in error paths.
//
// NOT goroutine-safe. This function mutates process-global state
// (os.Environ()). Callers must serialize concurrent invocations, or use a
// higher-level mechanism (e.g., subprocess env) when concurrency is needed.
//
// This is the foundational save/set/restore primitive. Other packages that
// currently have their own variants (e.g. internal/exec.setEnvVarsWithRestore,
// pkg/auth/cloud/gcp.PreserveEnvironment/RestoreEnvironment,
// pkg/telemetry.PreserveCIEnvVars/RestoreCIEnvVars,
// pkg/auth/identities/aws.setupAWSEnv) should migrate to this over time to
// eliminate duplication.
func SetWithRestore(vars map[string]string) (func(), error) {
	defer perf.Track(nil, "env.SetWithRestore")()

	if len(vars) == 0 {
		return func() {}, nil
	}

	saved := make(map[string]savedEnvEntry, len(vars))
	for k := range vars {
		if original, existed := os.LookupEnv(k); existed {
			saved[k] = savedEnvEntry{value: original, existed: true}
		} else {
			saved[k] = savedEnvEntry{existed: false}
		}
	}

	restored := false
	cleanup := func() {
		if restored {
			return
		}
		restored = true
		for k, original := range saved {
			if original.existed {
				_ = os.Setenv(k, original.value)
			} else {
				_ = os.Unsetenv(k)
			}
		}
	}

	for k, v := range vars {
		if err := setenvFn(k, v); err != nil {
			// Cleanup closure already knows about every key we attempted to
			// save, so defer-cleanup from the caller will correctly restore.
			return cleanup, err
		}
	}

	return cleanup, nil
}
