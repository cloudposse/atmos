// Package startup prints a short status banner when Atmos runs inside a
// detected CI provider, reporting Native CI mode, Atmos Pro, and legacy
// GitHub Action usage. It composes pkg/ci and pkg/ci/providers/github, which
// import each other and cannot depend on one another for this.
package startup

import (
	"runtime"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/version"
)

// PrintStartupStatus prints the Atmos version, Native CI status, Atmos Pro
// status, and (when detected) a legacy-action warning. It is a no-op unless
// Atmos detects it is actually running inside a CI provider.
func PrintStartupStatus(atmosConfig *schema.AtmosConfiguration) {
	defer perf.Track(atmosConfig, "startup.PrintStartupStatus")()

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
