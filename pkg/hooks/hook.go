package hooks

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Hook is the structure for a hook configured in stack YAML.
// Each hook has a Kind that determines what engine runs it.
type Hook struct {
	// Kind selects the engine that runs this hook. Built-in kinds:
	// "store" (existing semantics), "command" (generic binary execution),
	// plus step-backed kinds like "step" and "steps", and named tool kinds
	// like "infracost", "checkov", "trivy", "kics".
	//
	// For back-compat, the legacy `command:` YAML key is accepted as an
	// alias when `kind:` is absent. See UnmarshalYAML.
	Kind string `yaml:"kind,omitempty"`

	Events []string `yaml:"events,omitempty"`

	// When selects whether this hook runs. Empty defaults to success-only,
	// preserving the original behavior where after-* hooks fired only when the
	// operation succeeded.
	When schema.Condition `yaml:"when,omitempty"`

	// Generic command-kind fields. Used by the command engine and by named
	// tool kinds via their defaults.
	//
	// Command is the binary to execute (resolved via the toolchain). For
	// named kinds it defaults to the kind's binary; users may override.
	Command string            `yaml:"command,omitempty"`
	Args    []string          `yaml:"args,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	// Format is the inline rendering / parsing hint for generic kinds. Accepts
	// "markdown" (render the artifact inline), "sarif" (parse the output as
	// SARIF — giving a custom `kind: command` hook the same findings summary,
	// CI annotations, and SARIF upload as the built-in scanner kinds), or
	// empty (= downloadable artifact, no inline render).
	Format string `yaml:"format,omitempty"`

	// Results names the file a `format: sarif` command writes its SARIF to,
	// relative to $ATMOS_OUTPUT_DIR, for tools that write a fixed filename
	// into a directory rather than to $ATMOS_OUTPUT_FILE. When empty, the
	// SARIF is read from $ATMOS_OUTPUT_FILE.
	Results string `yaml:"results,omitempty"`

	// OnFailure is the failure mode. "warn" (default for tool kinds),
	// "fail" (propagate non-zero exit), or "ignore" (swallow).
	OnFailure string `yaml:"on_failure,omitempty"`

	// Tfmigrate-kind specific fields.
	Migration     string   `yaml:"migration,omitempty"`
	Config        string   `yaml:"config,omitempty"`
	BackendConfig []string `yaml:"backend_config,omitempty"`
	Mode          string   `yaml:"mode,omitempty"`

	// Store-kind specific (existing, unchanged semantics).
	Name    string            `yaml:"name,omitempty"`
	Outputs map[string]string `yaml:"outputs,omitempty"`

	// Git-kind specific (kind: git; see pkg/hooks/kinds/git).
	//
	// Repository names a managed repository under the top-level
	// git.repositories config. Empty targets the current repository.
	Repository string `yaml:"repository,omitempty"`
	// Commit configures the commit the git kind creates.
	Commit *GitCommitSpec `yaml:"commit,omitempty"`
	// Push pushes the created commit to the remote when true.
	Push bool `yaml:"push,omitempty"`

	// Step-kind specific (kind: step; see pkg/hooks/step_engine.go).
	//
	// Type names the step-registry step type to run (e.g. "container",
	// "toast", "http"). Required for kind: step.
	Type string `yaml:"type,omitempty"`
	// With holds the kind-specific payload. For kind: step it is the step's
	// parameter map, decoded into one WorkflowStep. For kind: steps it is an
	// ordered list of WorkflowStep objects. Rendered (templates + YAML
	// functions) by resolveHookForExecution before the engine sees it.
	With any `yaml:"with,omitempty"`
	// Retry wraps the step execution in retry.Do. Same schema as a
	// workflow step's retry block; interpreted by the bridge, not the step.
	Retry *schema.RetryConfig `yaml:"retry,omitempty"`
}

// GitCommitSpec is the `commit` block of a git-kind hook. Message supports
// the same template rendering as every other hook string (rendered by
// resolveHookForExecution before the engine runs).
type GitCommitSpec struct {
	// Message is the commit message; empty selects the engine default
	// ("Update artifacts for <component> in <stack>").
	Message string `yaml:"message,omitempty"`
	// Paths are repo-relative paths staged for the commit.
	Paths []string `yaml:"paths,omitempty"`
}

// UnmarshalYAML decodes a Hook, accepting the legacy `command:` key as an
// alias for `kind:` when `kind:` is absent. Pre-existing stack manifests
// with `command: store` continue to parse identically post-rename.
func (h *Hook) UnmarshalYAML(unmarshal func(any) error) error {
	type hookAlias Hook // avoid recursion into UnmarshalYAML
	var aux hookAlias
	if err := unmarshal(&aux); err != nil {
		return err
	}
	*h = Hook(aux)

	// Legacy alias fix-up: if `kind:` is empty but `command:` is set, the
	// YAML is the pre-rename form where `command:` was the dispatch
	// discriminator (e.g. "store"). Promote it to Kind and clear Command.
	if h.Kind == "" && h.Command != "" {
		h.Kind = h.Command
		h.Command = ""
	}
	return nil
}

// RunStatus is the outcome of the lifecycle operation a hook fires around.
type RunStatus string

// Lifecycle operation outcomes reported to hooks.
const (
	RunSuccess RunStatus = "success"
	RunFailure RunStatus = "failure"
)

// When values for Hook.When.
const (
	WhenSuccess = "success"
	WhenFailure = "failure"
	WhenAlways  = "always"
)

// RunsWhen reports whether this hook should run given lifecycle status and CI
// state. An empty When defaults to success-only, preserving the pre-When
// behavior where after-* hooks fired only on success.
func (h *Hook) RunsWhen(status RunStatus, isCI bool) bool {
	ok, err := h.RunsWhenE(schema.ConditionContext{
		CI:     isCI,
		Status: string(status),
	})
	return err == nil && ok
}

//nolint:gocritic // Public compatibility API keeps ConditionContext by value.
func (h *Hook) RunsWhenE(ctx schema.ConditionContext) (bool, error) {
	return h.When.EvaluateWithImplicitSuccessE(ctx)
}

// RunsOnStatus reports whether this hook should run given the lifecycle
// operation's status. It is retained for tests and callers that do not need CI
// conditions.
func (h *Hook) RunsOnStatus(status RunStatus) bool {
	return h.RunsWhen(status, false)
}

// MatchesEvent reports whether this hook should run for the given event.
// It normalises the yaml event format (hyphens, e.g. "after-terraform-apply")
// to the canonical Go format (dots, e.g. "after.terraform.apply") before
// comparing, so both styles are accepted in stack configuration.
//
// If the hook has no events configured, it matches all events to preserve
// backward compatibility with configs written before event filtering existed.
func (h Hook) MatchesEvent(event HookEvent) bool {
	if len(h.Events) == 0 {
		return true
	}
	normalizedEvent := event.Normalize()
	for _, e := range h.Events {
		if HookEvent(strings.ReplaceAll(e, "-", ".")).Normalize() == normalizedEvent {
			return true
		}
	}
	return false
}
