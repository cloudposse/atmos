// Package startup prints a short status banner when Atmos runs inside a
// detected CI provider, reporting Native CI mode, Atmos Pro, and legacy
// GitHub Action usage. It composes pkg/ci and pkg/ci/providers/github, which
// pkg/ci cannot import directly: pkg/ci/providers/github already imports
// pkg/ci to register itself, so the reverse import would create a cycle.
package startup

import (
	"os"
	"runtime"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/version"
)

// noticesShownEnvVar marks that this process tree has already printed its
// startup notices, so atmos child processes spawned afterward (workflow and
// custom-command steps re-exec the atmos binary once per component) can skip
// reprinting them. Mirrors the loop-guard sentinel pattern in
// pkg/reexec/depth.go's ATMOS_REEXEC_DEPTH.
const noticesShownEnvVar = "ATMOS_STARTUP_NOTICES_SHOWN"

// AlreadyShown reports whether a top-level atmos invocation has already
// printed its startup notices earlier in this process tree.
func AlreadyShown() bool {
	defer perf.Track(nil, "startup.AlreadyShown")()

	return os.Getenv(noticesShownEnvVar) != ""
}

// MarkShown records that this process tree has already handled startup
// notices. Any atmos child process spawned afterward inherits this via the
// OS environment and skips startup banners and warn-mode experimental notices.
// Daily experimental warnings are tracked separately per feature in the cache.
func MarkShown() {
	defer perf.Track(nil, "startup.MarkShown")()

	_ = os.Setenv(noticesShownEnvVar, "1")
}

// PrintStartupStatus prints the Atmos version, Native CI status, Atmos Pro
// status, and (when detected) a legacy-action warning. It is a no-op unless
// Atmos detects it is actually running inside a CI provider.
func PrintStartupStatus(atmosConfig *schema.AtmosConfiguration) {
	defer perf.Track(atmosConfig, "startup.PrintStartupStatus")()

	if AlreadyShown() {
		return
	}

	if !ci.IsCI() {
		return
	}

	printStatusLines(atmosConfig)
}

// printStatusLines prints the status lines unconditionally, without checking
// whether Atmos is actually running inside a CI provider. Split out from
// PrintStartupStatus so the line-selection logic is testable without having
// to mock CI-provider detection.
func printStatusLines(atmosConfig *schema.AtmosConfiguration) {
	defer perf.Track(atmosConfig, "startup.printStatusLines")()

	ui.Infof("Atmos version %s %s/%s", version.Version, runtime.GOOS, runtime.GOARCH)

	if ci.Enabled(atmosConfig) {
		ui.Successf("Atmos CI is enabled; learn more at https://atmos.tools/ci")
	} else {
		ui.Errorf("Atmos CI is disabled; learn more at https://atmos.tools/ci")
	}

	if atmosConfig != nil && atmosConfig.Settings.Pro.WorkspaceID != "" {
		ui.Successf("Atmos Pro is enabled; learn more at https://atmos.tools/pro")
	} else {
		ui.Errorf("Atmos Pro is disabled; learn more at https://atmos.tools/pro")
	}

	if repo, ok := github.LegacyActionRepo(); ok {
		ui.Warningf("Detected legacy action %s; migrate to Native CI for better performance — learn more at https://atmos.tools/ci", repo)
	}
}
