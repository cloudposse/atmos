package conformance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	"github.com/cloudposse/atmos/pkg/plugin/bridge"
	"github.com/cloudposse/atmos/pkg/plugin/extension"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginplatform "github.com/cloudposse/atmos/pkg/plugin/platform"
	pluginpolicy "github.com/cloudposse/atmos/pkg/plugin/policy"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/plugin/runtime/native"
	"github.com/cloudposse/atmos/pkg/runner/step"
	"github.com/cloudposse/atmos/pkg/schema"
	_ "github.com/cloudposse/atmos/pkg/workflow" // Registers the shared test scheduler and reporter.
)

const suiteTimeoutMinutes = 5

// Options controls fixture selection and the shared assertion output streams.
type Options struct {
	Run         string
	Integration bool
	// Build compiles the selected native target before conformance.
	Build bool
	// Platform asserts a declared target executable on this host.
	Platform string
	// CoverProfile collects Go module unit coverage at a package-relative path.
	CoverProfile string
	// SourceRunner overrides source build/coverage operations for another authoring runtime.
	SourceRunner SourceRunner
	// Project is the project directory that policy input.path is relative to. Empty means the
	// current directory.
	Project        string
	Stdout, Stderr io.Writer
	// Registry supplies extension contracts and their local conformance hooks.
	// A nil registry uses the contracts registered by Atmos bootstrap.
	Registry *extension.Registry
	// OnResult receives each completed check for live human-readable progress.
	// Result names remain stable for machine reports and --run selectors.
	OnResult func(Result)
}

// Result reports case names and outcomes. A failed fixture also carries Detail, the expected
// and observed values that differ, truncated and passed through secret masking. Raw plugin
// transport errors may contain credentials and never appear here unmasked.
type Result struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail,omitempty"`
}

// Report keeps mock conformance separate from explicitly requested live tests.
type Report struct {
	Mode     string   `json:"mode"`
	Results  []Result `json:"results"`
	onResult func(Result)
}

func (report *Report) add(result Result) {
	report.Results = append(report.Results, result)
	if report.onResult != nil {
		report.onResult(result)
	}
}

type suite struct {
	directory, executable string
	expected              *pluginv1.DescribeResponse
	bindings              []extension.Binding
	registry              *extension.Registry
}

// Run validates the package, optionally builds and collects unit coverage, then
// launches the selected native executable and runs authored conformance fixtures.
//
//nolint:gocritic // Keep the public options API value-based; internal helpers share its address.
func Run(ctx context.Context, directory string, options Options) (*Report, error) {
	defer perf.Track(nil, "conformance.Run")()
	if err := validateSourceOptions(&options); err != nil {
		return nil, err
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	// Testing local source is governed by `on: plugin` rules for operation "test" only, never by
	// plugins.policy.require_signature: authors must be able to test an unsigned plugin before they
	// can sign it. The tested directory is exposed as input.path.
	project := options.Project
	if project == "" {
		project = "."
	}
	policyPath := pluginpolicy.WithPath(pluginpolicy.DevelopmentPath(project, directory))
	if err := pluginpolicy.Source(ctx, pluginpolicy.OperationTest, "", policyPath); err != nil {
		return nil, err
	}
	config, err := load(directory)
	if err != nil {
		return nil, err
	}
	suite, err := prepare(directory, options.Registry)
	if err != nil {
		return nil, err
	}
	definition, err := manifest.Load(directory)
	if err != nil {
		return nil, err
	}
	if err := pluginpolicy.Package(ctx, pluginpolicy.OperationTest, "", definition, pluginpolicy.WithExtensions(suite.registry), policyPath); err != nil {
		return nil, err
	}
	report := &Report{Mode: "mock", Results: []Result{}, onResult: options.OnResult}
	if options.Integration {
		report.Mode = "integration"
		if len(config.Integration) == 0 {
			return report, errUtils.Build(errUtils.ErrPluginInvalidConfig).
				WithExplanation("`--integration` needs explicit live steps, but plugin.tests.yaml has no `integration` entries.").
				WithHint("Add steps under `integration:` in plugin.tests.yaml, or run `atmos plugin test` without `--integration`.").Err()
		}
	}
	if err := prepareSource(ctx, directory, definition, &options, suite); err != nil {
		return report, err
	}
	policy := api.ExecutionPolicy{Timeout: suiteTimeoutMinutes * time.Minute}
	err = policy.Execute(ctx, func(ctx context.Context) error { return suite.run(ctx, config, &options, report) }, nil)
	return report, safeFailure(err)
}

func prepare(directory string, registry *extension.Registry) (*suite, error) {
	definition, err := manifest.Load(directory)
	if err != nil {
		return nil, err
	}
	if registry == nil {
		registry, err = bootstrap.Descriptors()
		if err != nil {
			return nil, err
		}
	}
	bindings, err := registry.Validate(directory, definition.Spec.Capabilities)
	if err != nil {
		return nil, err
	}
	expected, err := bootstrap.DescribePackage(directory, definition, bindings)
	if err != nil {
		return nil, err
	}
	if len(definition.Spec.Platforms) > 0 {
		if _, err := pluginplatform.Select(definition.Spec.Platforms, pluginplatform.Current()); err != nil {
			return nil, err
		}
	}
	return &suite{directory: directory, executable: filepath.Join(directory, manifest.Executable(definition, pluginplatform.Current())), expected: expected, bindings: bindings, registry: registry}, nil
}

func (s *suite) start(ctx context.Context) (*native.Handle, error) {
	return native.Start(ctx, s.executable, native.WithDirectory(s.directory), native.WithDescription(s.expected))
}

func (s *suite) run(ctx context.Context, config *Config, options *Options, report *Report) error {
	// Description validation always runs, even when selecting a single test.
	handle, err := s.start(ctx)
	if err != nil {
		err = withDetail(err, "%s", s.startDetail(err))
		report.add(Result{Name: "manifest-handler-agreement", Detail: detailOf(err)})
		return err
	}
	handle.Close()
	report.add(Result{Name: "manifest-handler-agreement", Passed: true})
	if options.Integration {
		return runSteps(ctx, s.directory, config.Integration, options, report)
	}
	return s.runFixtures(ctx, config, options, report)
}

func (s *suite) runFixtures(ctx context.Context, config *Config, options *Options, report *Report) error {
	initialResults := len(report.Results)
	if err := s.extensions(ctx, options.Run, report); err != nil {
		return err
	}
	if config.OIDC != nil && matches(options.Run, "oidc-lifecycle") {
		err := s.oidc(ctx, config.OIDC)
		report.add(Result{Name: "oidc-lifecycle", Passed: err == nil, Detail: failureDetail(err)})
		if err != nil {
			return errors.Join(errUtils.ErrTestsFailed, err)
		}
	}
	if err := s.commands(ctx, config, options, report); err != nil {
		return err
	}
	if err := runSteps(ctx, s.directory, config.Steps, options, report); err != nil {
		return err
	}
	if len(report.Results) == initialResults {
		return s.noMatch(config, options.Run)
	}
	return nil
}

// noMatch explains an empty selection and lists what the filter could have selected.
func (s *suite) noMatch(config *Config, filter string) error {
	names := s.testNames(config)
	builder := errUtils.Build(errUtils.ErrPluginTestsNotMatched)
	if len(names) == 0 {
		return builder.WithExplanation("Neither plugin.tests.yaml nor any registered extension defines a test.").
			WithHint("Add `commands` or `steps` entries to plugin.tests.yaml.").Err()
	}
	return builder.WithExplanationf("`--run %s` is not contained in any test name.", filter).
		WithHintf("Available tests: %s.", strings.Join(names, ", ")).Err()
}

// testNames lists every name `--run` can select, in execution order.
func (s *suite) testNames(config *Config) []string {
	names := []string{}
	for _, binding := range s.bindings {
		if descriptor, found := s.registry.Get(binding.Capability); found && descriptor.Test != nil {
			names = append(names, "extension/"+binding.Capability+"/"+binding.Name)
		}
	}
	if config.OIDC != nil {
		names = append(names, "oidc-lifecycle")
	}
	for index := range config.Commands {
		names = append(names, "command/"+config.Commands[index].Name)
	}
	return append(names, stepNames(config.Steps)...)
}

func stepNames(steps []schema.WorkflowStep) []string {
	names := []string{}
	for index := range steps {
		names = append(names, steps[index].Name)
	}
	return names
}

// startDetail explains a failed handshake without echoing unmasked plugin output.
func (s *suite) startDetail(err error) string {
	if _, statErr := os.Stat(s.executable); statErr != nil {
		return fmt.Sprintf("plugin executable %q is not available (%s); run `atmos plugin build` first", clip(s.executable), clip(statErr.Error()))
	}
	return fmt.Sprintf("the plugin did not start or its Describe response disagrees with plugin.yaml: %s", clip(err.Error()))
}

// failureDetail returns recorded evidence, or a masked and bounded error message.
func failureDetail(err error) string {
	if err == nil {
		return ""
	}
	if detail := detailOf(err); detail != "" {
		return detail
	}
	return clip(err.Error())
}

func matches(filter, name string) bool { return filter == "" || strings.Contains(name, filter) }

func (s *suite) command(ctx context.Context, fixture *CommandFixture) error {
	if !s.exports("command", fixture.Name) {
		return withDetail(errUtils.ErrPluginInvalidConfig, "plugin.yaml does not export a command named %q", clip(fixture.Name))
	}
	handle, err := s.start(ctx)
	if err != nil {
		return withDetail(err, "%s", s.startDetail(err))
	}
	defer handle.Close()
	var stdout, stderr capturedOutput
	err = handle.Invoke(ctx, bridge.Invocation{Namespace: "conformance/command"}, func(ctx context.Context, invocation *pluginv1.Invocation) error {
		return handle.Command(ctx, &pluginv1.CommandRequest{Invocation: invocation, Name: fixture.Name, Arguments: fixture.Arguments}, &stdout, &stderr)
	})
	var exit errUtils.ExitCodeError
	if err != nil && !errors.As(err, &exit) {
		// A transport or protocol failure is never accepted as an expected exit status.
		return withDetail(errUtils.ErrPluginProtocol, "the command invocation failed before producing a result: %s", clip(err.Error()))
	}
	if mismatch := commandMismatch(fixture, stdout.String(), stderr.String(), errUtils.GetExitCode(err)); mismatch != "" {
		return withDetail(errUtils.ErrTestsFailed, "%s", mismatch)
	}
	return nil
}

func (s *suite) exports(capability, name string) bool {
	for _, binding := range s.bindings {
		if binding.Capability == capability && binding.Name == name {
			return true
		}
	}
	return false
}

func runSteps(ctx context.Context, directory string, steps []schema.WorkflowStep, options *Options, report *Report) error {
	selected := []schema.WorkflowStep{}
	for itemIndex := range steps {
		entry := steps[itemIndex]
		if matches(options.Run, entry.Name) {
			selected = append(selected, entry)
		}
	}
	if len(selected) == 0 {
		if options.Integration {
			return errUtils.Build(errUtils.ErrPluginTestsNotMatched).
				WithExplanationf("`--run %s` is not contained in any integration step name.", options.Run).
				WithHintf("Available integration steps: %s.", strings.Join(stepNames(steps), ", ")).Err()
		}
		return nil
	}
	vars := step.NewVariables()
	vars.OutputWriters = step.OutputWriters{Stdout: options.Stdout, Stderr: options.Stderr}
	parent := &schema.WorkflowStep{Type: schema.TaskTypeTest, Name: "plugin-authored-tests", WorkingDirectory: directory, Steps: selected}
	_, err := (&step.TestHandler{}).Execute(ctx, parent, vars)
	report.add(Result{Name: "authored-tests", Passed: err == nil, Detail: authoredDetail(err)})
	return err
}

// authoredDetail summarizes a failed authored test group. Per-test assertion output is written
// by the shared test reporter to the UI streams as the tests run.
func authoredDetail(err error) string {
	if err == nil {
		return ""
	}
	var failure *step.TestFailureError
	if errors.As(err, &failure) {
		return failure.Error() + " (per-test output was written to stderr as the tests ran)"
	}
	return clip(err.Error())
}

func (s *suite) commands(ctx context.Context, config *Config, options *Options, report *Report) error {
	for index := range config.Commands {
		fixture := &config.Commands[index]
		if !matches(options.Run, "command/"+fixture.Name) {
			continue
		}
		err := s.command(ctx, fixture)
		report.add(Result{Name: "command/" + fixture.Name, Passed: err == nil, Detail: failureDetail(err)})
		if err != nil {
			return errors.Join(errUtils.ErrTestsFailed, err)
		}
	}
	return nil
}
