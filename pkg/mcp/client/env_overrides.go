package client

import (
	"os"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// AtmosEnvPrefix is the prefix used to identify Atmos environment variables.
const AtmosEnvPrefix = "ATMOS_"

// ApplyAtmosEnvOverrides applies ATMOS_* environment variables from the given
// map to the parent process environment, returning a restore function that
// reverts the changes.
//
// This is used during MCP server auth setup so that ATMOS_PROFILE,
// ATMOS_CLI_CONFIG_PATH, ATMOS_BASE_PATH, and other ATMOS_* variables defined
// in a server's `env:` block influence atmos config loading and identity
// resolution — even though the auth setup runs in the parent process before
// the subprocess is spawned.
//
// The returned restore function is idempotent. Callers should `defer restore()`.
//
// Note: this function is intentionally NOT goroutine-safe. Callers must
// serialize calls when multiple servers are being authenticated concurrently.
// In current usage MCP servers are started sequentially, so this is fine.
func ApplyAtmosEnvOverrides(env map[string]string) func() {
	defer perf.Track(nil, "mcp.client.ApplyAtmosEnvOverrides")()

	if len(env) == 0 {
		return func() {}
	}

	// Collect ATMOS_* keys in deterministic order so save/restore is stable.
	keys := make([]string, 0, len(env))
	for k := range env {
		if strings.HasPrefix(k, AtmosEnvPrefix) {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return func() {}
	}
	sort.Strings(keys)

	type savedEntry struct {
		key   string
		value string
		had   bool
	}
	saved := make([]savedEntry, 0, len(keys))

	for _, k := range keys {
		old, had := os.LookupEnv(k)
		saved = append(saved, savedEntry{key: k, value: old, had: had})
		_ = os.Setenv(k, env[k])
	}

	restored := false
	return func() {
		if restored {
			return
		}
		restored = true
		for _, s := range saved {
			if s.had {
				_ = os.Setenv(s.key, s.value)
			} else {
				_ = os.Unsetenv(s.key)
			}
		}
	}
}
