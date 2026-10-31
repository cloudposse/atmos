package step

import (
	"github.com/cloudposse/atmos/pkg/artifact/report"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// UseArtifactReport makes the artifact steps run with these variables report to CI, and returns the
// function that writes the report. The execution boundary that owns the variables (a workflow, a
// custom command, a standalone script, or a hook) calls it once and defers the returned function:
//
//	defer vars.UseArtifactReport(config)()
//
// The step handler records every attempt of every artifact operation into the report session; the
// returned function writes one summary and one set of CI outputs, after retries, in scheduled order,
// even when the run failed. A reporting failure is logged and never changes the run's result.
//
// When every ci.* reporting surface is disabled, nothing is installed and the returned function
// does nothing, so the gates cost nothing. When a report session is already installed, the
// boundary that installed it owns the report, so a nested boundary reuses it instead of rendering
// a second copy.
func (v *Variables) UseArtifactReport(config *schema.AtmosConfiguration, options ...report.Option) (flush func()) {
	defer perf.Track(config, "step.Variables.UseArtifactReport")()

	session := v.artifactSession()
	session.mu.Lock()
	_, owned := session.collector.(*report.Session)
	session.mu.Unlock()
	if owned {
		return func() {}
	}

	reporting := report.NewSession(config, options...)
	if !reporting.Enabled() {
		return func() {}
	}
	v.SetArtifactCollector(reporting)
	return reporting.FlushAndLog
}
