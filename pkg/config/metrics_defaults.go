package config

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// DisableMetricsSummaryByDefault turns the local resource-usage summary off unless the user
// explicitly set settings.metrics.enabled. Standalone scripts and Git hooks use this: a script
// is the user's own CLI tool and a hook runs inside git commit or git push, so the final
// "Total for this invocation" line must be opt-in there. An explicit true or false is kept;
// every other Atmos command keeps the default of enabled.
func DisableMetricsSummaryByDefault(config *schema.AtmosConfiguration) {
	defer perf.Track(nil, "config.DisableMetricsSummaryByDefault")()

	if config != nil && config.Settings.Metrics.Enabled == nil {
		disabled := false
		config.Settings.Metrics.Enabled = &disabled
	}
}
