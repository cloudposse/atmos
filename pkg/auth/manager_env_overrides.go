package auth

import (
	"strings"

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
// NOT goroutine-safe: env.SetWithRestore mutates process-global state.
// Current Atmos callers (sequential MCP server startup, single-auth-manager
// commands) serialize access, so this is fine.
func CreateAndAuthenticateManagerWithEnvOverrides(envOverrides map[string]string) (AuthManager, error) {
	defer perf.Track(nil, "auth.CreateAndAuthenticateManagerWithEnvOverrides")()

	atmosOnly := filterAtmosOverrides(envOverrides)

	restore, err := setEnvWithRestoreFn(atmosOnly)
	if err != nil {
		if restore != nil {
			restore()
		}
		return nil, err
	}
	defer restore()

	loadedConfig, err := initCliConfigFn(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, err
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
