package policy

import (
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// DetectAgents captures only agent names, never session IDs or arbitrary environment values.
// These inherited markers are advisory: a caller can remove or forge them.
func DetectAgents() []string {
	defer perf.Track(nil, "policy.DetectAgents")()

	agents := []string{}
	settings := viper.New()
	settings.MustBindEnv("claude", "CLAUDECODE")
	settings.MustBindEnv("codex", "CODEX_THREAD_ID")
	if settings.GetString("claude") == "1" {
		agents = append(agents, "claude-code")
	}
	if settings.GetString("codex") != "" {
		agents = append(agents, "codex")
	}
	return agents
}
