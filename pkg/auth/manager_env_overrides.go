package auth

import (
	"fmt"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// initCliConfigFn, createAuthManager, and setEnvWithRestoreFn are
// package-level hooks used by CreateAndAuthenticateManagerWithEnvOverrides.
// They are variables (not constants) only so tests can substitute fakes
// without needing real atmos config fixtures or simulating os.Setenv
// failures. Production code always uses the real defaults.
var (
	initCliConfigFn     = cfg.InitCliConfig
	createAuthManager   = CreateAndAuthenticateManagerWithAtmosConfig
	setEnvWithRestoreFn = env.SetWithRestore
)

// managerEnvOverridesMu serializes calls to
// CreateAndAuthenticateManagerWithEnvOverrides. It is required for
// correctness, not just hardening: the function mutates os.Environ() and
// then reads it back via cfg.InitCliConfig (Viper). Without this lock,
// concurrent invocations would race — goroutine A's InitCliConfig could
// observe goroutine B's ATMOS_* overrides and load the wrong profile and
// identities for A.
//
// The lock lives at this composition layer (rather than inside
// env.SetWithRestore) because the unsafe pattern is the *read-after-write*
// across multiple steps. The env.SetWithRestore primitive on its own is
// fine for callers that don't subsequently read os.Environ().
var managerEnvOverridesMu sync.Mutex

// CreateAndAuthenticateManagerWithEnvOverrides builds and authenticates an
// AuthManager "as if" the given ATMOS_* environment variables were set on
// the parent process.
//
// It is the composition of three concerns a caller would otherwise orchestrate
// manually:
//
//  1. env.SetWithRestore — temporarily applies ATMOS_* variables from the
//     map to os.Environ() and returns a restore closure. The generic
//     save/set/restore primitive lives in pkg/env.
//  2. cfg.InitCliConfig — re-loads the atmos configuration so that
//     ATMOS_PROFILE / ATMOS_CLI_CONFIG_PATH / ATMOS_BASE_PATH from step 1
//     influence which profile and identities are discovered.
//  3. CreateAndAuthenticateManagerWithAtmosConfig — constructs and
//     authenticates the manager against the freshly-loaded config.
//
// Only keys with the ATMOS_* prefix in envOverrides are applied — other keys
// are silently ignored. This primitive is intentionally scoped to atmos
// config/identity resolution; callers that need to mutate arbitrary env
// variables should use env.SetWithRestore directly.
//
// The env overrides are reverted before this function returns. The returned
// manager's identity map and credentials are already populated during
// construction, so the restoration does not affect subsequent use.
//
// Returns (nil, nil) under the same conditions as
// CreateAndAuthenticateManagerWithAtmosConfig: authentication disabled, no
// identity specified, or no default identity configured. Callers that require
// a non-nil manager must check explicitly.
//
// Goroutine-safe: a package-level mutex (managerEnvOverridesMu) serializes
// concurrent calls to prevent races between the os.Environ() write and the
// subsequent cfg.InitCliConfig read. Concurrent callers will block rather
// than observe each other's overrides.
func CreateAndAuthenticateManagerWithEnvOverrides(envOverrides map[string]string) (AuthManager, error) {
	defer perf.Track(nil, "auth.CreateAndAuthenticateManagerWithEnvOverrides")()

	// Serialize: see managerEnvOverridesMu doc for why this is correctness,
	// not just hardening.
	managerEnvOverridesMu.Lock()
	defer managerEnvOverridesMu.Unlock()

	atmosOnly := filterAtmosOverrides(envOverrides)

	restore, err := setEnvWithRestoreFn(atmosOnly)
	if err != nil {
		if restore != nil {
			restore()
		}
		return nil, fmt.Errorf("%w: failed to apply ATMOS_* env overrides: %w", errUtils.ErrAuthManager, err)
	}
	defer restore()

	loadedConfig, err := initCliConfigFn(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to initialize CLI config for scoped auth manager: %w", errUtils.ErrAuthManager, err)
	}

	return createAuthManager(
		"", &loadedConfig.Auth, cfg.IdentityFlagSelectValue, &loadedConfig,
	)
}

// filterAtmosOverrides returns a new map containing only the entries whose
// key has the canonical Atmos env-var prefix (cfg.AtmosEnvVarPrefix). Nil
// or empty input yields nil.
func filterAtmosOverrides(overrides map[string]string) map[string]string {
	if len(overrides) == 0 {
		return nil
	}
	out := make(map[string]string, len(overrides))
	for k, v := range overrides {
		if strings.HasPrefix(k, cfg.AtmosEnvVarPrefix) {
			out[k] = v
		}
	}
	return out
}
