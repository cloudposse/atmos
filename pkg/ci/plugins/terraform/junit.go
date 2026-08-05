package terraform

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/junit"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// junitFilePerm is the mode for the generated JUnit report (world-readable so a
// downstream upload/report step can pick it up).
const junitFilePerm = 0o644

// toJUnit converts parsed terraform test results into a JUnit report: one suite
// per `.tftest.hcl` file, one case per run, with the failing assertion's
// message and file:line carried through for annotations/reporters.
func toJUnit(data *plugin.TerraformTestOutputData, component string) junit.Report {
	defer perf.Track(nil, "terraform.toJUnit")()

	if data == nil {
		return junit.Report{Name: component}
	}

	suites := map[string]*junit.Suite{}
	var order []string
	for _, run := range data.Runs {
		file := run.File
		if file == "" {
			file = component
		}
		suite, ok := suites[file]
		if !ok {
			suite = &junit.Suite{Name: file}
			suites[file] = suite
			order = append(order, file)
		}

		testCase := junit.Case{Name: run.Name, Classname: component, File: run.File, Line: run.Line, Time: run.Duration}
		detail := &junit.Detail{Message: run.Error, Text: run.Error}
		switch run.Status {
		case "error":
			testCase.Error = detail
		case "fail":
			testCase.Failure = detail
		case "skip":
			testCase.Skipped = &junit.Detail{}
		}
		suite.Cases = append(suite.Cases, testCase)
	}

	report := junit.Report{Name: component}
	for _, file := range order {
		report.Suites = append(report.Suites, *suites[file])
	}
	report.Aggregate()
	return report
}

// writeJUnitReport formats the test results as JUnit XML, writes them next to
// the component's terraform working dir (falling back to the CWD), and exposes
// the path via the CI output writer (`$GITHUB_OUTPUT` `junit_report`).
func (p *Plugin) writeJUnitReport(ctx *plugin.HookContext, result *plugin.OutputResult) {
	defer perf.Track(ctx.Config, "terraform.Plugin.writeJUnitReport")()

	data, ok := result.Data.(*plugin.TerraformTestOutputData)
	if !ok || data == nil {
		return
	}

	report := toJUnit(data, ctx.Info.ComponentFromArg)
	xml, err := junit.Format(&report)
	if err != nil {
		log.Warn("CI JUnit report failed", "error", err)
		return
	}

	path := filepath.Join(junitReportDir(p, ctx), sanitizeFilename(ctx.Info.ComponentFromArg)+".junit.xml")
	if err := os.WriteFile(path, xml, junitFilePerm); err != nil {
		log.Warn("CI JUnit report failed", "error", err)
		return
	}

	log.Debug("Wrote JUnit report", "path", path, "component", ctx.Info.ComponentFromArg)
	if writer := ctx.Provider.OutputWriter(); writer != nil {
		if err := writer.WriteOutput("junit_report", path); err != nil {
			log.Warn("Failed to write CI output", "key", "junit_report", "error", err)
		}
	}
}

// junitReportDir resolves the component's terraform working directory (where the
// planfile lives), falling back to the current working directory.
func junitReportDir(p *Plugin, ctx *plugin.HookContext) string {
	if planPath := p.resolveArtifactPath(ctx); planPath != "" {
		return filepath.Dir(planPath)
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

// emitTestAnnotations emits one inline CI annotation per failing/errored run
// (GitHub: `::error file=…,line=…::message`). No-op when the provider lacks
// annotation support or there are no failures.
func (p *Plugin) emitTestAnnotations(ctx *plugin.HookContext, result *plugin.OutputResult) {
	defer perf.Track(ctx.Config, "terraform.Plugin.emitTestAnnotations")()

	data, ok := result.Data.(*plugin.TerraformTestOutputData)
	if !ok || data == nil {
		return
	}
	annotator, ok := ctx.Provider.(provider.Annotator)
	if !ok {
		return
	}

	var annotations []provider.Annotation
	for i := range data.Runs {
		run := &data.Runs[i]
		if !shouldAnnotateTestRun(run) {
			continue
		}
		annotations = append(annotations, annotationForTestRun(run))
	}
	if len(annotations) == 0 {
		return
	}
	if err := annotator.Annotate(annotations); err != nil {
		log.Warn("CI annotations failed", "error", err)
	}
}

func shouldAnnotateTestRun(run *plugin.TerraformTestRun) bool {
	if run.Status != "fail" && run.Status != "error" {
		return false
	}
	return run.File != "" && run.Line > 0
}

func annotationForTestRun(run *plugin.TerraformTestRun) provider.Annotation {
	return provider.Annotation{
		Path:      run.File,
		StartLine: run.Line,
		Level:     provider.AnnotationError,
		Title:     "terraform test: " + run.Name,
		Message:   run.Error,
	}
}

// sanitizeFilename makes a component name safe for a filename (slashes → dashes).
func sanitizeFilename(name string) string {
	replaced := strings.NewReplacer("/", "-", "\\", "-", " ", "-").Replace(name)
	if replaced == "" {
		return "component"
	}
	return replaced
}
