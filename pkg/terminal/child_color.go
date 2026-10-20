package terminal

import (
	"io"
	"os"
	"slices"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ChildColorEnv preserves color preferences when a child's output is forwarded through a pipe
// to destination. It conveys rendering only; stdin and TTY/interactivity remain untouched.
// Explicit child color settings are preserved. Parent global opt-outs still disable colors.
func ChildColorEnv(environ []string, destination io.Writer) []string {
	defer perf.Track(nil, "terminal.ChildColorEnv")()
	return childColorEnv(buildConfig(), environ, destination != nil && IsTTYWriter(destination), destination != nil)
}

func childColorEnv(config *Config, environ []string, isTTY, visible bool) []string {
	result := slices.Clone(environ)

	if config.NoColor || config.EnvNoColor || (config.AtmosConfig.Settings.Terminal.NoColor && !config.ShouldUseColor(isTTY)) {
		// Append wins when callers merge step environments with the process environment.
		return append(result, "NO_COLOR=1")
	}
	if hasChildColorPreference(result) {
		return result
	}

	// The actual destination matters: a terminal on stdout does not justify adding color
	// to a redirected stderr, and an in-memory capture is not a terminal.
	if visible && config.ShouldUseColor(isTTY) {
		result = append(result, "ATMOS_FORCE_COLOR=true")
	}
	return result
}

func childEnvValue(environ []string, key string) (string, bool) {
	for i := len(environ) - 1; i >= 0; i-- {
		if k, v, ok := strings.Cut(environ[i], "="); ok && k == key {
			return v, true
		}
	}
	return os.LookupEnv(key)
}

func hasChildColorPreference(environ []string) bool {
	if value, _ := childEnvValue(environ, "NO_COLOR"); value != "" {
		return true
	}
	if value, _ := childEnvValue(environ, "CLICOLOR"); value == "0" {
		return true
	}
	for _, key := range []string{"ATMOS_FORCE_COLOR", "FORCE_COLOR", "CLICOLOR_FORCE"} {
		if value, set := childEnvValue(environ, key); set && value != "" {
			return true
		}
	}
	return false
}
