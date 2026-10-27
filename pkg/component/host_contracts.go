package component

import (
	cobra "github.com/spf13/cobra"

	auth "github.com/cloudposse/atmos/pkg/auth"
	degradation "github.com/cloudposse/atmos/pkg/degradation"
	m "github.com/cloudposse/atmos/pkg/merge"
	schema "github.com/cloudposse/atmos/pkg/schema"
)

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
}

type (
	OnErrorMode                string
	DescribeStacksErrorOptions struct {
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
