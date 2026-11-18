package component

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/degradation"
	m "github.com/cloudposse/atmos/pkg/merge"
	metricsprocess "github.com/cloudposse/atmos/pkg/metrics/process"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ComponentProvider is the interface that component type implementations
// must satisfy to register with the Atmos component registry.
//
// Component providers are registered via init() functions and enable
// extensible component type support without modifying core code.
type ComponentProvider interface {
	// GetType returns the component type identifier (e.g., "terraform", "helmfile", "mock").
	GetType() string

	// GetGroup returns the component group for categorization.
	// Examples: "Infrastructure as Code", "Kubernetes", "Images", "Testing"
	GetGroup() string

	// GetBasePath returns the base directory path for this component type.
	// Example: For terraform, returns "components/terraform"
	GetBasePath(atmosConfig *schema.AtmosConfiguration) string

	// ListComponents discovers all components of this type in a stack.
	// Returns component names found in the stack configuration.
	ListComponents(ctx context.Context, stack string, stackConfig map[string]any) ([]string, error)

	// ValidateComponent validates component configuration.
	// Returns error if configuration is invalid for this component type.
	ValidateComponent(config map[string]any) error

	// Execute runs a command for this component type.
	// Context provides all necessary information for execution.
	Execute(ctx *ExecutionContext) error

	// GenerateArtifacts creates necessary files for component execution.
	// Examples: backend.tf for Terraform, varfile for Helmfile
	GenerateArtifacts(ctx *ExecutionContext) error

	// GetAvailableCommands returns list of commands this component type supports.
	// Example: For terraform: ["plan", "apply", "destroy", "workspace"]
	GetAvailableCommands() []string
}

// ExecutionContext contains component execution metadata and the caller context.
// Construct values with keyed fields because new execution metadata may be added over time.
type ExecutionContext struct {
	// Context carries caller cancellation and deadlines into component execution.
	// Use GoContext so legacy callers that omit it retain background behavior.
	Context context.Context
	// Options carries process streams, environment, and reporting through provider dispatch.
	Options []ExecutionOption
	// WorkingDir is required by providers that generate artifacts for a prepared component.
	WorkingDir          string
	AtmosConfig         *schema.AtmosConfiguration
	ComponentType       string
	Component           string
	Stack               string
	Command             string
	SubCommand          string
	ComponentConfig     map[string]any
	ConfigAndStacksInfo schema.ConfigAndStacksInfo
	Args                []string
	Flags               map[string]any
}

// GoContext returns the caller context or a non-nil compatibility fallback.
func (e *ExecutionContext) GoContext() context.Context {
	if e != nil && e.Context != nil {
		return e.Context
	}
	return context.Background()
}

// ComponentInfo provides metadata about a component provider.
type ComponentInfo struct {
	Type        string
	Group       string
	Commands    []string
	Description string
}

type ExecuteDescribeComponentParams struct {
	// ResolveSecrets retrieves actual secrets for internal value-producing callers.
	// The default preserves credential-free, masked inspection.
	ResolveSecrets       bool
	AtmosConfig          *schema.AtmosConfiguration // Optional: Use provided config instead of initializing new one.
	Component            string
	Stack                string
	ComponentType        string // Optional: if set, only try this component type.
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	UseMocks             bool
	Skip                 []string
	AuthManager          auth.AuthManager
	AuthDisabled         bool
	ErrorOptions         DescribeStacksErrorOptions
	// CommandPath names the CLI command that needs the component, such as
	// ["secret", "shell"], for component admission. Empty means `describe component`.
	CommandPath []string
}

type (
	OnErrorMode                string
	DescribeStacksErrorOptions struct {
		// StaticOnly inspects merged configuration without auth, expression evaluation, workspace
		// synthesis, or evaluated-map cache reuse. Stack name templates allow only Go's pure built-ins.
		StaticOnly bool
		// EvaluationPaths optionally narrows list columns to fields within sections.
		EvaluationPaths [][]string
		OnError         OnErrorMode
		OnWarning       func(degradation.Warning)
		// OnProgress, when non-nil, is called once per stack file immediately before it's
		// processed — stackFile is the file name, index is 0-based, total is the number of
		// stack files describe-stacks will visit this call (both counted before filtering by
		// includeEmptyStacks, so total is a stable upper bound even though some files are
		// skipped). Bundled onto this struct (rather than a new parameter) purely for plumbing
		// convenience — it shares the same threading path as OnError/OnWarning. Optional; a nil
		// OnProgress is a no-op, matching every existing caller's behavior exactly.
		OnProgress func(stackFile string, index, total int)
		// StrictAuth, when true, keeps a component-specific auth-manager construction failure
		// fatal even under OnErrorWarn (see describeStacksProcessor.resolveComponentAuthManager) --
		// only the recoverable per-value YAML-function errors (e.g. !terraform.state hitting a
		// not-yet-provisioned backend) degrade. The zero value (false) preserves the original
		// list/describe --error-mode=warn behavior, where a component whose own declared identity
		// can't be resolved gracefully falls back to the parent AuthManager instead of aborting --
		// appropriate for merely listing/describing stacks, but not for a Terraform preflight
		// about to run real commands with whatever credentials it resolves.
		StrictAuth bool
		// ExtraComponentTypes names additional component kinds, such as custom kinds declared
		// by custom commands, to process after the built-in kinds. Each kind must already be
		// registered with the component registry for its stack section to be available. The
		// zero value leaves the described component kinds exactly as before.
		ExtraComponentTypes []string
	}
)

type DescribeAffectedCmdArgs struct {
	CLIConfig                   *schema.AtmosConfiguration
	Base                        string // Unified base commit (ref or SHA). Takes precedence over Ref/SHA.
	CloneTargetRef              bool
	Format                      string
	IncludeDependents           bool
	IncludeSettings             bool
	IncludeSpaceliftAdminStacks bool
	OutputFile                  string
	GithubOutputFile            string // Output file for $GITHUB_OUTPUT format (key=value).
	Ref                         string
	RepoPath                    string
	SHA                         string
	SSHKeyPath                  string
	SSHKeyPassword              string
	Verbose                     bool
	Upload                      bool
	Stack                       string
	Query                       string
	ProcessTemplates            bool
	ProcessYamlFunctions        bool
	Skip                        []string
	ExcludeLocked               bool
	Tags                        []string          // Keep only components whose `metadata.tags` contain any of these (from --tags).
	LabelsRaw                   string            // Raw --labels value (comma-separated key=value pairs).
	Labels                      map[string]string // Parsed --labels: keep only components whose `metadata.labels` contain all of these.
	TagsEnvVar                  string            // Environment variable that supplied Tags (for example ATMOS_TAGS); empty when Tags came from the command line or are unset.
	LabelsEnvVar                string            // Environment variable that supplied Labels; empty when Labels came from the command line or are unset.
	Flatten                     bool              // Lift every dependent into the top-level list as `affected: dependent` (from --flatten); requires IncludeDependents.
	FlattenEnvVar               string            // Environment variable that supplied Flatten; empty when it came from the command line or is unset.
	AuthManager                 auth.AuthManager  // Optional: Auth manager for credential management (from --identity flag).
	AuthDisabled                bool              // True when --identity=false (or alias) explicitly disables authentication; routes stack resolution to ExecuteDescribeStacksWithAuthDisabled.
	HeadSHAOverride             string            // PR head SHA from CI event payload, used for upload correlation with Atmos Pro.
	CIEventType                 string            // CI event type (e.g., "pull_request", "push") for upload validation.
	TargetBranch                string            // PR target branch (e.g., "main") used to auto-fetch when refs are missing locally.
	ErrorMode                   string            // How to handle recoverable errors: "strict" (default), "warn", or "silent".
	Cmd                         *cobra.Command    // The invoking Cobra command, used to derive Flags for the exec-metadata sync capture (proexec.FlagsFromCommand).
}

const (
	OnErrorStrict OnErrorMode = "strict"
	OnErrorWarn   OnErrorMode = "warn"
)

// ComponentDeferredContexts holds the per-section *merge.DeferredMergeContext produced while
// merging a single component's configuration, keyed by section name (vars, settings, env, auth,
// providers, required_providers, hooks, test, generate — see the cfg.*SectionName constants used
// as keys). Each context tracks the deferred YAML functions (!template, !terraform.output,
// !labels, etc.) collected for that section during the raw structural merge; a later,
// per-invocation stage resolves them with a real YAMLFunctionProcessor and deep-merges the result
// against any concrete override at the same path.
type ComponentDeferredContexts map[string]*m.DeferredMergeContext

// StackComponentDeferredContexts holds, for a single stack, every component's
// ComponentDeferredContexts keyed by [componentType][component] (e.g.
// ["terraform"]["vpc"]). It is the shape ProcessStackConfig produces per stack.
type StackComponentDeferredContexts map[string]map[string]ComponentDeferredContexts

// AllStacksDeferredContexts holds every stack's deferred contexts keyed by
// [stack][componentType][component]. This is the shape threaded from
// ProcessYAMLConfigFiles through FindStacksMap to the per-invocation Stage 3
// resolver (see resolveDeferredYamlFunctions).
type AllStacksDeferredContexts map[string]StackComponentDeferredContexts

// ExecutionOption is a functional option for ExecuteShellCommand.
type ExecutionOption func(*ExecutionOptions)

// ExecutionOptions holds optional configuration for shell command execution.
type ExecutionOptions struct {
	StdoutCapture  io.Writer
	StderrCapture  io.Writer
	StdoutOverride io.Writer
	Streams        *process.Streams
	Ctx            context.Context
	// ProcessEnv replaces os.Environ() as the process environment.
	// When set, ExecuteShellCommand uses this instead of re-reading os.Environ().
	// This is used when auth has already sanitized the environment (e.g., removed IRSA vars).
	ProcessEnv []string
	// InvokingCmd is the Cobra command the user actually invoked (e.g. the
	// terraform plan subcommand). Threaded through so ExecuteTerraform can
	// report the real Flags actually passed (FR-003b) to the exec-metadata
	// sync capture, instead of a pass-through-args collection that cannot
	// contain atmos-recognized flags (research.md Decision 14).
	InvokingCmd *cobra.Command
	// ExecMetadataParser, when set, is called once by captureExecMetadataSync
	// (sync-allowlisted commands only) to obtain command-specific structured
	// data (FR-006) for the execution record. cmd/terraform supplies this —
	// it can safely call pkg/ci/plugins/terraform's output parser, unlike
	// internal/exec, which cannot import it without reintroducing a confirmed
	// import cycle (research.md Decision 18). The output parameter is the
	// scoped exec-metadata output text executeCommandPipeline captured for
	// just the main plan/apply/deploy subprocess (FR-006f).
	ExecMetadataParser func(subCommand, component string, exitCode int, output string) any

	// ExecMetadataStdoutCapture/ExecMetadataStderrCapture, when set, additionally
	// tee this specific ExecuteShellCommand call's stdout/stderr into these
	// writers, alongside (not instead of) StdoutCapture/StderrCapture. Used
	// internally by executeCommandPipeline (via WithMetadataOutputCapture) to
	// scope the exec-metadata parser's input to only the final plan/apply/deploy
	// subprocess invocation, while StdoutCapture/StderrCapture's buffers keep
	// accumulating the whole pipeline's output for other consumers (e.g. CI
	// job-summary hooks) that legitimately need it (FR-006f, research.md
	// Decision 32).
	ExecMetadataStdoutCapture io.Writer
	ExecMetadataStderrCapture io.Writer

	// MetricsCallback, when set, is invoked once after the subprocess exits
	// with the subprocess tree's collected resource-usage metrics (nil-safe:
	// only called when a non-nil result was collected). Lets a caller (e.g.
	// executeMainTerraformCommand) capture this specific shell command's
	// subprocess-tree metrics for combining with atmos's own self-usage, for
	// both the local per-command display and the exec-metadata sync upload.
	MetricsCallback func(*metricsprocess.ProcessMetrics)
}

// WithInvokingCommand provides the Cobra command the user actually invoked,
// so ExecuteTerraform's exec-metadata sync capture can derive Flags from the
// command's own record of explicitly-set flags (proexec.FlagsFromCommand)
// rather than info.AdditionalArgsAndFlags.
func WithInvokingCommand(cmd *cobra.Command) ExecutionOption {
	defer perf.Track(nil, "component.WithInvokingCommand")()

	return func(c *ExecutionOptions) {
		c.InvokingCmd = cmd
	}
}

// InvokingCommandFromOptions extracts the invoking *cobra.Command (if any) set
// via WithInvokingCommand among opts.
func InvokingCommandFromOptions(opts ...ExecutionOption) *cobra.Command {
	defer perf.Track(nil, "component.InvokingCommandFromOptions")()

	var cfg ExecutionOptions
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg.InvokingCmd
}

// WithExecMetadataParser provides a closure that, given the invoking
// subcommand name, resolved component name, and command's own exit code, returns command-specific
// structured data (FR-006) for the execution record — or nil if there is
// none to report. The exitCode parameter is the terraform/tofu subprocess's
// own exit code (research.md Decision 27), supplied by
// captureExecMetadataSync at call time since it isn't known when this
// closure is created. Cmd/terraform
// supplies this closure so ExecuteTerraform's exec-metadata sync capture can
// obtain parsed terraform plan/apply/deploy output without internal/exec
// itself importing the CI plugin's parser (research.md Decision 18).
func WithExecMetadataParser(fn func(subCommand, component string, exitCode int, output string) any) ExecutionOption {
	defer perf.Track(nil, "component.WithExecMetadataParser")()

	return func(c *ExecutionOptions) {
		c.ExecMetadataParser = fn
	}
}

// MetadataParserFromOptions extracts the parser closure (if any) set via
// WithExecMetadataParser among opts.
func MetadataParserFromOptions(opts ...ExecutionOption) func(subCommand, component string, exitCode int, output string) any {
	defer perf.Track(nil, "component.MetadataParserFromOptions")()

	var cfg ExecutionOptions
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg.ExecMetadataParser
}

// WithMetricsCallback provides a closure that ExecuteShellCommand invokes
// once, after the subprocess exits, with the subprocess tree's collected
// resource-usage metrics. Used by executeMainTerraformCommand to capture the
// main plan/apply/deploy subprocess's own metrics for combining with atmos's
// self-usage — both for local display and the exec-metadata sync upload.
func WithMetricsCallback(fn func(*metricsprocess.ProcessMetrics)) ExecutionOption {
	defer perf.Track(nil, "component.WithMetricsCallback")()

	return func(c *ExecutionOptions) {
		c.MetricsCallback = fn
	}
}

// WithMetadataOutputCapture tees this specific ExecuteShellCommand call's
// stdout/stderr into stdoutW/stderrW, in addition to whatever
// WithStdoutCapture/WithStderrCapture already capture. Used internally by
// executeCommandPipeline to scope the exec-metadata parser's input to only the
// final plan/apply/deploy subprocess invocation (FR-006f), without disturbing
// the broader stdout/stderr buffers other consumers (e.g. cmd/terraform's
// capturedPlanOutput, used by CI job-summary hooks) rely on for the whole
// init+workspace-select+main pipeline.
func WithMetadataOutputCapture(stdoutW, stderrW io.Writer) ExecutionOption {
	defer perf.Track(nil, "component.WithMetadataOutputCapture")()

	return func(c *ExecutionOptions) {
		c.ExecMetadataStdoutCapture = stdoutW
		c.ExecMetadataStderrCapture = stderrW
	}
}

// MetadataOutputCaptureFromOptions extracts the exec-metadata stdout/stderr
// tee writers (if any) set via WithMetadataOutputCapture among opts, so
// callers that retry the underlying invocation (executeShellCommandWithRetry)
// can reset them between attempts.
func MetadataOutputCaptureFromOptions(opts ...ExecutionOption) (io.Writer, io.Writer) {
	defer perf.Track(nil, "component.MetadataOutputCaptureFromOptions")()

	var cfg ExecutionOptions
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg.ExecMetadataStdoutCapture, cfg.ExecMetadataStderrCapture
}

// WithStdoutCapture returns an ExecutionOption that tees stdout to the provided writer.
// The captured output is the raw, unmasked subprocess output; only the terminal copy is
// masked. Callers that persist or display captured output must mask it themselves.
func WithStdoutCapture(w io.Writer) ExecutionOption {
	defer perf.Track(nil, "component.WithStdoutCapture")()

	return func(c *ExecutionOptions) {
		c.StdoutCapture = w
	}
}

// WithStderrCapture returns an ExecutionOption that tees stderr to the provided writer.
// The captured output is the raw, unmasked subprocess output; only the terminal copy is
// masked. Callers that persist or display captured output must mask it themselves.
func WithStderrCapture(w io.Writer) ExecutionOption {
	defer perf.Track(nil, "component.WithStderrCapture")()

	return func(c *ExecutionOptions) {
		c.StderrCapture = w
	}
}

// WithStdoutOverride returns an ExecutionOption that replaces the default stdout
// (os.Stdout) with a different writer. Used to redirect noisy commands (e.g.,
// workspace select) to stderr so they don't pollute data-producing commands like output.
func WithStdoutOverride(w io.Writer) ExecutionOption {
	defer perf.Track(nil, "component.WithStdoutOverride")()

	return func(c *ExecutionOptions) {
		c.StdoutOverride = w
	}
}

// WithProcessStreams provides the subprocess standard streams.
func WithProcessStreams(streams process.Streams) ExecutionOption {
	defer perf.Track(nil, "component.WithProcessStreams")()

	return func(c *ExecutionOptions) {
		c.Streams = &streams
	}
}

// WithProcessContext provides the context used for subprocess execution.
func WithProcessContext(ctx context.Context) ExecutionOption {
	defer perf.Track(nil, "component.WithProcessContext")()

	return func(c *ExecutionOptions) {
		c.Ctx = ctx
	}
}

// ContextFromOptions returns the last supplied context or a background fallback.
func ContextFromOptions(opts ...ExecutionOption) context.Context {
	defer perf.Track(nil, "component.ContextFromOptions")()

	var cfg ExecutionOptions
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.Ctx == nil {
		return context.Background()
	}
	return cfg.Ctx
}

// WithEnvironment provides a pre-sanitized process environment for subprocess execution.
// When provided, ExecuteShellCommand uses this instead of re-reading os.Environ().
// Pass nil to fall back to the default os.Environ() behavior.
func WithEnvironment(env []string) ExecutionOption {
	defer perf.Track(nil, "component.WithEnvironment")()

	return func(c *ExecutionOptions) {
		c.ProcessEnv = env
	}
}
