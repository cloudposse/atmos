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
// Explicit child force-color and opt-out settings are preserved. Parent global opt-outs still disable colors.
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
	// The actual destination matters: a terminal on stdout does not justify adding color
	// to a redirected stderr, and an in-memory capture is not a terminal.
	// CI's automatic UI color support is not an explicit force-color preference: turning
	// it into one would also highlight machine-readable stdout redirected inside the shell.
	forwarding := *config
	forwarding.EnvCI = false
	if !hasChildColorPreference(result) && visible && forwarding.ShouldUseColor(isTTY) {
		result = append(result, "ATMOS_FORCE_COLOR=true")
	}
	return childRedirectedColorEnv(result, visible && isTTY)
}

// childRedirectedColorEnv prevents CI auto-color from leaking into captured or redirected output.
func childRedirectedColorEnv(environ []string, terminalDestination bool) []string {
	if ci, _ := childEnvValue(environ, "CI"); ci != "" && !terminalDestination {
		// Keep CI available to scripts, but prevent it from enabling automatic color
		// in a child whose output is redirected. Explicit force-color still wins over
		// CLICOLOR=0, unlike NO_COLOR, which would suppress intentional forced color.
		return append(environ, "CLICOLOR=0")
	}
	return environ
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
