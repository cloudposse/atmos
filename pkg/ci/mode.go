package ci

import (
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/viperguard"
)

// WorkerModeEnv carries an orchestrator's resolved CI mode into child Atmos
// processes without removing CI provider metadata from their environment.
const WorkerModeEnv = "ATMOS_CI_WORKER_MODE"

const modeFlag = "ci"

// ResolveMode honors the parent invocation before automatic CI detection.
// Callers must handle an explicitly changed command flag before calling this.
func ResolveMode(configured bool) bool {
	defer perf.Track(nil, "ci.ResolveMode")()

	if value, err := strconv.ParseBool(os.Getenv(WorkerModeEnv)); err == nil {
		return value
	}
	return configured || IsCI()
}

// ModeEnabled reports whether CI mode is requested by a flag, configuration,
// or an active CI provider.
func ModeEnabled(cmd *cobra.Command) bool {
	defer perf.Track(nil, "ci.ModeEnabled")()

	if cmd != nil {
		if flag := cmd.Flags().Lookup(modeFlag); flag != nil && flag.Changed {
			value, _ := cmd.Flags().GetBool(modeFlag)
			return value
		}
		if flag := cmd.InheritedFlags().Lookup(modeFlag); flag != nil && flag.Changed {
			value, _ := cmd.InheritedFlags().GetBool(modeFlag)
			return value
		}
	}
	return ResolveMode(viperguard.GetBool(modeFlag))
}

// Enabled reports whether the ci.enabled master switch is set.
func Enabled(atmosConfig *schema.AtmosConfiguration) bool {
	defer perf.Track(atmosConfig, "ci.Enabled")()

	return atmosConfig != nil && atmosConfig.CI.Enabled
}

// AnnotationsEnabled reports whether CI annotations are enabled. They default
// to enabled whenever ci.enabled is set.
func AnnotationsEnabled(atmosConfig *schema.AtmosConfiguration) bool {
	defer perf.Track(atmosConfig, "ci.AnnotationsEnabled")()

	if !Enabled(atmosConfig) {
		return false
	}
	enabled := atmosConfig.CI.Annotations.Enabled
	return enabled == nil || *enabled
}

// ResultsEnabled reports whether SARIF uploads are enabled. Uploads are opt-in
// because they have external side effects and provider-specific requirements.
func ResultsEnabled(atmosConfig *schema.AtmosConfiguration) bool {
	defer perf.Track(atmosConfig, "ci.ResultsEnabled")()

	if !Enabled(atmosConfig) {
		return false
	}
	enabled := atmosConfig.CI.Results.Enabled
	return enabled != nil && *enabled
}

// SummaryEnabled reports whether the job step summary is enabled. It defaults
// to enabled whenever ci.enabled is set.
func SummaryEnabled(atmosConfig *schema.AtmosConfiguration) bool {
	defer perf.Track(atmosConfig, "ci.SummaryEnabled")()

	if !Enabled(atmosConfig) {
		return false
	}
	enabled := atmosConfig.CI.Summary.Enabled
	return enabled == nil || *enabled
}

// OutputEnabled reports whether CI output variables are enabled. They default
// to enabled whenever ci.enabled is set.
func OutputEnabled(atmosConfig *schema.AtmosConfiguration) bool {
	defer perf.Track(atmosConfig, "ci.OutputEnabled")()

	if !Enabled(atmosConfig) {
		return false
	}
	enabled := atmosConfig.CI.Output.Enabled
	return enabled == nil || *enabled
}

// ChecksEnabled reports whether CI status checks are enabled. Checks are
// opt-in because they have external side effects and need provider permissions.
func ChecksEnabled(atmosConfig *schema.AtmosConfiguration) bool {
	defer perf.Track(atmosConfig, "ci.ChecksEnabled")()

	if !Enabled(atmosConfig) {
		return false
	}
	enabled := atmosConfig.CI.Checks.Enabled
	return enabled != nil && *enabled
}

// CommentsEnabled reports whether PR/MR comments are enabled. Comments are
// opt-in because they have external side effects and need provider permissions.
func CommentsEnabled(atmosConfig *schema.AtmosConfiguration) bool {
	defer perf.Track(atmosConfig, "ci.CommentsEnabled")()

	if !Enabled(atmosConfig) {
		return false
	}
	enabled := atmosConfig.CI.Comments.Enabled
	return enabled != nil && *enabled
}
