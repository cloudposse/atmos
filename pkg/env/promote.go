package env

import (
	"os"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// AtmosEnvPrefix is the prefix for environment variables that configure Atmos itself
// (e.g. ATMOS_PROFILE, ATMOS_BASE_PATH).
const AtmosEnvPrefix = "ATMOS_"

// LookupFunc resolves an environment variable, returning its value and whether it is set.
type LookupFunc func(key string) (string, bool)

// SetFunc sets an environment variable.
type SetFunc func(key, value string) error

// PromoteAtmosEnv promotes ATMOS_-prefixed keys from the given map into the process
// environment so they can configure Atmos itself. This lets a project pin ATMOS_PROFILE
// (and other ATMOS_* settings) by including a dotenv file in the `env:` section of
// atmos.yaml (`env: !include .env`).
//
// Real environment variables always take precedence: a key already present in the
// environment is never overwritten. Returns the names of the keys that were newly set,
// in no particular order (map iteration).
func PromoteAtmosEnv(envMap map[string]string) []string {
	defer perf.Track(nil, "env.PromoteAtmosEnv")()

	return PromoteAtmosEnvWith(envMap, os.LookupEnv, os.Setenv)
}

// PromoteAtmosEnvWith is the dependency-injected core of PromoteAtmosEnv. It uses the
// provided lookup and setter so the promotion policy (prefix filter, real-env-wins) can
// be unit-tested without touching the real process environment.
func PromoteAtmosEnvWith(envMap map[string]string, lookup LookupFunc, set SetFunc) []string {
	defer perf.Track(nil, "env.PromoteAtmosEnvWith")()

	if len(envMap) == 0 {
		return nil
	}

	var promoted []string
	for key, value := range envMap {
		// Only promote variables that configure Atmos itself.
		if !strings.HasPrefix(key, AtmosEnvPrefix) {
			continue
		}
		// Real environment variables win: never overwrite an already-set variable.
		if _, exists := lookup(key); exists {
			continue
		}
		if err := set(key, value); err != nil {
			log.Trace("Failed to promote ATMOS env var from config", "key", key, "error", err)
			continue
		}
		promoted = append(promoted, key)
	}
	return promoted
}
